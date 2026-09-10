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
	"os"
	"strings"
	"time"
)

const devstackNetworkHelperSocket = "/run/devstack/netd.sock"

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

func networkHelperStatus(ctx context.Context) NetworkHelperStatus {
	fallback := NetworkHelperStatus{
		Message: "DevStack network helper is not installed or not accessible.",
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
	if _, err := os.Stat(devstackNetworkHelperSocket); err != nil {
		return fmt.Errorf(
			"network helper socket %s is unavailable: %w; run scripts/install-containerd-networking.sh",
			devstackNetworkHelperSocket,
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
			}).DialContext(ctx, "unix", devstackNetworkHelperSocket)
		},
	}

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
		"http://devstack"+path,
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
				"permission denied connecting to %s; log out/in after joining the devstack group or rerun the networking installer",
				devstackNetworkHelperSocket,
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
