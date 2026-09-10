package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	gocni "github.com/containerd/go-cni"
)

const (
	defaultSocket    = "/run/devstack/netd.sock"
	defaultStateDir  = "/var/lib/devstack/networks"
	defaultCNIConfig = "/etc/cni/net.d/10-devstack.conflist"

	networkName   = "devstack-net"
	bridgeName    = "devstack0"
	networkSubnet = "10.89.0.0/16"
)

var validID = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`,
)

type portMapping struct {
	HostPort      int32  `json:"hostPort"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIp"`
}

type setupRequest struct {
	ID           string        `json:"id"`
	PortMappings []portMapping `json:"portMappings"`
}

type removeRequest struct {
	ID string `json:"id"`
}

type state struct {
	ID           string        `json:"id"`
	NetNSName    string        `json:"netnsName"`
	NetNSPath    string        `json:"netnsPath"`
	IPAddress    string        `json:"ipAddress"`
	PortMappings []portMapping `json:"portMappings"`
	CreatedAt    string        `json:"createdAt"`
}

type statusResponse struct {
	Ready      bool     `json:"ready"`
	Network    string   `json:"network,omitempty"`
	Bridge     string   `json:"bridge,omitempty"`
	Subnet     string   `json:"subnet,omitempty"`
	PluginDirs []string `json:"pluginDirs,omitempty"`
	Message    string   `json:"message,omitempty"`
}

type server struct {
	stateDir   string
	configFile string
	pluginDirs []string
	cni        gocni.CNI
	mu         sync.Mutex
}

func main() {
	socket := flag.String("socket", defaultSocket, "Unix socket path")
	stateDir := flag.String("state-dir", defaultStateDir, "network state directory")
	configFile := flag.String("cni-config", defaultCNIConfig, "CNI conflist path")
	flag.Parse()

	if os.Geteuid() != 0 {
		fatal(errors.New("devstack-netd must run as root"))
	}

	if _, err := exec.LookPath("ip"); err != nil {
		fatal(errors.New("ip command was not found; install iproute2"))
	}

	pluginDirs := discoverPluginDirs()
	if len(pluginDirs) == 0 {
		fatal(errors.New(
			"CNI plugin binaries bridge/host-local/loopback/portmap were not found",
		))
	}

	cni, err := gocni.New(
		gocni.WithPluginDir(pluginDirs),
		gocni.WithPluginConfDir(filepath.Dir(*configFile)),
		gocni.WithInterfacePrefix("eth"),
		gocni.WithMinNetworkCount(2),
	)
	if err != nil {
		fatal(err)
	}

	if err := cni.Load(
		gocni.WithLoNetwork,
		gocni.WithConfListFile(*configFile),
	); err != nil {
		fatal(fmt.Errorf("load CNI config: %w", err))
	}

	if err := os.MkdirAll(*stateDir, 0o750); err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(*socket), 0o755); err != nil {
		fatal(err)
	}

	_ = os.Remove(*socket)

	listener, err := net.Listen("unix", *socket)
	if err != nil {
		fatal(err)
	}
	defer listener.Close()

	if err := applySocketPermissions(*socket); err != nil {
		fatal(err)
	}

	srv := &server{
		stateDir:   *stateDir,
		configFile: *configFile,
		pluginDirs: pluginDirs,
		cni:        cni,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", srv.handleStatus)
	mux.HandleFunc("/v1/setup", srv.handleSetup)
	mux.HandleFunc("/v1/remove", srv.handleRemove)
	mux.HandleFunc("/v1/network", srv.handleInspect)

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}

	if err := httpServer.Serve(listener); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		fatal(err)
	}
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, statusResponse{
		Ready:      true,
		Network:    networkName,
		Bridge:     bridgeName,
		Subnet:     networkSubnet,
		PluginDirs: s.pluginDirs,
		Message:    "CNI bridge networking, host DNS, and localhost-only TCP publishing are ready.",
	})
}

