//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	dockivaContainerdNamespace = "dockiva"
	dockivaStateRoot           = "/var/lib/dockiva"

	labelManaged      = "dockiva.io/managed"
	labelImage        = "dockiva.io/image"
	labelSnapshotter  = "dockiva.io/snapshotter"
	labelNetworkMode  = "dockiva.io/network-mode"
	labelNetNSPath    = "dockiva.io/netns-path"
	labelPortMappings = "dockiva.io/port-mappings"

	networkModeCNI = "cni"
)

var validContainerdVolumeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type containerdRuntime struct {
	socket      string
	namespace   string
	snapshotter string
	rootless    bool

	mu     sync.Mutex
	client *containerd.Client
}

func newContainerdRuntime(socket string) (*containerdRuntime, error) {
	socket = strings.TrimSpace(socket)
	if socket == "" {
		return nil, errors.New("containerd socket is empty")
	}

	cli, err := containerd.New(
		socket,
		containerd.WithDefaultNamespace(dockivaContainerdNamespace),
		containerd.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	r := &containerdRuntime{
		socket:    socket,
		namespace: dockivaContainerdNamespace,
		rootless:  isRootlessContainerdSocket(socket),
		client:    cli,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	if _, err := cli.Version(ctx); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("connect containerd: %w", err)
	}

	if err := r.ensureNamespace(ctx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	snapshotter, err := selectContainerdSnapshotter(ctx, cli)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}

	r.snapshotter = snapshotter
	return r, nil
}

func isRootlessContainerdSocket(socket string) bool {
	socket = filepath.Clean(socket)
	return strings.Contains(socket, "/containerd-rootless/") ||
		strings.Contains(socket, "/proc/")
}

func (r *containerdRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client == nil {
		return nil
	}

	err := r.client.Close()
	r.client = nil
	return err
}

func (r *containerdRuntime) clientOrError() (*containerd.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client == nil {
		return nil, errors.New("containerd client is closed")
	}

	return r.client, nil
}

func (r *containerdRuntime) ensureNamespace(ctx context.Context) error {
	cli, err := r.clientOrError()
	if err != nil {
		return err
	}

	err = cli.NamespaceService().Create(
		ctx,
		r.namespace,
		map[string]string{labelManaged: "true"},
	)

	if err != nil && !errdefs.IsAlreadyExists(err) {
		return fmt.Errorf("create containerd namespace %q: %w", r.namespace, err)
	}

	return nil
}

func selectContainerdSnapshotter(
	ctx context.Context,
	cli *containerd.Client,
) (string, error) {
	preferred := strings.TrimSpace(os.Getenv("DOCKIVA_CONTAINERD_SNAPSHOTTER"))

	candidates := []string{}
	if preferred != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "overlayfs", "native")

	seen := map[string]struct{}{}

	for _, snapshotter := range candidates {
		if _, ok := seen[snapshotter]; ok {
			continue
		}
		seen[snapshotter] = struct{}{}

		if _, err := cli.GetSnapshotterSupportedPlatforms(ctx, snapshotter); err == nil {
			return snapshotter, nil
		}
	}

	return "", errors.New(
		"no supported containerd snapshotter found; tried DOCKIVA_CONTAINERD_SNAPSHOTTER, overlayfs, and native",
	)
}

func (r *containerdRuntime) cniReady(ctx context.Context) (bool, string) {
	if r.rootless {
		return false, "CNI bridge is disabled for rootless containerd in Milestone 12"
	}

	status := networkHelperStatus(ctx)
	return status.Ready, status.Message
}

func (r *containerdRuntime) Info(ctx context.Context) ContainerRuntimeInfo {
	cniReady, cniMessage := r.cniReady(ctx)

	info := ContainerRuntimeInfo{
		Provider:     "containerd",
		DisplayName:  "containerd",
		Endpoint:     r.socket,
		Experimental: true,
		Capabilities: RuntimeCapabilities{
			Containers:      true,
			Lifecycle:       true,
			Stats:           true,
			Logs:            true,
			Terminal:        true,
			Images:          true,
			PullImages:      true,
			CreateContainer: true,
			Volumes:         true,
			Networks:        cniReady,
			PortPublishing:  cniReady,
			DNS:             cniReady,
			Compose:         false,
		},
	}

	cli, err := r.clientOrError()
	if err != nil {
		info.Message = err.Error()
		return info
	}

	version, err := cli.Version(ctx)
	if err != nil {
		info.Message = err.Error()
		return info
	}

	info.Connected = true
	info.Message = fmt.Sprintf(
		"containerd %s · namespace %s · snapshotter %s · %s",
		version.Version,
		r.namespace,
		r.snapshotter,
		cniMessage,
	)

	return info
}

