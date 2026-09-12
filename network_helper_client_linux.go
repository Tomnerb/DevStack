//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const dockivaNetworkHelperSocket = "/run/dockiva/netd.sock"

type NetworkHelperStatus struct {
	Ready      bool     `json:"ready"`
	Network    string   `json:"network,omitempty"`
	Bridge     string   `json:"bridge,omitempty"`
	Subnet     string   `json:"subnet,omitempty"`
	PluginDirs []string `json:"pluginDirs,omitempty"`
	Message    string   `json:"message,omitempty"`
}

type NetworkHelperState struct {
	ID           string               `json:"id"`
	NetNSName    string               `json:"netnsName"`
	NetNSPath    string               `json:"netnsPath"`
	IPAddress    string               `json:"ipAddress"`
	PortMappings []RuntimePortMapping `json:"portMappings"`
}

type NetworkHelperSetupRequest struct {
	ID           string               `json:"id"`
	PortMappings []RuntimePortMapping `json:"portMappings"`
}

type NetworkHelperContainerCreateRequest struct {
	ID          string   `json:"id"`
	Image       string   `json:"image"`
	Command     []string `json:"command"`
	Snapshotter string   `json:"snapshotter"`
	AutoStart   bool     `json:"autoStart"`
}

type NetworkHelperVolume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Mountpoint string            `json:"mountpoint"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type NetworkHelperVolumeRemoveRequest struct {
	Name string `json:"name"`
}

func networkHelperCreateContainer(ctx context.Context, request NetworkHelperContainerCreateRequest) error {
	return networkHelperRequest(ctx, http.MethodPost, "/v1/containers/create", request, nil)
}

func networkHelperImportDockerImages(ctx context.Context, archive string) error {
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open image archive: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat image archive: %w", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", dockivaNetworkHelperSocket)
		},
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://dockiva/v1/migration/images/upload", file)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/vnd.oci.image.layout.v1.tar")
	request.ContentLength = info.Size()
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return errors.New(message)
	}
	return nil
}

func networkHelperCopyDockerVolume(ctx context.Context, name, source string) error {
	return networkHelperRequest(ctx, http.MethodPost, "/v1/migration/volumes/copy", map[string]string{
		"name": name, "source": source,
	}, nil)
}

func networkHelperUploadDockerVolume(ctx context.Context, name, archive string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", dockivaNetworkHelperSocket)
	}}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://dockiva/v1/migration/volumes/upload?name="+url.QueryEscape(name), file)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(strings.TrimSpace(string(body)))
	}
	return nil
}

func networkHelperListVolumes(ctx context.Context) ([]NetworkHelperVolume, error) {
	var out []NetworkHelperVolume
	if err := networkHelperRequest(
		ctx,
		http.MethodGet,
		"/v1/volumes",
		nil,
		&out,
	); err != nil {
		return nil, err
	}
	return out, nil
}

func networkHelperRemoveVolume(ctx context.Context, name string) error {
	return networkHelperRequest(
		ctx,
		http.MethodPost,
		"/v1/volumes/remove",
		NetworkHelperVolumeRemoveRequest{Name: name},
		nil,
	)
}

type networkHelperTerminalOutputChunk struct {
	Data       string `json:"data"`
	NextOffset int64  `json:"nextOffset"`
	Closed     bool   `json:"closed"`
	Error      string `json:"error"`
}

func networkHelperTerminalOpen(ctx context.Context, id string, width, height uint) (string, error) {
	var out map[string]string
	err := networkHelperRequest(ctx, http.MethodPost, "/v1/terminals/open", map[string]any{"id": id, "width": width, "height": height}, &out)
	return out["session"], err
}
func networkHelperTerminalInput(ctx context.Context, session, input string) error {
	return networkHelperRequest(ctx, http.MethodPost, "/v1/terminals/input", map[string]string{"session": session, "input": input}, nil)
}
func networkHelperTerminalOutput(ctx context.Context, session string, offset int64) (networkHelperTerminalOutputChunk, error) {
	var out networkHelperTerminalOutputChunk
	err := networkHelperRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/terminals/output?session=%s&offset=%d", session, offset), nil, &out)
	return out, err
}
func networkHelperTerminalClose(ctx context.Context, session string) error {
	return networkHelperRequest(ctx, http.MethodPost, "/v1/terminals/close", map[string]string{"session": session}, nil)
}

func networkHelperStatus(ctx context.Context) NetworkHelperStatus {
	fallback := NetworkHelperStatus{
		Message: "Dockiva network helper is not installed or not accessible.",
	}

	var response NetworkHelperStatus
	if err := networkHelperRequest(ctx, http.MethodGet, "/v1/status", nil, &response); err != nil {
		fallback.Message = err.Error()
		return fallback
	}

	return response
}

func networkHelperSetup(
	ctx context.Context,
	id string,
	ports []RuntimePortMapping,
) (NetworkHelperState, error) {
	var response NetworkHelperState

	err := networkHelperRequest(
		ctx,
		http.MethodPost,
		"/v1/setup",
		NetworkHelperSetupRequest{
			ID:           id,
			PortMappings: ports,
		},
		&response,
	)

	return response, err
}

func networkHelperInspect(
	ctx context.Context,
	id string,
) (NetworkHelperState, error) {
	var response NetworkHelperState

	err := networkHelperRequest(
		ctx,
		http.MethodGet,
		"/v1/network?id="+id,
		nil,
		&response,
	)

	return response, err
}

func networkHelperRemove(ctx context.Context, id string) error {
	return networkHelperRequest(
		ctx,
		http.MethodPost,
		"/v1/remove",
		map[string]string{"id": id},
		nil,
	)
}

func networkHelperRequest(
	ctx context.Context,
	method string,
	path string,
	body any,
	out any,
) error {
	if _, err := os.Stat(dockivaNetworkHelperSocket); err != nil {
		return fmt.Errorf(
			"network helper socket %s is unavailable: %w; run scripts/install-containerd-networking.sh",
			dockivaNetworkHelperSocket,
			err,
		)
	}

	transport := &http.Transport{
		DialContext: func(
			ctx context.Context,
			network string,
			address string,
		) (net.Conn, error) {
			return (&net.Dialer{
				Timeout: 2 * time.Second,
			}).DialContext(ctx, "unix", dockivaNetworkHelperSocket)
		},
	}
	// This short-lived client is used for individual helper RPCs. Explicitly
	// close its idle Unix connection after each call; otherwise terminal polling
	// creates an unbounded number of persistConn goroutines.
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
	}

	var reader io.Reader

	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		method,
		"http://dockiva"+path,
		reader,
	)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, os.ErrPermission) ||
			strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			return fmt.Errorf(
				"permission denied connecting to %s; log out/in after joining the dockiva group or rerun the networking installer",
				dockivaNetworkHelperSocket,
			)
		}
		return err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = response.Status
		}
		return errors.New(message)
	}

	if out == nil || len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode network helper response: %w", err)
	}

	return nil
}