func (s *server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request setupRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateSetupRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, err := s.readState(request.ID); err == nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	if err := s.checkPortConflicts(request.ID, request.PortMappings); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	netnsName := networkNamespaceName(request.ID)

	if output, err := exec.Command(
		"ip",
		"netns",
		"add",
		netnsName,
	).CombinedOutput(); err != nil {
		http.Error(
			w,
			"create network namespace: "+strings.TrimSpace(string(output)),
			http.StatusInternalServerError,
		)
		return
	}

	netnsPath := filepath.Join("/var/run/netns", netnsName)

	cniMappings := make([]gocni.PortMapping, 0, len(request.PortMappings))
	for _, mapping := range request.PortMappings {
		cniMappings = append(cniMappings, gocni.PortMapping{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			Protocol:      "tcp",
			HostIP:        "127.0.0.1",
		})
	}

	labels := map[string]string{
		"K8S_POD_NAMESPACE":          "devstack",
		"K8S_POD_NAME":               request.ID,
		"K8S_POD_INFRA_CONTAINER_ID": request.ID,
		"IgnoreUnknown":              "1",
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	result, err := s.cni.SetupSerially(
		ctx,
		request.ID,
		netnsPath,
		gocni.WithLabels(labels),
		gocni.WithCapabilityPortMap(cniMappings),
	)
	if err != nil {
		_ = s.cni.Remove(
			context.Background(),
			request.ID,
			netnsPath,
			gocni.WithLabels(labels),
			gocni.WithCapabilityPortMap(cniMappings),
		)
		_, _ = exec.Command("ip", "netns", "delete", netnsName).CombinedOutput()

		http.Error(w, "CNI setup: "+err.Error(), http.StatusInternalServerError)
		return
	}

	networkState := state{
		ID:           request.ID,
		NetNSName:    netnsName,
		NetNSPath:    netnsPath,
		IPAddress:    extractIPAddress(result),
		PortMappings: request.PortMappings,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.writeState(networkState); err != nil {
		_ = s.removeNetwork(context.Background(), networkState)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, networkState)
}

func (s *server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request removeRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !validID.MatchString(request.ID) {
		http.Error(w, "invalid container ID", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	networkState, err := s.readState(request.ID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := s.removeNetwork(ctx, networkState); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleInspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !validID.MatchString(id) {
		http.Error(w, "invalid container ID", http.StatusBadRequest)
		return
	}

	networkState, err := s.readState(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "network not found", http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, networkState)
}

func (s *server) removeNetwork(ctx context.Context, networkState state) error {
	cniMappings := make([]gocni.PortMapping, 0, len(networkState.PortMappings))
	for _, mapping := range networkState.PortMappings {
		cniMappings = append(cniMappings, gocni.PortMapping{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			Protocol:      "tcp",
			HostIP:        "127.0.0.1",
		})
	}

	labels := map[string]string{
		"K8S_POD_NAMESPACE":          "devstack",
		"K8S_POD_NAME":               networkState.ID,
		"K8S_POD_INFRA_CONTAINER_ID": networkState.ID,
		"IgnoreUnknown":              "1",
	}

	cniErr := s.cni.Remove(
		ctx,
		networkState.ID,
		networkState.NetNSPath,
		gocni.WithLabels(labels),
		gocni.WithCapabilityPortMap(cniMappings),
	)

	output, netnsErr := exec.Command(
		"ip",
		"netns",
		"delete",
		networkState.NetNSName,
	).CombinedOutput()

	if netnsErr != nil &&
		!strings.Contains(string(output), "No such file") {
		return fmt.Errorf(
			"delete netns: %s",
			strings.TrimSpace(string(output)),
		)
	}

	if cniErr != nil {
		return fmt.Errorf("CNI remove: %w", cniErr)
	}

	_ = os.Remove(s.statePath(networkState.ID))
	return nil
}

func validateSetupRequest(request setupRequest) error {
	if !validID.MatchString(request.ID) {
		return errors.New(
			"container ID must match [A-Za-z0-9][A-Za-z0-9_.-]{0,127}",
		)
	}

	seen := map[int32]struct{}{}

	for _, mapping := range request.PortMappings {
		if mapping.HostPort < 1024 || mapping.HostPort > 65535 {
			return fmt.Errorf(
				"host port %d is invalid; only localhost ports 1024-65535 are allowed",
				mapping.HostPort,
			)
		}

		if mapping.ContainerPort < 1 || mapping.ContainerPort > 65535 {
			return fmt.Errorf(
				"container port %d is invalid",
				mapping.ContainerPort,
			)
		}

		if mapping.Protocol != "" &&
			strings.ToLower(mapping.Protocol) != "tcp" {
			return errors.New("only TCP port publishing is allowed")
		}

		if mapping.HostIP != "" && mapping.HostIP != "127.0.0.1" {
			return errors.New("host IP is restricted to 127.0.0.1")
		}

		if _, exists := seen[mapping.HostPort]; exists {
			return fmt.Errorf("host port %d is duplicated", mapping.HostPort)
		}

		seen[mapping.HostPort] = struct{}{}
	}

	return nil
}

func (s *server) checkPortConflicts(id string, mappings []portMapping) error {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	used := map[int32]string{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.stateDir, entry.Name()))
		if err != nil {
			continue
		}

		var existing state
		if json.Unmarshal(data, &existing) != nil {
			continue
		}

		if existing.ID == id {
			continue
		}

		for _, mapping := range existing.PortMappings {
			used[mapping.HostPort] = existing.ID
		}
	}

	for _, mapping := range mappings {
		if owner, exists := used[mapping.HostPort]; exists {
			return fmt.Errorf(
				"host port %d is already published by DevStack container %s",
				mapping.HostPort,
				owner,
			)
		}

		listener, err := net.Listen(
			"tcp",
			fmt.Sprintf("127.0.0.1:%d", mapping.HostPort),
		)
		if err != nil {
			return fmt.Errorf(
				"host port %d is already in use: %w",
				mapping.HostPort,
				err,
			)
		}
		_ = listener.Close()
	}

	return nil
}

func extractIPAddress(result *gocni.Result) string {
	if result == nil {
		return ""
	}

	if config, ok := result.Interfaces["eth0"]; ok {
		for _, ipConfig := range config.IPConfigs {
			if ipConfig != nil && ipConfig.IP != nil {
				return ipConfig.IP.String()
			}
		}
	}

	for name, config := range result.Interfaces {
		if name == "lo" || config == nil {
			continue
		}

		for _, ipConfig := range config.IPConfigs {
			if ipConfig != nil && ipConfig.IP != nil {
				return ipConfig.IP.String()
			}
		}
	}

	return ""
}

func networkNamespaceName(id string) string {
	value := "devstack-" + id
	if len(value) > 63 {
		value = value[:63]
	}
	return strings.Trim(value, ".-_")
}

func (s *server) statePath(id string) string {
	return filepath.Join(s.stateDir, id+".json")
}

func (s *server) readState(id string) (state, error) {
	data, err := os.ReadFile(s.statePath(id))
	if err != nil {
		return state{}, err
	}

	var result state
	if err := json.Unmarshal(data, &result); err != nil {
		return state{}, err
	}

	return result, nil
}

func (s *server) writeState(value state) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	path := s.statePath(value.ID)
	temp := path + ".tmp"

	if err := os.WriteFile(temp, data, 0o640); err != nil {
		return err
	}

	return os.Rename(temp, path)
}

func discoverPluginDirs() []string {
	candidates := []string{
		"/opt/cni/bin",
		"/usr/lib/cni",
		"/usr/libexec/cni",
		"/usr/local/lib/cni",
	}

	required := []string{
		"bridge",
		"host-local",
		"loopback",
		"portmap",
	}

	var result []string

	for _, dir := range candidates {
		allFound := true

		for _, binary := range required {
			path := filepath.Join(dir, binary)
			info, err := os.Stat(path)

			if err != nil || info.IsDir() {
				allFound = false
				break
			}
		}

		if allFound {
			result = append(result, dir)
		}
	}

	return result
}

func applySocketPermissions(path string) error {
	group, err := user.LookupGroup("devstack")
	if err != nil {
		return errors.New("system group 'devstack' does not exist")
	}

	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}

	if err := os.Chown(path, 0, gid); err != nil {
		return err
	}

	return os.Chmod(path, 0o660)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "devstack-netd:", err)
	os.Exit(1)
}