// ListRuntimeImages exposes the images in Dockiva's dedicated containerd
// namespace. They are not Docker images and must never be read from a Docker
// endpoint when Native is selected.
func (r *containerdRuntime) ListRuntimeImages(ctx context.Context) ([]ImageInfo, error) {
	cli, err := r.clientOrError()
	if err != nil {
		return nil, err
	}

	items, err := cli.ListImages(ctx)
	if err != nil {
		return nil, err
	}

	images := make([]ImageInfo, 0, len(items))
	for _, item := range items {
		target := item.Target()
		id := target.Digest.String()
		if id == "" {
			id = item.Name()
		}
		images = append(images, ImageInfo{
			ID:         id,
			ShortID:    shortenID(id),
			Tags:       []string{item.Name()},
			Size:       target.Size,
			Created:    0,
			Containers: 0,
		})
	}

	sort.Slice(images, func(i, j int) bool { return images[i].Tags[0] < images[j].Tags[0] })
	return images, nil
}

func (r *containerdRuntime) ListRuntimeNetworks(ctx context.Context) ([]NetworkInfo, error) {
	ready, message := r.cniReady(ctx)
	if !ready {
		return nil, errors.New(message)
	}
	return []NetworkInfo{{
		ID:         "dockiva-net",
		ShortID:    "dockiva-net",
		Name:       "dockiva-net",
		Driver:     "cni-bridge",
		Scope:      "local",
		Attachable: true,
	}}, nil
}

func (r *containerdRuntime) ListRuntimeVolumes(_ context.Context) ([]VolumeInfo, error) {
	items, err := networkHelperListRuntimeVolumes()
	if err != nil {
		// The Native runtime's volume store is deliberately root-owned. Never
		// fall back to reading it from the desktop process: that produces a
		// misleading permission error and bypasses the helper's authorization.
		return nil, fmt.Errorf("list Dockiva volumes through the privileged helper: %w", err)
	}
	return items, nil
}

func (r *containerdRuntime) RemoveRuntimeVolume(_ context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("volume name is required")
	}
	if !validContainerdVolumeID.MatchString(name) {
		return errors.New("invalid volume name")
	}

	if err := networkHelperRemoveRuntimeVolume(context.Background(), name); err != nil {
		return fmt.Errorf("remove Dockiva volume through the privileged helper: %w", err)
	}
	return nil
}

