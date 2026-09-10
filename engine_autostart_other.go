//go:build darwin || windows

package main

func tryPlatformAutomaticEngineStart(
	service *DockerService,
	backend string,
	platformOption string,
) bool {
	backend = normalizeEngineBackend(backend)

	// Never launch external Docker Desktop automatically.
	if backend == "external" {
		return false
	}

	_, err := service.StartManagedEngine(
		backend,
		platformOption,
	)

	return err == nil
}
