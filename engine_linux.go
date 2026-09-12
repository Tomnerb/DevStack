//go:build linux

package main

import "errors"

type linuxEngineBackend struct{}

func newPlatformEngineBackend() platformEngineBackend {
	return linuxEngineBackend{}
}

func (linuxEngineBackend) Name() string {
	return "native"
}

func nativeRuntimeProvider() string {
	return "containerd"
}

func (linuxEngineBackend) Options() []string {
	return []string{}
}

func (backend linuxEngineBackend) Status(_ *DockerService, _ string) EngineStatus {
	endpoint, reachable, message := detectContainerdSocket()
	status := EngineStatus{
		Backend:         backend.Name(),
		Platform:        "linux",
		Supported:       true,
		HelperInstalled: true,
		EngineInstalled: endpoint != "",
		Running:         reachable,
		Endpoint:        endpoint,
		Message:         message,
	}
	if reachable {
		status.Message = "Dockiva native containerd is running."
	}
	return status
}

func (backend linuxEngineBackend) Start(service *DockerService, option string) (EngineActionResult, error) {
	status := backend.Status(service, option)
	if !status.Running {
		service.useOfflineNativeRuntime(status)
		return EngineActionResult{Status: status}, errors.New(
			"Dockiva native containerd is not running; install/start containerd and ensure its socket is accessible",
		)
	}
	if err := service.SelectContainerRuntime(nativeRuntimeProvider()); err != nil {
		return EngineActionResult{Status: status}, err
	}
	return EngineActionResult{
		Status: status,
		Output: "Connected to Dockiva native containerd at " + status.Endpoint + ".",
	}, nil
}

func (backend linuxEngineBackend) Stop(service *DockerService, option string) (EngineActionResult, error) {
	return EngineActionResult{Status: backend.Status(service, option)},
		errors.New("Dockiva will not stop the host containerd service")
}

func (backend linuxEngineBackend) Delete(service *DockerService, option string) (EngineActionResult, error) {
	return EngineActionResult{Status: backend.Status(service, option)},
		errors.New("native Linux containerd is not a Dockiva-owned VM")
}

func (backend linuxEngineBackend) Provision(service *DockerService, option string) (EngineActionResult, error) {
	return EngineActionResult{Status: backend.Status(service, option)},
		errors.New("Linux containerd provisioning is handled by the host distribution")
}