func networkHelperListRuntimeVolumes() ([]VolumeInfo, error) {
	items, err := networkHelperListVolumes(context.Background())
	if err != nil {
		return nil, err
	}

	result := make([]VolumeInfo, 0, len(items))
	for _, item := range items {
		result = append(result, VolumeInfo{
			Name:       item.Name,
			Driver:     item.Driver,
			Scope:      item.Scope,
			Mountpoint: item.Mountpoint,
			CreatedAt:  item.CreatedAt,
			Labels:     item.Labels,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func networkHelperRemoveRuntimeVolume(ctx context.Context, name string) error {
	return networkHelperRemoveVolume(ctx, name)
}

func (r *containerdRuntime) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	cli, err := r.clientOrError()
	if err != nil {
		return nil, err
	}

	containers, err := cli.Containers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ContainerInfo, 0, len(containers))

	for _, container := range containers {
		item := ContainerInfo{
			ID:      container.ID(),
			ShortID: shortenID(container.ID()),
			Name:    container.ID(),
			State:   "created",
			Status:  "metadata only",
			Ports:   []PortInfo{},
		}

		if image, imageErr := container.Image(ctx); imageErr == nil {
			item.Image = image.Name()
		}

		labels, _ := container.Labels(ctx)

		portMappings := decodePortMappings(labels[labelPortMappings])
		item.Ports = runtimePortInfos(portMappings)

		if labels[labelNetworkMode] == networkModeCNI {
			networkCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)

			if state, inspectErr := networkHelperInspect(
				networkCtx,
				container.ID(),
			); inspectErr == nil {
				item.IPAddress = state.IPAddress
			}

			cancel()
		}

		task, taskErr := container.Task(ctx, nil)

		if taskErr == nil {
			status, statusErr := task.Status(ctx)
			if statusErr == nil {
				item.State = string(status.Status)
				item.Status = fmt.Sprintf("%s · pid %d", status.Status, task.Pid())

				if item.IPAddress != "" {
					item.Status += " · " + item.IPAddress
				}
			}
		} else if !errdefs.IsNotFound(taskErr) {
			item.Status = taskErr.Error()
		}

		result = append(result, item)
	}

	return result, nil
}

func runtimePortInfos(mappings []RuntimePortMapping) []PortInfo {
	result := make([]PortInfo, 0, len(mappings))

	for _, mapping := range mappings {
		if mapping.HostPort < 1 || mapping.HostPort > 65535 ||
			mapping.ContainerPort < 1 || mapping.ContainerPort > 65535 {
			continue
		}

		publicPort := uint16(mapping.HostPort)
		privatePort := uint16(mapping.ContainerPort)

		result = append(result, PortInfo{
			PrivatePort: privatePort,
			PublicPort:  publicPort,
			Type:        mapping.Protocol,
			Display: fmt.Sprintf(
				"%d:%d/%s",
				publicPort,
				privatePort,
				mapping.Protocol,
			),
			URL: localhostURL(publicPort, privatePort, mapping.Protocol),
		})
	}

	return result
}

func decodePortMappings(raw string) []RuntimePortMapping {
	if strings.TrimSpace(raw) == "" {
		return []RuntimePortMapping{}
	}

	var mappings []RuntimePortMapping
	if err := json.Unmarshal([]byte(raw), &mappings); err != nil {
		return []RuntimePortMapping{}
	}

	return mappings
}

func encodePortMappings(mappings []RuntimePortMapping) (string, error) {
	data, err := json.Marshal(mappings)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeRuntimePortMappings(
	mappings []RuntimePortMapping,
) ([]RuntimePortMapping, error) {
	seen := map[int32]struct{}{}
	result := make([]RuntimePortMapping, 0, len(mappings))

	for _, mapping := range mappings {
		if mapping.HostPort == 0 && mapping.ContainerPort == 0 {
			continue
		}

		if mapping.HostPort < 1024 || mapping.HostPort > 65535 {
			return nil, fmt.Errorf(
				"host port %d is invalid; direct containerd publishing is restricted to localhost ports 1024-65535",
				mapping.HostPort,
			)
		}

		if mapping.ContainerPort < 1 || mapping.ContainerPort > 65535 {
			return nil, fmt.Errorf("container port %d is invalid", mapping.ContainerPort)
		}

		protocol := strings.ToLower(strings.TrimSpace(mapping.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" {
			return nil, errors.New(
				"Milestone 12 direct containerd port publishing supports TCP only",
			)
		}

		if _, exists := seen[mapping.HostPort]; exists {
			return nil, fmt.Errorf("host port %d is duplicated", mapping.HostPort)
		}
		seen[mapping.HostPort] = struct{}{}

		result = append(result, RuntimePortMapping{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			Protocol:      "tcp",
			HostIP:        "127.0.0.1",
		})
	}

	return result, nil
}

func (r *containerdRuntime) PullRuntimeImage(
	ctx context.Context,
	reference string,
) (RuntimeOperationResult, error) {
	cli, err := r.clientOrError()
	if err != nil {
		return RuntimeOperationResult{}, err
	}

	reference = normalizeContainerdImageReference(reference)

	leaseCtx, done, err := cli.WithLease(ctx)
	if err != nil {
		return RuntimeOperationResult{}, err
	}
	defer func() {
		_ = done(leaseCtx)
	}()

	image, err := cli.Pull(
		leaseCtx,
		reference,
		containerd.WithPullSnapshotter(r.snapshotter),
		containerd.WithPullUnpack,
	)
	if err != nil {
		return RuntimeOperationResult{}, err
	}

	size, _ := image.Size(leaseCtx)

	return RuntimeOperationResult{
		Message: fmt.Sprintf("Pulled %s", image.Name()),
		Detail: fmt.Sprintf(
			"namespace=%s snapshotter=%s size=%d",
			r.namespace,
			r.snapshotter,
			size,
		),
	}, nil
}

func (r *containerdRuntime) CreateRuntimeContainer(
	ctx context.Context,
	request RuntimeCreateContainerRequest,
) (ContainerInfo, error) {
	cli, err := r.clientOrError()
	if err != nil {
		return ContainerInfo{}, err
	}

	name := sanitizeContainerdID(request.Name)
	if name == "" {
		return ContainerInfo{}, errors.New(
			"container name must contain letters, digits, dot, underscore, or dash",
		)
	}

	reference := normalizeContainerdImageReference(request.Image)

	image, err := cli.GetImage(ctx, reference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return ContainerInfo{}, fmt.Errorf(
				"image %q is not present in the dockiva namespace; pull it first",
				reference,
			)
		}
		return ContainerInfo{}, err
	}

	portMappings, err := normalizeRuntimePortMappings(request.PortMappings)
	if err != nil {
		return ContainerInfo{}, err
	}

	labels := map[string]string{
		labelManaged:     "true",
		labelImage:       reference,
		labelSnapshotter: r.snapshotter,
	}

	// containerd's overlay snapshot mounts are performed in the caller's mount
	// namespace. A desktop user can safely talk to containerd but cannot mount
	// its root-owned snapshots, so delegate container creation to the privileged
	// helper when possible.
	if !r.rootless && len(portMappings) == 0 {
		if err := networkHelperCreateContainer(ctx, NetworkHelperContainerCreateRequest{
			ID: name, Image: reference, Command: request.Command, Snapshotter: r.snapshotter, AutoStart: request.AutoStart,
		}); err != nil {
			// Do not fall through to the desktop process. It cannot enter
			// containerd's root-owned overlay snapshot and would mask a helper
			// setup error as ".../snapshots/.../fs: permission denied".
			return ContainerInfo{}, fmt.Errorf("create container through the privileged Dockiva helper: %w", err)
		}
		if request.AutoStart {
			items, listErr := r.ListContainers(ctx)
			if listErr == nil {
				for _, item := range items {
					if item.ID == name {
						return item, nil
					}
				}
			}
			return ContainerInfo{ID: name, ShortID: shortenID(name), Name: name, Image: reference, State: "running"}, nil
		}

		return ContainerInfo{
			ID:      name,
			ShortID: shortenID(name),
			Name:    name,
			Image:   reference,
			State:   "created",
			Ports:   runtimePortInfos(portMappings),
		}, nil
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostname(name),
	}

	if len(request.Command) > 0 {
		specOpts = []oci.SpecOpts{
			oci.WithImageConfigArgs(image, request.Command),
			oci.WithHostname(name),
		}
	}

	var networkState NetworkHelperState
	networkConfigured := false

	if len(portMappings) > 0 {
		if r.rootless {
			return ContainerInfo{}, errors.New(
				"CNI bridge/port publishing is not enabled for rootless containerd in Milestone 12",
			)
		}

		networkCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		status := networkHelperStatus(networkCtx)

		if !status.Ready {
			cancel()
			return ContainerInfo{}, fmt.Errorf(
				"container networking is not ready: %s; run scripts/install-containerd-networking.sh",
				status.Message,
			)
		}

		networkState, err = networkHelperSetup(
			networkCtx,
			name,
			portMappings,
		)
		cancel()

		if err != nil {
			return ContainerInfo{}, err
		}

		networkConfigured = true

		encodedPorts, err := encodePortMappings(portMappings)
		if err != nil {
			_ = networkHelperRemove(context.Background(), name)
			return ContainerInfo{}, err
		}

		labels[labelNetworkMode] = networkModeCNI
		labels[labelNetNSPath] = networkState.NetNSPath
		labels[labelPortMappings] = encodedPorts

		specOpts = append(
			specOpts,
			oci.WithLinuxNamespace(specs.LinuxNamespace{
				Type: specs.NetworkNamespace,
				Path: networkState.NetNSPath,
			}),
			oci.WithHostResolvconf,
		)
	}

	leaseCtx, done, err := cli.WithLease(ctx)
	if err != nil {
		if networkConfigured {
			_ = networkHelperRemove(context.Background(), name)
		}
		return ContainerInfo{}, err
	}
	defer func() {
		_ = done(leaseCtx)
	}()

	snapshotKey := "dockiva-" + name

	container, err := cli.NewContainer(
		leaseCtx,
		name,
		containerd.WithSnapshotter(r.snapshotter),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(snapshotKey, image),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(labels),
	)
	if err != nil {
		if networkConfigured {
			_ = networkHelperRemove(context.Background(), name)
		}
		return ContainerInfo{}, err
	}

	if request.AutoStart {
		if err := r.startLoadedContainer(leaseCtx, container); err != nil {
			_ = container.Delete(leaseCtx, containerd.WithSnapshotCleanup)

			if networkConfigured {
				_ = networkHelperRemove(context.Background(), name)
			}

			return ContainerInfo{}, err
		}
	}

	items, listErr := r.ListContainers(ctx)
	if listErr == nil {
		for _, item := range items {
			if item.ID == container.ID() {
				return item, nil
			}
		}
	}

	state := "created"
	if request.AutoStart {
		state = "running"
	}

	return ContainerInfo{
		ID:        container.ID(),
		ShortID:   shortenID(container.ID()),
		Name:      container.ID(),
		Image:     reference,
		IPAddress: networkState.IPAddress,
		State:     state,
		Ports:     runtimePortInfos(portMappings),
	}, nil
}

func (r *containerdRuntime) StartContainer(
	ctx context.Context,
	containerID string,
) error {
	cli, err := r.clientOrError()
	if err != nil {
		return err
	}

	container, err := cli.LoadContainer(ctx, containerID)
	if err != nil {
		return err
	}

	return r.startLoadedContainer(ctx, container)
}

func (r *containerdRuntime) startLoadedContainer(
	ctx context.Context,
	container containerd.Container,
) error {
	task, err := container.Task(ctx, nil)

	if err == nil {
		status, statusErr := task.Status(ctx)
		if statusErr != nil {
			return statusErr
		}

		switch status.Status {
		case containerd.Running:
			return nil

		case containerd.Paused, containerd.Pausing:
			return task.Resume(ctx)

		case containerd.Created:
			exitC, waitErr := task.Wait(context.Background())
			if waitErr != nil {
				return waitErr
			}

			if err := task.Start(ctx); err != nil {
				return err
			}

			go drainContainerdExit(exitC)
			return nil

		case containerd.Stopped:
			if _, err := task.Delete(ctx); err != nil &&
				!errdefs.IsNotFound(err) {
				return err
			}

		default:
			if _, err := task.Delete(
				ctx,
				containerd.WithProcessKill,
			); err != nil && !errdefs.IsNotFound(err) {
				return err
			}
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}

	task, err = container.NewTask(ctx, r.containerTaskIO(container.ID()))
	if err != nil {
		return err
	}

	exitC, err := task.Wait(context.Background())
	if err != nil {
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
		return err
	}

	if err := task.Start(ctx); err != nil {
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
		return err
	}

	go drainContainerdExit(exitC)
	return nil
}

func drainContainerdExit(exitC <-chan containerd.ExitStatus) {
	<-exitC
}

func (r *containerdRuntime) StopContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	cli, err := r.clientOrError()
	if err != nil {
		return err
	}

	container, err := cli.LoadContainer(ctx, containerID)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		_, err := task.Delete(ctx)
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err

	case containerd.Paused, containerd.Pausing:
		if err := task.Resume(ctx); err != nil {
			return err
		}
	}

	exitC, err := task.Wait(context.Background())
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	if err := task.Kill(ctx, syscall.SIGTERM); err != nil &&
		!errdefs.IsNotFound(err) {
		return err
	}

	if exitC != nil {
		timer := time.NewTimer(timeout)

		select {
		case <-exitC:
			timer.Stop()

		case <-timer.C:
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil &&
				!errdefs.IsNotFound(err) {
				return err
			}

			select {
			case <-exitC:
			case <-ctx.Done():
				return ctx.Err()
			}

		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}

	if _, err := task.Delete(
		ctx,
		containerd.WithProcessKill,
	); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *containerdRuntime) RestartContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	if err := r.StopContainer(ctx, containerID, timeout); err != nil {
		return err
	}

	return r.StartContainer(ctx, containerID)
}

func (r *containerdRuntime) RemoveContainer(
	ctx context.Context,
	containerID string,
	force bool,
) error {
	cli, err := r.clientOrError()
	if err != nil {
		return err
	}

	container, err := cli.LoadContainer(ctx, containerID)
	if err != nil {
		return err
	}

	task, taskErr := container.Task(ctx, nil)

	if taskErr == nil {
		status, statusErr := task.Status(ctx)
		if statusErr != nil {
			return statusErr
		}

		running := status.Status == containerd.Running ||
			status.Status == containerd.Paused ||
			status.Status == containerd.Pausing

		if running && !force {
			return errors.New(
				"container task is running; stop it first or force removal",
			)
		}

		if running {
			exitC, waitErr := task.Wait(context.Background())
			if waitErr != nil && !errdefs.IsNotFound(waitErr) {
				return waitErr
			}

			if err := task.Kill(ctx, syscall.SIGKILL); err != nil &&
				!errdefs.IsNotFound(err) {
				return err
			}

			if exitC != nil {
				select {
				case <-exitC:
				case <-time.After(5 * time.Second):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		if _, err := task.Delete(
			ctx,
			containerd.WithProcessKill,
		); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	} else if !errdefs.IsNotFound(taskErr) {
		return taskErr
	}

	labels, _ := container.Labels(ctx)

	if labels[labelNetworkMode] == networkModeCNI {
		networkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		removeErr := networkHelperRemove(networkCtx, containerID)
		cancel()

		if removeErr != nil {
			return fmt.Errorf(
				"remove container network before deleting metadata: %w",
				removeErr,
			)
		}
	}

	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}

func normalizeContainerdImageReference(reference string) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return reference
	}

	if !strings.Contains(reference, "/") {
		reference = "docker.io/library/" + reference
	} else {
		first := strings.SplitN(reference, "/", 2)[0]

		if first != "localhost" &&
			!strings.Contains(first, ".") &&
			!strings.Contains(first, ":") {
			reference = "docker.io/" + reference
		}
	}

	if strings.Contains(reference, "@") {
		return reference
	}

	lastSlash := strings.LastIndex(reference, "/")
	lastColon := strings.LastIndex(reference, ":")

	if lastColon <= lastSlash {
		reference += ":latest"
	}

	return reference
}

func platformRuntimeCandidates() []RuntimeCandidateInfo {
	socket, reachable, socketMessage := detectContainerdSocket()

	message := socketMessage + " · namespace " + dockivaContainerdNamespace

	if reachable {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			1500*time.Millisecond,
		)
		helper := networkHelperStatus(ctx)
		cancel()

		if helper.Ready {
			message += " · CNI bridge/localhost port publishing ready"
		} else {
			message += " · CNI helper not ready: " + helper.Message
		}
	}

	return []RuntimeCandidateInfo{
		{
			Provider:    "docker",
			DisplayName: "Docker Engine (Moby API)",
			Available:   true,
			Detected:    true,
			Selectable:  true,
			Message:     "Full Docker feature set including Compose, ports, terminal, logs, images, volumes, and networks.",
		},
		{
			Provider:     "containerd",
			DisplayName:  "containerd (direct)",
			Available:    reachable,
			Detected:     socket != "",
			Selectable:   reachable,
			Experimental: true,
			Endpoint:     socket,
			Message:      message,
		},
	}
}

func createPlatformRuntime(
	service *DockerService,
	provider string,
) (ContainerRuntime, error) {
	if provider != "containerd" {
		return nil, fmt.Errorf(
			"runtime %q is not available on Linux",
			provider,
		)
	}

	socket, reachable, message := detectContainerdSocket()
	if socket == "" || !reachable {
		return nil, errors.New(message)
	}

	return newContainerdRuntime(socket)
}

func detectContainerdSocket() (string, bool, string) {
	var candidates []string

	if runtimeDir := strings.TrimSpace(
		os.Getenv("XDG_RUNTIME_DIR"),
	); runtimeDir != "" {
		childPIDFile := filepath.Join(
			runtimeDir,
			"containerd-rootless",
			"child_pid",
		)

		if data, err := os.ReadFile(childPIDFile); err == nil {
			pid := strings.TrimSpace(string(data))

			if pid != "" {
				candidates = append(
					candidates,
					filepath.Join(
						"/proc",
						pid,
						"root",
						"run",
						"containerd",
						"containerd.sock",
					),
				)
			}
		}

		candidates = append(
			candidates,
			filepath.Join(
				runtimeDir,
				"containerd",
				"containerd.sock",
			),
		)
	}

	candidates = append(
		candidates,
		"/run/containerd/containerd.sock",
		"/var/run/containerd/containerd.sock",
	)

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}

		conn, dialErr := net.DialTimeout(
			"unix",
			candidate,
			800*time.Millisecond,
		)

		if dialErr != nil {
			return candidate,
				false,
				"containerd socket exists but the current user cannot connect: " +
					dialErr.Error()
		}

		_ = conn.Close()
		return candidate, true, "containerd socket is reachable"
	}

	ctrInfo := ""
	if ctrPath, err := exec.LookPath("ctr"); err == nil {
		ctrInfo = " (ctr found at " + ctrPath + ")"
	}

	return "", false, "no accessible containerd socket detected" + ctrInfo
}
