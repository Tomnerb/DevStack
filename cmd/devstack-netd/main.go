package main

import (
	"context"
	"crypto/rand"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	gocni "github.com/containerd/go-cni"
	"github.com/opencontainers/runtime-spec/specs-go"
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

// containerCreateRequest is intentionally narrow: the privileged helper only
// creates DevStack-labelled containers in its own namespace. It never accepts
// arbitrary shell input.
type containerCreateRequest struct {
	ID          string   `json:"id"`
	Image       string   `json:"image"`
	Command     []string `json:"command"`
	Snapshotter string   `json:"snapshotter"`
	AutoStart   bool     `json:"autoStart"`
}

type imageImportRequest struct {
	Archive string `json:"archive"`
}

type volumeCopyRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type volumeInfoResponse struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Mountpoint string            `json:"mountpoint"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type volumeRemoveRequest struct {
	Name string `json:"name"`
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
	terminalMu sync.Mutex
	terminals  map[string]*terminalState
}

type terminalState struct {
	client  *containerd.Client
	process containerd.Process
	stdin   io.WriteCloser
	output  *terminalBuffer
}
type terminalBuffer struct {
	mu     sync.Mutex
	data   []byte
	closed bool
	err    string
}

func (b *terminalBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.data = append(b.data, p...)
	b.mu.Unlock()
	return len(p), nil
}
func (b *terminalBuffer) after(offset int64) (string, int64, bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(b.data)) {
		offset = int64(len(b.data))
	}
	return string(b.data[offset:]), int64(len(b.data)), b.closed, b.err
}
func (b *terminalBuffer) finish(err error) {
	b.mu.Lock()
	b.closed = true
	if err != nil {
		b.err = err.Error()
	}
	b.mu.Unlock()
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
		terminals:  make(map[string]*terminalState),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", srv.handleStatus)
	mux.HandleFunc("/v1/setup", srv.handleSetup)
	mux.HandleFunc("/v1/remove", srv.handleRemove)
	mux.HandleFunc("/v1/network", srv.handleInspect)
	mux.HandleFunc("/v1/containers/create", srv.handleContainerCreate)
	mux.HandleFunc("/v1/migration/images/import", srv.handleImageImport)
	mux.HandleFunc("/v1/migration/images/upload", srv.handleImageUpload)
	mux.HandleFunc("/v1/migration/volumes/copy", srv.handleVolumeCopy)
	mux.HandleFunc("/v1/migration/volumes/upload", srv.handleVolumeUpload)
	mux.HandleFunc("/v1/volumes", srv.handleVolumes)
	mux.HandleFunc("/v1/volumes/remove", srv.handleVolumeRemove)
	mux.HandleFunc("/v1/terminals/open", srv.handleTerminalOpen)
	mux.HandleFunc("/v1/terminals/input", srv.handleTerminalInput)
	mux.HandleFunc("/v1/terminals/output", srv.handleTerminalOutput)
	mux.HandleFunc("/v1/terminals/close", srv.handleTerminalClose)

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

func (s *server) handleImageImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request imageImportRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	archive := filepath.Clean(request.Archive)
	if !strings.HasPrefix(archive, "/tmp/devstack-docker-images-") || !strings.HasSuffix(archive, ".tar") {
		http.Error(w, "invalid migration archive", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(archive)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 40<<30 {
		http.Error(w, "migration archive is unavailable or invalid", http.StatusBadRequest)
		return
	}
	output, err := exec.Command("ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "devstack", "images", "import", archive).CombinedOutput()
	if err != nil {
		http.Error(w, strings.TrimSpace(string(output)), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Docker images imported into DevStack Native"})
}

// handleImageUpload receives an archive over the Unix socket instead of a
// caller-owned pathname. The service has PrivateTmp enabled, so it cannot (and
// must not) rely on seeing the desktop user's /tmp namespace.
func (s *server) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 40<<30)
	archive, err := os.CreateTemp(s.stateDir, "migration-image-*.tar")
	if err != nil {
		http.Error(w, "could not create migration archive", http.StatusInternalServerError)
		return
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	bytesWritten, copyErr := io.Copy(archive, r.Body)
	closeErr := archive.Close()
	if copyErr != nil || closeErr != nil || bytesWritten == 0 {
		http.Error(w, "could not receive migration archive", http.StatusBadRequest)
		return
	}
	output, err := exec.Command("ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "devstack", "images", "import", archivePath).CombinedOutput()
	if err != nil {
		http.Error(w, strings.TrimSpace(string(output)), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Docker image imported into DevStack Native"})
}

func (s *server) handleVolumeCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request volumeCopyRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(request.Name)
	source := filepath.Clean(request.Source)
	if !validID.MatchString(name) || !strings.HasPrefix(source, "/var/lib/docker/volumes/") {
		http.Error(w, "invalid Docker volume source", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		http.Error(w, "Docker volume data directory is unavailable", http.StatusBadRequest)
		return
	}
	destination := filepath.Join(filepath.Dir(s.stateDir), "volumes", name)
	if err := os.MkdirAll(destination, 0o750); err != nil {
		http.Error(w, "could not create DevStack volume", http.StatusInternalServerError)
		return
	}
	// Archive streaming preserves file modes and avoids placing user-controlled
	// paths into a shell command. Existing data is retained for retry safety.
	archive := exec.Command("tar", "-C", source, "-cf", "-", ".")
	extract := exec.Command("tar", "-C", destination, "-xf", "-")
	pipe, err := archive.StdoutPipe()
	if err != nil {
		http.Error(w, "could not prepare Docker volume copy", http.StatusInternalServerError)
		return
	}
	extract.Stdin = pipe
	var archiveErr, extractErr strings.Builder
	archive.Stderr = &archiveErr
	extract.Stderr = &extractErr
	if err := archive.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := extract.Start(); err != nil {
		_ = archive.Process.Kill()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	errExtract := extract.Wait()
	errArchive := archive.Wait()
	if errArchive != nil || errExtract != nil {
		http.Error(w, strings.TrimSpace(archiveErr.String()+" "+extractErr.String()), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "mountpoint": destination})
}

func (s *server) handleVolumeUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !validID.MatchString(name) {
		http.Error(w, "invalid volume name", http.StatusBadRequest)
		return
	}
	destination := filepath.Join(filepath.Dir(s.stateDir), "volumes", name)
	if err := os.MkdirAll(destination, 0o750); err != nil {
		http.Error(w, "could not create DevStack volume", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 40<<30)
	extract := exec.Command("tar", "-C", destination, "-xf", "-")
	extract.Stdin = r.Body
	output, err := extract.CombinedOutput()
	if err != nil {
		http.Error(w, strings.TrimSpace(string(output)), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "mountpoint": destination})
}

func (s *server) handleVolumes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	volumeDir := filepath.Join(filepath.Dir(s.stateDir), "volumes")
	entries, err := os.ReadDir(volumeDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, []volumeInfoResponse{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	volumes := make([]volumeInfoResponse, 0, len(entries))
	for _, entry := range entries {
		mountpoint := filepath.Join(volumeDir, entry.Name())
		if entry.IsDir() {
			// Directory-backed volume.
		} else if (entry.Type() & os.ModeSymlink) != 0 {
			info, err := os.Stat(mountpoint)
			if err != nil || !info.IsDir() {
				continue
			}
		} else {
			continue
		}

		info, err := os.Stat(mountpoint)
		if err != nil {
			continue
		}

		volumes = append(volumes, volumeInfoResponse{
			Name:       entry.Name(),
			Driver:     "local",
			Scope:      "local",
			Mountpoint: mountpoint,
			CreatedAt:  info.ModTime().Format(time.RFC3339),
			Labels: map[string]string{
				"devstack.io/managed": "true",
			},
		})
	}

	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})

	writeJSON(w, http.StatusOK, volumes)
}

func (s *server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request volumeRemoveRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(request.Name)
	if !validID.MatchString(name) {
		http.Error(w, "invalid volume name", http.StatusBadRequest)
		return
	}

	mountpoint := filepath.Join(filepath.Dir(s.stateDir), "volumes", name)
	_, err := os.Stat(mountpoint)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.RemoveAll(mountpoint); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request containerCreateRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validID.MatchString(request.ID) || strings.TrimSpace(request.Image) == "" || strings.ContainsAny(request.Image, " \t\n") {
		http.Error(w, "invalid container id or image reference", http.StatusBadRequest)
		return
	}
	if request.Snapshotter != "overlayfs" && request.Snapshotter != "native" {
		http.Error(w, "unsupported snapshotter", http.StatusBadRequest)
		return
	}
	for _, arg := range request.Command {
		if strings.ContainsRune(arg, 0) {
			http.Error(w, "invalid command argument", http.StatusBadRequest)
			return
		}
	}
	var args []string
	if request.AutoStart {
		args = []string{"--address", "/run/containerd/containerd.sock", "--namespace", "devstack", "run", "--detach", "--snapshotter", request.Snapshotter, "--label", "devstack.io/managed=true", request.Image, request.ID}
		args = append(args, request.Command...)
	} else {
		args = []string{"--address", "/run/containerd/containerd.sock", "--namespace", "devstack", "container", "create", "--snapshotter", request.Snapshotter, "--label", "devstack.io/managed=true", request.Image, request.ID}
		args = append(args, request.Command...)
	}
	output, err := exec.Command("ctr", args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		http.Error(w, "container create failed: "+message, http.StatusInternalServerError)
		return
	}
	state := "created"
	if request.AutoStart {
		state = "running"
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": request.ID, "state": state})
}

type terminalOpenRequest struct {
	ID     string `json:"id"`
	Width  uint   `json:"width"`
	Height uint   `json:"height"`
}
type terminalInputRequest struct {
	Session string `json:"session"`
	Input   string `json:"input"`
}

func (s *server) handleTerminalOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var request terminalOpenRequest
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !validID.MatchString(request.ID) {
		http.Error(w, "invalid container id", 400)
		return
	}
	cli, err := containerd.New("/run/containerd/containerd.sock", containerd.WithDefaultNamespace("devstack"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	container, err := cli.LoadContainer(r.Context(), request.ID)
	if err != nil {
		cli.Close()
		http.Error(w, err.Error(), 404)
		return
	}
	info, err := container.Info(r.Context())
	if err != nil || info.Labels["devstack.io/managed"] != "true" {
		cli.Close()
		http.Error(w, "container is not DevStack-managed", 403)
		return
	}
	task, err := container.Task(r.Context(), nil)
	if err != nil {
		cli.Close()
		http.Error(w, err.Error(), 409)
		return
	}
	spec, err := container.Spec(r.Context())
	if err != nil || spec.Process == nil {
		cli.Close()
		http.Error(w, "OCI process spec unavailable", 500)
		return
	}
	processSpec := *spec.Process
	processSpec.Terminal = true
	processSpec.Args = []string{"/bin/sh", "-i"}
	processSpec.Cwd = "/"
	processSpec.Env = append(processSpec.Env, "TERM=xterm-256color", "PS1=devstack$ ")
	processSpec.ConsoleSize = &specs.Box{Width: request.Width, Height: request.Height}
	if processSpec.ConsoleSize.Width == 0 {
		processSpec.ConsoleSize.Width = 120
	}
	if processSpec.ConsoleSize.Height == 0 {
		processSpec.ConsoleSize.Height = 28
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		cli.Close()
		http.Error(w, err.Error(), 500)
		return
	}
	session := fmt.Sprintf("%x", bytes)
	inR, inW := io.Pipe()
	out := &terminalBuffer{}
	creator := cio.NewCreator(cio.WithStreams(inR, out, out), cio.WithTerminal, cio.WithFIFODir("/run/devstack/fifo"))
	process, err := task.Exec(r.Context(), session, &processSpec, creator)
	if err != nil {
		cli.Close()
		http.Error(w, err.Error(), 500)
		return
	}
	exitC, err := process.Wait(context.Background())
	if err != nil {
		cli.Close()
		http.Error(w, err.Error(), 500)
		return
	}
	if err := process.Start(r.Context()); err != nil {
		cli.Close()
		http.Error(w, err.Error(), 500)
		return
	}
	s.terminalMu.Lock()
	s.terminals[session] = &terminalState{client: cli, process: process, stdin: inW, output: out}
	s.terminalMu.Unlock()
	go func() {
		status := <-exitC
		_, _, runErr := status.Result()
		process.IO().Wait()
		process.IO().Close()
		inR.Close()
		inW.Close()
		out.finish(runErr)
		cli.Close()
		s.terminalMu.Lock()
		delete(s.terminals, session)
		s.terminalMu.Unlock()
	}()
	writeJSON(w, 201, map[string]string{"session": session})
}

func (s *server) terminal(session string) (*terminalState, bool) {
	s.terminalMu.Lock()
	t, ok := s.terminals[session]
	s.terminalMu.Unlock()
	return t, ok
}
func (s *server) handleTerminalInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var q terminalInputRequest
	if err := decodeJSON(r, &q); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t, ok := s.terminal(q.Session)
	if !ok {
		http.Error(w, "terminal closed", 404)
		return
	}
	if _, err := t.stdin.Write([]byte(q.Input)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
func (s *server) handleTerminalOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	t, ok := s.terminal(r.URL.Query().Get("session"))
	if !ok {
		writeJSON(w, 200, map[string]any{"closed": true})
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	data, next, closed, errText := t.output.after(offset)
	writeJSON(w, 200, map[string]any{"data": data, "nextOffset": next, "closed": closed, "error": errText})
}
func (s *server) handleTerminalClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var q terminalInputRequest
	if err := decodeJSON(r, &q); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t, ok := s.terminal(q.Session)
	if ok {
		_ = t.stdin.Close()
		_, _ = t.process.Delete(r.Context(), containerd.WithProcessKill)
	}
	w.WriteHeader(204)
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
