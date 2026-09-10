package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

type dockerRuntime struct {
	service *DockerService
}

func newDockerRuntime(service *DockerService) ContainerRuntime {
	return &dockerRuntime{service: service}
}

func (r *dockerRuntime) Info(ctx context.Context) ContainerRuntimeInfo {
	info := ContainerRuntimeInfo{
		Provider:    "docker",
		DisplayName: "Docker Engine (Moby API)",
		Capabilities: RuntimeCapabilities{
			Containers:      true,
			Lifecycle:       true,
			Stats:           true,
			Logs:            true,
			Terminal:        true,
			Images:          true,
			PullImages:      false,
			CreateContainer: false,
			Volumes:         true,
			Networks:        true,
			PortPublishing:  true,
			DNS:             true,
			Compose:         true,
		},
	}

	cli := r.service.client.Load()
	if cli == nil {
		info.Message = "Docker client is not initialized."
		return info
	}

	ping, err := cli.Ping(ctx, client.PingOptions{
		NegotiateAPIVersion: true,
	})
	if err != nil {
		info.Message = err.Error()
		return info
	}

	info.Connected = true
	info.Message = fmt.Sprintf("Docker Engine API %s is connected.", ping.APIVersion)
	info.Endpoint = r.service.externalEngineStatus().Endpoint
	return info
}

func (r *dockerRuntime) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	cli := r.service.client.Load()
	if cli == nil {
		return nil, fmt.Errorf("Docker client is not initialized")
	}

	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	containers := make([]ContainerInfo, 0, len(result.Items))
	for _, container := range result.Items {
		name := ""
		if len(container.Names) > 0 {
			name = strings.TrimPrefix(container.Names[0], "/")
		}

		ports := make([]PortInfo, 0, len(container.Ports))
		for _, port := range container.Ports {
			display := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
			if port.PublicPort > 0 {
				display = fmt.Sprintf(
					"%d:%d/%s",
					port.PublicPort,
					port.PrivatePort,
					port.Type,
				)
			}

			ports = append(ports, PortInfo{
				PrivatePort: port.PrivatePort,
				PublicPort:  port.PublicPort,
				Type:        port.Type,
				Display:     display,
				URL: localhostURL(
					port.PublicPort,
					port.PrivatePort,
					port.Type,
				),
			})
		}

		sort.Slice(ports, func(i, j int) bool {
			if ports[i].PublicPort == ports[j].PublicPort {
				return ports[i].PrivatePort < ports[j].PrivatePort
			}
			return ports[i].PublicPort < ports[j].PublicPort
		})

		containers = append(containers, ContainerInfo{
			ID:             container.ID,
			ShortID:        shortenID(container.ID),
			Name:           name,
			Image:          container.Image,
			State:          string(container.State),
			Status:         container.Status,
			ComposeProject: container.Labels[composeProjectLabel],
			ComposeService: container.Labels[composeServiceLabel],
			Ports:          ports,
		})
	}

	return containers, nil
}

func (r *dockerRuntime) StartContainer(ctx context.Context, containerID string) error {
	_, err := r.service.client.Load().ContainerStart(
		ctx,
		containerID,
		client.ContainerStartOptions{},
	)
	return err
}

func (r *dockerRuntime) StopContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	seconds := int(timeout.Seconds())
	_, err := r.service.client.Load().ContainerStop(
		ctx,
		containerID,
		client.ContainerStopOptions{Timeout: &seconds},
	)
	return err
}

func (r *dockerRuntime) RestartContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	seconds := int(timeout.Seconds())
	_, err := r.service.client.Load().ContainerRestart(
		ctx,
		containerID,
		client.ContainerRestartOptions{Timeout: &seconds},
	)
	return err
}

func (r *dockerRuntime) RemoveContainer(
	ctx context.Context,
	containerID string,
	force bool,
) error {
	_, err := r.service.client.Load().ContainerRemove(
		ctx,
		containerID,
		client.ContainerRemoveOptions{
			Force:         force,
			RemoveVolumes: false,
			RemoveLinks:   false,
		},
	)
	return err
}
