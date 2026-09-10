//go:build linux

package main

func tryPlatformAutomaticEngineStart(
	service *DockerService,
	backend string,
	platformOption string,
) bool {
	backend = normalizeEngineBackend(backend)

	if backend == "external" {
		return false
	}

	_, err := service.StartManagedEngine(backend, platformOption)
	return err == nil
}
