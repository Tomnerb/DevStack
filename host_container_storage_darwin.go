//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type darwinContainerStorageLocation struct {
	id          string
	name        string
	description string
	path        string
	endpoint    string
}

func (s *DockerService) GetHostContainerStorage() ([]HostContainerStorageItem, error) {
	locations, err := darwinContainerStorageLocations()
	if err != nil {
		return nil, err
	}

	items := make([]HostContainerStorageItem, 0, len(locations))
	for _, location := range locations {
		info, statErr := os.Stat(location.path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, statErr
		}

		size, sizeErr := allocatedPathSize(location.path)
		if sizeErr != nil && !info.IsDir() {
			size = info.Size()
		}

		engineRunning := location.endpoint != "" && reachableDockerEndpoint(location.endpoint)
		canClean := engineRunning
		if location.id == "docker-desktop" {
			_, cliErr := dockerCLIPath()
			canClean = cliErr == nil
		}

		items = append(items, HostContainerStorageItem{
			ID:            location.id,
			Name:          location.name,
			Description:   location.description,
			Path:          location.path,
			SizeBytes:     size,
			Size:          formatStorageBytes(size),
			CanReveal:     true,
			CanClean:      canClean,
			EngineRunning: engineRunning,
			CleanupLabel:  "Review cleanup",
		})
	}

	return items, nil
}

func (s *DockerService) RevealHostContainerStorage(id string) error {
	location, err := resolveDarwinContainerStorageLocation(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(location.path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("container storage path no longer exists: %s", location.path)
		}
		return fmt.Errorf("access container storage path: %w", err)
	}

	return exec.Command("/usr/bin/open", "-R", location.path).Start()
}

func (s *DockerService) CleanHostContainerStorage(id string, mode string) (DockerCLIResult, error) {
	location, err := resolveDarwinContainerStorageLocation(id)
	if err != nil {
		return DockerCLIResult{}, err
	}
	if location.id == "docker-desktop" && !reachableDockerEndpoint(location.endpoint) {
		if err := startDockerDesktop(location.endpoint); err != nil {
			return DockerCLIResult{}, err
		}
	}
	if location.endpoint == "" || !reachableDockerEndpoint(location.endpoint) {
		return DockerCLIResult{}, errors.New("this container engine is not running; start it before cleaning unused data")
	}

	var args []string
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "cache":
		args = []string{"builder", "prune", "-a", "-f"}
	case "safe", "":
		args = []string{"system", "prune", "-f"}
	case "deep":
		args = []string{"system", "prune", "-a", "--volumes", "-f"}
	default:
		return DockerCLIResult{}, fmt.Errorf("unsupported container storage cleanup mode: %s", mode)
	}

	args = append([]string{"--host", location.endpoint}, args...)
	output, err := runDockerCLI(
		10*time.Minute,
		"",
		args...,
	)
	return DockerCLIResult{Output: output}, err
}

func startDockerDesktop(endpoint string) error {
	if endpoint == "" {
		return errors.New("Docker Desktop endpoint is unavailable")
	}

	dockerPath, err := dockerCLIPath()
	if err != nil {
		return errors.New("Docker CLI was not found; open Docker Desktop before cleaning its data")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, dockerPath, "desktop", "start").CombinedOutput()
	if err != nil {
		// Older Docker CLI builds do not expose `docker desktop start`. Opening
		// Docker.app provides the same startup behavior and works from a GUI app.
		openOutput, openErr := exec.Command("/usr/bin/open", "-a", "Docker").CombinedOutput()
		if openErr != nil {
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				detail = err.Error()
			}
			openDetail := strings.TrimSpace(string(openOutput))
			if openDetail == "" {
				openDetail = openErr.Error()
			}
			return fmt.Errorf("start Docker Desktop: %s; open Docker.app: %s", detail, openDetail)
		}
	}

	for attempt := 0; attempt < 120; attempt++ {
		if reachableDockerEndpoint(endpoint) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("Docker Desktop started but its engine did not become ready")
}

func resolveDarwinContainerStorageLocation(id string) (darwinContainerStorageLocation, error) {
	locations, err := darwinContainerStorageLocations()
	if err != nil {
		return darwinContainerStorageLocation{}, err
	}
	for _, location := range locations {
		if location.id == id {
			return location, nil
		}
	}
	return darwinContainerStorageLocation{}, fmt.Errorf("unknown container storage location: %s", id)
}

func darwinContainerStorageLocations() ([]darwinContainerStorageLocation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dockerDesktopSocket := filepath.Join(home, ".docker", "run", "docker.sock")
	dockerEndpoint := "unix://" + dockerDesktopSocket

	dockivaRoot := filepath.Join(home, "Library", "Application Support", "Dockiva")
	dockivaSocket := filepath.Join(dockivaRoot, "run", "docker.sock")

	return []darwinContainerStorageLocation{
		{
			id:          "docker-desktop",
			name:        "Docker container data",
			description: "Docker Desktop images, containers, volumes, and virtual-machine data.",
			path:        filepath.Join(home, "Library", "Containers", "com.docker.docker"),
			endpoint:    dockerEndpoint,
		},
		{
			id:          "dockiva-native",
			name:        "Dockiva Native data",
			description: "Dockiva's Linux guest, container images, volumes, and runtime state.",
			path:        filepath.Join(dockivaRoot, "guest"),
			endpoint:    "unix://" + dockivaSocket,
		},
	}, nil
}

func allocatedPathSize(path string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "/usr/bin/du", "-sk", path).Output()
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, err
	}

	kibibytes, parseErr := strconv.ParseInt(fields[0], 10, 64)
	if parseErr != nil {
		return 0, parseErr
	}
	return kibibytes * 1024, nil
}
