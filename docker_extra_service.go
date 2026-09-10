package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

const (
	composeWorkingDirLabel  = "com.docker.compose.project.working_dir"
	composeConfigFilesLabel = "com.docker.compose.project.config_files"
	composeEnvironmentLabel = "com.docker.compose.project.environment_file"
)

type ContainerMountDetail struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

type ContainerNetworkDetail struct {
	Name      string `json:"name"`
	IPAddress string `json:"ipAddress"`
	Gateway   string `json:"gateway"`
	Mac       string `json:"mac"`
}

type ContainerDetails struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Image         string                   `json:"image"`
	Created       string                   `json:"created"`
	Path          string                   `json:"path"`
	Args          []string                 `json:"args"`
	Entrypoint    []string                 `json:"entrypoint"`
	Command       []string                 `json:"command"`
	WorkingDir    string                   `json:"workingDir"`
	User          string                   `json:"user"`
	Environment   []string                 `json:"environment"`
	Mounts        []ContainerMountDetail   `json:"mounts"`
	Networks      []ContainerNetworkDetail `json:"networks"`
	Labels        map[string]string        `json:"labels"`
	RestartPolicy string                   `json:"restartPolicy"`
	State         string                   `json:"state"`
	ExitCode      int                      `json:"exitCode"`
	StartedAt     string                   `json:"startedAt"`
	FinishedAt    string                   `json:"finishedAt"`
}

type ComposeProjectDetails struct {
	Project     string   `json:"project"`
	WorkingDir  string   `json:"workingDir"`
	ConfigFiles []string `json:"configFiles"`
	EnvFile     string   `json:"envFile,omitempty"`
}

type DockerDiskUsageItem struct {
	Type        string `json:"type"`
	TotalCount  int    `json:"totalCount"`
	Active      int    `json:"active"`
	Size        string `json:"size"`
	Reclaimable string `json:"reclaimable"`
}

type DockerCLIResult struct {
	Output string `json:"output"`
}

