//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

const dockivaWSLPort = 23750

type windowsEngineBackend struct{}

func newPlatformEngineBackend() platformEngineBackend {
	return windowsEngineBackend{}
}

func (windowsEngineBackend) Name() string {
	return "wsl2"
}

func nativeRuntimeProvider() string {
	return "docker"
}

func (windowsEngineBackend) Options() []string {
	return listWSLDistros()
}

func (backend windowsEngineBackend) Status(
	_ *DockerService,
	distro string,
) EngineStatus {
	status := EngineStatus{
		Backend:    "wsl2",
		Platform:   "windows",
		Supported:  true,
		Message:    "Dockiva-managed Docker bridge through WSL2.",
		WSLDistros: listWSLDistros(),
	}

	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		status.Message = "WSL is not installed."
		return status
	}

	status.HelperInstalled = true

	distro = strings.TrimSpace(distro)
	if distro == "" {
		status.Message = "Choose an installed WSL2 Linux distribution."
		return status
	}

	found := false
	for _, item := range status.WSLDistros {
		if strings.EqualFold(item, distro) {
			found = true
			break
		}
	}

	if !found {
		status.Message = "The selected WSL distribution is not installed."
		return status
	}

	output, _ := runCommand(
		8*time.Second,
		"",
		wsl,
		"-d",
		distro,
		"--",
		"sh",
		"-lc",
		"command -v dockerd >/dev/null 2>&1 && echo docker=yes || echo docker=no; command -v socat >/dev/null 2>&1 && echo socat=yes || echo socat=no",
	)

	status.EngineInstalled =
		strings.Contains(output, "docker=yes") &&
			strings.Contains(output, "socat=yes")

	status.Endpoint = fmt.Sprintf(
		"tcp://127.0.0.1:%d",
		dockivaWSLPort,
	)

	probe, err := client.New(
		client.WithHost(status.Endpoint),
		client.WithUserAgent("dockiva/0.9.0"),
	)
	if err == nil {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			1200*time.Millisecond,
		)

		_, pingErr := probe.Ping(
			ctx,
			client.PingOptions{
				NegotiateAPIVersion: true,
			},
		)

		cancel()
		_ = probe.Close()

		status.Running = pingErr == nil
	}

	if status.Running {
		status.Message = "Dockiva's WSL2 Docker bridge is reachable."
	} else if status.EngineInstalled {
		status.Message = "Docker is provisioned; start the Dockiva WSL bridge."
	} else {
		status.Message = "Docker and socat are not provisioned in this WSL distribution."
	}

	return status
}

func (backend windowsEngineBackend) Start(
	service *DockerService,
	distro string,
) (EngineActionResult, error) {
	distro = strings.TrimSpace(distro)
	if distro == "" {
		return EngineActionResult{
			Status: backend.Status(service, distro),
		}, errors.New("choose a WSL distribution first")
	}

	status := backend.Status(service, distro)
	if !status.EngineInstalled {
		return EngineActionResult{
			Status: status,
		}, errors.New("provision Docker in the selected WSL distribution first")
	}

	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return EngineActionResult{
			Status: status,
		}, err
	}

	script := fmt.Sprintf(
		`(systemctl start docker || service docker start || true); pkill -f 'socat TCP-LISTEN:%d' >/dev/null 2>&1 || true; nohup socat TCP-LISTEN:%d,bind=127.0.0.1,reuseaddr,fork UNIX-CONNECT:/var/run/docker.sock >/tmp/dockiva-socat.log 2>&1 & sleep 1; docker info >/dev/null 2>&1; echo "Dockiva Docker bridge listening on 127.0.0.1:%d"`,
		dockivaWSLPort,
		dockivaWSLPort,
		dockivaWSLPort,
	)

	output, err := runCommand(
		2*time.Minute,
		"",
		wsl,
		"-d",
		distro,
		"-u",
		"root",
		"--",
		"sh",
		"-lc",
		script,
	)
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, distro),
			Output: output,
		}, err
	}

	endpoint := fmt.Sprintf(
		"tcp://127.0.0.1:%d",
		dockivaWSLPort,
	)

	var lastErr error

	for attempt := 0; attempt < 10; attempt++ {
		if err := service.ConfigureNativeDockerEndpoint(endpoint); err == nil {
			if err := service.SelectContainerRuntime(nativeRuntimeProvider()); err != nil {
				lastErr = err
				continue
			}
			return EngineActionResult{
				Status: backend.Status(service, distro),
				Output: output,
			}, nil
		} else {
			lastErr = err
		}

		time.Sleep(350 * time.Millisecond)
	}

	return EngineActionResult{
			Status: backend.Status(service, distro),
			Output: output,
		}, fmt.Errorf(
			"WSL bridge started but Docker API is not reachable: %w",
			lastErr,
		)
}

func (backend windowsEngineBackend) Stop(
	service *DockerService,
	distro string,
) (EngineActionResult, error) {
	distro = strings.TrimSpace(distro)
	if distro == "" {
		return EngineActionResult{
			Status: backend.Status(service, distro),
		}, errors.New("choose a WSL distribution first")
	}

	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, distro),
		}, err
	}

	output, err := runCommand(
		30*time.Second,
		"",
		wsl,
		"-d",
		distro,
		"-u",
		"root",
		"--",
		"sh",
		"-lc",
		fmt.Sprintf(
			"pkill -f 'socat TCP-LISTEN:%d' >/dev/null 2>&1 || true; echo bridge-stopped",
			dockivaWSLPort,
		),
	)
	if err == nil {
		service.useOfflineNativeRuntime(backend.Status(service, distro))
	}

	return EngineActionResult{
		Status: backend.Status(service, distro),
		Output: output,
	}, err
}

func (backend windowsEngineBackend) Delete(
	service *DockerService,
	distro string,
) (EngineActionResult, error) {
	return EngineActionResult{
			Status: backend.Status(service, distro),
		}, errors.New(
			"Dockiva will not unregister or delete a user's WSL distribution",
		)
}

func (backend windowsEngineBackend) Provision(
	service *DockerService,
	distro string,
) (EngineActionResult, error) {
	distro = strings.TrimSpace(distro)
	if distro == "" {
		return EngineActionResult{
			Status: backend.Status(service, distro),
		}, errors.New("choose a WSL distribution first")
	}

	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return EngineActionResult{
				Status: backend.Status(service, distro),
			}, errors.New(
				"WSL is not installed; use elevated PowerShell: wsl --install",
			)
	}

	script := `. /etc/os-release; case "${ID:-}:${ID_LIKE:-}" in *ubuntu*|*debian*) ;; *) echo "Automatic provisioning supports Ubuntu/Debian-family WSL distributions."; exit 64;; esac; export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get install -y docker.io socat; (systemctl enable --now docker || service docker start || true); docker info >/dev/null 2>&1; echo "Docker Engine and socat are ready."`

	output, err := runCommand(
		20*time.Minute,
		"",
		wsl,
		"-d",
		distro,
		"-u",
		"root",
		"--",
		"sh",
		"-lc",
		script,
	)

	return EngineActionResult{
		Status: backend.Status(service, distro),
		Output: output,
	}, err
}

func listWSLDistros() []string {
	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return []string{}
	}

	output, err := runCommand(
		8*time.Second,
		"",
		wsl,
		"--list",
		"--quiet",
	)
	if err != nil {
		return []string{}
	}

	output = strings.ReplaceAll(output, "\x00", "")

	result := make([]string, 0)

	for _, line := range strings.Split(output, "\n") {
		value := strings.TrimSpace(line)
		if value != "" {
			result = append(result, value)
		}
	}

	sort.Strings(result)
	return result
}
