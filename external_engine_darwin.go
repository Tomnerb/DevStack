//go:build darwin

package main

// startExternalDockerOnRequest is only called for an explicit user action.
// Automatic recovery must never launch an external desktop application.
func startExternalDockerOnRequest(endpoint string) (bool, error) {
	if dockerEndpointKind(endpoint) != "docker-desktop" {
		return false, nil
	}
	return true, startDockerDesktop(endpoint)
}