func (s *DockerService) GetContainerDetails(containerID string) (ContainerDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.client.Load().ContainerInspect(
		ctx,
		containerID,
		client.ContainerInspectOptions{},
	)
	if err != nil {
		return ContainerDetails{}, err
	}

	var raw struct {
		ID      string   `json:"Id"`
		Name    string   `json:"Name"`
		Image   string   `json:"Image"`
		Created string   `json:"Created"`
		Path    string   `json:"Path"`
		Args    []string `json:"Args"`

		Config struct {
			Env        []string          `json:"Env"`
			Entrypoint []string          `json:"Entrypoint"`
			Cmd        []string          `json:"Cmd"`
			WorkingDir string            `json:"WorkingDir"`
			User       string            `json:"User"`
			Labels     map[string]string `json:"Labels"`
		} `json:"Config"`

		HostConfig struct {
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`

		State struct {
			Status     string `json:"Status"`
			ExitCode   int    `json:"ExitCode"`
			StartedAt  string `json:"StartedAt"`
			FinishedAt string `json:"FinishedAt"`
		} `json:"State"`

		Mounts []struct {
			Type        string `json:"Type"`
			Name        string `json:"Name"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`

		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress  string `json:"IPAddress"`
				Gateway    string `json:"Gateway"`
				MacAddress string `json:"MacAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}

	if err := json.Unmarshal(result.Raw, &raw); err != nil {
		return ContainerDetails{}, err
	}

	details := ContainerDetails{
		ID:            raw.ID,
		Name:          strings.TrimPrefix(raw.Name, "/"),
		Image:         raw.Image,
		Created:       raw.Created,
		Path:          raw.Path,
		Args:          raw.Args,
		Entrypoint:    raw.Config.Entrypoint,
		Command:       raw.Config.Cmd,
		WorkingDir:    raw.Config.WorkingDir,
		User:          raw.Config.User,
		Environment:   raw.Config.Env,
		Labels:        raw.Config.Labels,
		RestartPolicy: raw.HostConfig.RestartPolicy.Name,
		State:         raw.State.Status,
		ExitCode:      raw.State.ExitCode,
		StartedAt:     raw.State.StartedAt,
		FinishedAt:    raw.State.FinishedAt,
	}

	if details.Labels == nil {
		details.Labels = map[string]string{}
	}

	for _, mount := range raw.Mounts {
		details.Mounts = append(details.Mounts, ContainerMountDetail{
			Type:        mount.Type,
			Name:        mount.Name,
			Source:      mount.Source,
			Destination: mount.Destination,
			RW:          mount.RW,
		})
	}

	for name, network := range raw.NetworkSettings.Networks {
		details.Networks = append(details.Networks, ContainerNetworkDetail{
			Name:      name,
			IPAddress: network.IPAddress,
			Gateway:   network.Gateway,
			Mac:       network.MacAddress,
		})
	}

	return details, nil
}

func (s *DockerService) GetComposeProjectDetails(project string) (ComposeProjectDetails, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return ComposeProjectDetails{}, errors.New("compose project name is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.client.Load().ContainerList(
		ctx,
		client.ContainerListOptions{
			All: true,
		},
	)
	if err != nil {
		return ComposeProjectDetails{}, err
	}

	for _, container := range result.Items {
		if container.Labels[composeProjectLabel] != project {
			continue
		}

		workingDir := strings.TrimSpace(container.Labels[composeWorkingDirLabel])
		configFilesRaw := strings.TrimSpace(container.Labels[composeConfigFilesLabel])
		envFile := strings.TrimSpace(container.Labels[composeEnvironmentLabel])

		var configFiles []string
		for _, value := range strings.Split(configFilesRaw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}

			if !filepath.IsAbs(value) && workingDir != "" {
				value = filepath.Join(workingDir, value)
			}

			configFiles = append(configFiles, filepath.Clean(value))
		}

		return ComposeProjectDetails{
			Project:     project,
			WorkingDir:  workingDir,
			ConfigFiles: configFiles,
			EnvFile:     envFile,
		}, nil
	}

	return ComposeProjectDetails{}, fmt.Errorf("compose project %q was not found", project)
}

func (s *DockerService) RunComposeCommand(project string, action string) (DockerCLIResult, error) {
	action = strings.ToLower(strings.TrimSpace(action))

	switch action {
	case "up", "down", "build", "rebuild":
	default:
		return DockerCLIResult{}, fmt.Errorf("unsupported compose action: %s", action)
	}

	meta, err := s.GetComposeProjectDetails(project)
	if err != nil {
		return DockerCLIResult{}, err
	}

	if meta.WorkingDir == "" && len(meta.ConfigFiles) == 0 {
		return DockerCLIResult{}, fmt.Errorf(
			"compose project %q does not expose its working directory/config files",
			project,
		)
	}

	args := []string{
		"compose",
		"--ansi", "never",
		"-p", meta.Project,
	}

	if meta.WorkingDir != "" {
		args = append(args, "--project-directory", meta.WorkingDir)
	}

	if meta.EnvFile != "" {
		envFile := meta.EnvFile
		if !filepath.IsAbs(envFile) && meta.WorkingDir != "" {
			envFile = filepath.Join(meta.WorkingDir, envFile)
		}
		args = append(args, "--env-file", envFile)
	}

	for _, configFile := range meta.ConfigFiles {
		args = append(args, "-f", configFile)
	}

	switch action {
	case "up":
		args = append(args, "up", "-d")

	case "down":
		args = append(args, "down")

	case "build":
		args = append(args, "--progress", "plain", "build")

	case "rebuild":
		buildArgs := append([]string{}, args...)
		buildArgs = append(buildArgs, "--progress", "plain", "build")

		buildResult, err := runDockerCLI(30*time.Minute, meta.WorkingDir, buildArgs...)
		if err != nil {
			return DockerCLIResult{Output: buildResult}, err
		}

		upArgs := append([]string{}, args...)
		upArgs = append(upArgs, "up", "-d")

		upResult, err := runDockerCLI(10*time.Minute, meta.WorkingDir, upArgs...)
		output := strings.TrimSpace(buildResult + "\n" + upResult)

		return DockerCLIResult{Output: output}, err
	}

	output, err := runDockerCLI(30*time.Minute, meta.WorkingDir, args...)
	return DockerCLIResult{Output: output}, err
}

func (s *DockerService) PullImage(reference string) (DockerCLIResult, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return DockerCLIResult{}, errors.New("image reference is required")
	}

	output, err := runDockerCLI(30*time.Minute, "", "pull", reference)
	return DockerCLIResult{Output: output}, err
}

