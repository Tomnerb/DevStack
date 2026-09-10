//go:build windows

package main

import (
	"fmt"
)

func platformRuntimeCandidates() []RuntimeCandidateInfo {
	return []RuntimeCandidateInfo{
		{
			Provider:    "docker",
			DisplayName: "Docker Engine (Moby API)",
			Available:   true,
			Detected:    true,
			Selectable:  true,
			Message:     "Current runtime implementation.",
		},
		{
			Provider:     "containerd",
			DisplayName:  "containerd",
			Available:    false,
			Detected:     false,
			Selectable:   false,
			Experimental: true,
			Message:      "Direct containerd will run inside DevStack's minimal Linux guest on this platform in a future milestone.",
		},
	}
}

func createPlatformRuntime(
	service *DockerService,
	provider string,
) (ContainerRuntime, error) {
	return nil, fmt.Errorf(
		"runtime %q is not directly available on this operating system",
		provider,
	)
}