func (s *DockerService) CreateVolume(name string, driver string) (DockerCLIResult, error) {
	name = strings.TrimSpace(name)
	driver = strings.TrimSpace(driver)

	if name == "" {
		return DockerCLIResult{}, errors.New("volume name is required")
	}

	args := []string{"volume", "create"}

	if driver != "" && driver != "local" {
		args = append(args, "--driver", driver)
	}

	args = append(args, name)

	output, err := runDockerCLI(2*time.Minute, "", args...)
	return DockerCLIResult{Output: output}, err
}

func (s *DockerService) CreateNetwork(name string, driver string) (DockerCLIResult, error) {
	name = strings.TrimSpace(name)
	driver = strings.TrimSpace(driver)

	if name == "" {
		return DockerCLIResult{}, errors.New("network name is required")
	}

	if driver == "" {
		driver = "bridge"
	}

	switch driver {
	case "bridge", "overlay", "macvlan", "ipvlan":
	default:
		return DockerCLIResult{}, fmt.Errorf("unsupported network driver: %s", driver)
	}

	args := []string{
		"network", "create",
		"--driver", driver,
		name,
	}

	output, err := runDockerCLI(2*time.Minute, "", args...)
	return DockerCLIResult{Output: output}, err
}

func (s *DockerService) GetDiskUsage() ([]DockerDiskUsageItem, error) {
	output, err := runDockerCLI(
		2*time.Minute,
		"",
		"system", "df",
		"--format", "json",
	)
	if err != nil {
		return nil, err
	}

	var items []DockerDiskUsageItem

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw struct {
			Type        string `json:"Type"`
			TotalCount  string `json:"TotalCount"`
			Active      string `json:"Active"`
			Size        string `json:"Size"`
			Reclaimable string `json:"Reclaimable"`
		}

		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("parse docker disk usage: %w", err)
		}

		totalCount, _ := strconv.Atoi(raw.TotalCount)
		active, _ := strconv.Atoi(raw.Active)

		items = append(items, DockerDiskUsageItem{
			Type:        raw.Type,
			TotalCount:  totalCount,
			Active:      active,
			Size:        raw.Size,
			Reclaimable: raw.Reclaimable,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *DockerService) PruneDocker(scope string) (DockerCLIResult, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))

	var args []string

	switch scope {
	case "containers":
		args = []string{"container", "prune", "-f"}

	case "images":
		// Intentionally prune dangling images only. We do not use -a.
		args = []string{"image", "prune", "-f"}

	case "networks":
		args = []string{"network", "prune", "-f"}

	case "volumes":
		// Docker volume prune defaults to anonymous unused volumes.
		// Named volumes are intentionally not included.
		args = []string{"volume", "prune", "-f"}

	case "system":
		// Intentionally excludes volumes and unused tagged images.
		args = []string{"system", "prune", "-f"}

	default:
		return DockerCLIResult{}, fmt.Errorf("unsupported prune scope: %s", scope)
	}

	output, err := runDockerCLI(10*time.Minute, "", args...)
	return DockerCLIResult{Output: output}, err
}

func runDockerCLI(timeout time.Duration, dir string, args ...string) (string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return "", errors.New("docker CLI was not found in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, dockerPath, args...)

	if dir != "" {
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			cmd.Dir = dir
		}
	}

	cmd.Env = append(
		os.Environ(),
		"COMPOSE_MENU=false",
	)

	outputBytes, runErr := cmd.CombinedOutput()
	output := strings.TrimSpace(string(outputBytes))

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("docker command timed out")
	}

	if runErr != nil {
		if output == "" {
			return output, runErr
		}

		return output, fmt.Errorf("%s", output)
	}

	return output, nil
}
