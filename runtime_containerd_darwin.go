//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
)

const dockivaDarwinContainerdNamespace = "dockiva"

type darwinContainerdRuntime struct {
	socket      string
	namespace   string
	snapshotter string
	client      *containerd.Client
}

func newDarwinContainerdRuntime(
	socket string,
) (*darwinContainerdRuntime, error) {
	cli, err := containerd.New(
		socket,
		containerd.WithDefaultNamespace(
			dockivaDarwinContainerdNamespace,
		),
		containerd.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	runtime := &darwinContainerdRuntime{
		socket:    socket,
		namespace: dockivaDarwinContainerdNamespace,
		client:    cli,
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		6*time.Second,
	)
	defer cancel()

	if _, err := cli.Version(ctx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	if err := runtime.ensureNamespace(ctx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	snapshotter, err := selectDarwinGuestSnapshotter(
		ctx,
		cli,
	)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}

	runtime.snapshotter = snapshotter
	return runtime, nil
}

func (r *darwinContainerdRuntime) Close() error {
	if r.client == nil {
		return nil
	}

	err := r.client.Close()
	r.client = nil
	return err
}

func (r *darwinContainerdRuntime) ensureNamespace(
	ctx context.Context,
) error {
	err := r.client.NamespaceService().Create(
		ctx,
		r.namespace,
		map[string]string{
			"dockiva.io/managed": "true",
		},
	)

	if err != nil &&
		!errdefs.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func selectDarwinGuestSnapshotter(
	ctx context.Context,
	cli *containerd.Client,
) (string, error) {
	for _, snapshotter := range []string{
		"overlayfs",
		"native",
	} {
		if _, err :=
			cli.GetSnapshotterSupportedPlatforms(
				ctx,
				snapshotter,
			); err == nil {
			return snapshotter, nil
		}
	}

	return "", errors.New(
		"guest containerd has no usable overlayfs/native snapshotter",
	)
}

func (r *darwinContainerdRuntime) Info(
	ctx context.Context,
) ContainerRuntimeInfo {
	info := ContainerRuntimeInfo{
		Provider:     "containerd",
		DisplayName:  "containerd (native macOS guest)",
		Endpoint:     r.socket,
		Experimental: true,
		Capabilities: RuntimeCapabilities{
			Containers:      true,
			Lifecycle:       true,
			Stats:           false,
			Logs:            false,
			Terminal:        false,
			Images:          true,
			PullImages:      true,
			CreateContainer: true,
			Volumes:         false,
			Networks:        false,
			PortPublishing:  false,
			DNS:             false,
			Compose:         false,
		},
	}

	version, err := r.client.Version(ctx)
	if err != nil {
		info.Message = err.Error()
		return info
	}

	info.Connected = true
	info.Message = fmt.Sprintf(
		"containerd %s inside Dockiva's native Virtualization.framework guest · namespace %s · snapshotter %s",
		version.Version,
		r.namespace,
		r.snapshotter,
	)

	return info
}

func (r *darwinContainerdRuntime) ListContainers(
	ctx context.Context,
) ([]ContainerInfo, error) {
	containers, err := r.client.Containers(ctx)
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

		task, taskErr := container.Task(ctx, nil)

		if taskErr == nil {
			status, statusErr := task.Status(ctx)
			if statusErr == nil {
				item.State = string(status.Status)
				item.Status = fmt.Sprintf(
					"%s · pid %d",
					status.Status,
					task.Pid(),
				)
			}
		} else if !errdefs.IsNotFound(taskErr) {
			item.Status = taskErr.Error()
		}

		result = append(result, item)
	}

	return result, nil
}

func (r *darwinContainerdRuntime) PullRuntimeImage(
	ctx context.Context,
	reference string,
) (RuntimeOperationResult, error) {
	reference = normalizeDarwinImageReference(reference)

	leaseCtx, done, err := r.client.WithLease(ctx)
	if err != nil {
		return RuntimeOperationResult{}, err
	}
	defer func() {
		_ = done(leaseCtx)
	}()

	image, err := r.client.Pull(
		leaseCtx,
		reference,
		containerd.WithPullSnapshotter(
			r.snapshotter,
		),
		containerd.WithPullUnpack,
	)
	if err != nil {
		return RuntimeOperationResult{}, err
	}

	return RuntimeOperationResult{
		Message: "Pulled " + image.Name(),
		Detail: fmt.Sprintf(
			"native macOS guest · namespace=%s · snapshotter=%s",
			r.namespace,
			r.snapshotter,
		),
	}, nil
}

func (r *darwinContainerdRuntime) CreateRuntimeContainer(
	ctx context.Context,
	request RuntimeCreateContainerRequest,
) (ContainerInfo, error) {
	if len(request.PortMappings) > 0 {
		return ContainerInfo{}, errors.New(
			"native macOS guest port publishing is not enabled in Milestone 13",
		)
	}

	name := sanitizeDarwinContainerID(request.Name)
	if name == "" {
		return ContainerInfo{}, errors.New("invalid container name")
	}

	reference := normalizeDarwinImageReference(request.Image)

	image, err := r.client.GetImage(ctx, reference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return ContainerInfo{}, fmt.Errorf(
				"image %q is not present; pull it first",
				reference,
			)
		}
		return ContainerInfo{}, err
	}

	leaseCtx, done, err := r.client.WithLease(ctx)
	if err != nil {
		return ContainerInfo{}, err
	}
	defer func() {
		_ = done(leaseCtx)
	}()

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostname(name),
	}

	if len(request.Command) > 0 {
		specOpts = []oci.SpecOpts{
			oci.WithImageConfigArgs(
				image,
				request.Command,
			),
			oci.WithHostname(name),
		}
	}

	container, err := r.client.NewContainer(
		leaseCtx,
		name,
		containerd.WithSnapshotter(r.snapshotter),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(
			"dockiva-"+name,
			image,
		),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(
			map[string]string{
				"dockiva.io/managed":     "true",
				"dockiva.io/image":       reference,
				"dockiva.io/snapshotter": r.snapshotter,
			},
		),
	)
	if err != nil {
		return ContainerInfo{}, err
	}

	if request.AutoStart {
		if err := r.startLoadedContainer(
			leaseCtx,
			container,
		); err != nil {
			_ = container.Delete(
				leaseCtx,
				containerd.WithSnapshotCleanup,
			)
			return ContainerInfo{}, err
		}
	}

	state := "created"
	if request.AutoStart {
		state = "running"
	}

	return ContainerInfo{
		ID:      container.ID(),
		ShortID: shortenID(container.ID()),
		Name:    container.ID(),
		Image:   reference,
		State:   state,
		Ports:   []PortInfo{},
	}, nil
}

func (r *darwinContainerdRuntime) StartContainer(
	ctx context.Context,
	containerID string,
) error {
	container, err := r.client.LoadContainer(
		ctx,
		containerID,
	)
	if err != nil {
		return err
	}

	return r.startLoadedContainer(ctx, container)
}

func (r *darwinContainerdRuntime) startLoadedContainer(
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

		case containerd.Paused,
			containerd.Pausing:
			return task.Resume(ctx)

		case containerd.Created:
			exitC, waitErr := task.Wait(
				context.Background(),
			)
			if waitErr != nil {
				return waitErr
			}

			if err := task.Start(ctx); err != nil {
				return err
			}

			go func() {
				<-exitC
			}()
			return nil

		case containerd.Stopped:
			if _, err := task.Delete(ctx); err != nil &&
				!errdefs.IsNotFound(err) {
				return err
			}
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}

	task, err = container.NewTask(ctx, cio.NullIO)
	if err != nil {
		return err
	}

	exitC, err := task.Wait(context.Background())
	if err != nil {
		_, _ = task.Delete(
			ctx,
			containerd.WithProcessKill,
		)
		return err
	}

	if err := task.Start(ctx); err != nil {
		_, _ = task.Delete(
			ctx,
			containerd.WithProcessKill,
		)
		return err
	}

	go func() {
		<-exitC
	}()

	return nil
}

func (r *darwinContainerdRuntime) StopContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	container, err := r.client.LoadContainer(
		ctx,
		containerID,
	)
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

	if status.Status == containerd.Paused ||
		status.Status == containerd.Pausing {
		if err := task.Resume(ctx); err != nil {
			return err
		}
	}

	exitC, waitErr := task.Wait(context.Background())
	if waitErr != nil && !errdefs.IsNotFound(waitErr) {
		return waitErr
	}

	if err := task.Kill(
		ctx,
		syscall.SIGTERM,
	); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	if exitC != nil {
		timer := time.NewTimer(timeout)

		select {
		case <-exitC:
			timer.Stop()
		case <-timer.C:
			if err := task.Kill(
				ctx,
				syscall.SIGKILL,
			); err != nil && !errdefs.IsNotFound(err) {
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

func (r *darwinContainerdRuntime) RestartContainer(
	ctx context.Context,
	containerID string,
	timeout time.Duration,
) error {
	if err := r.StopContainer(
		ctx,
		containerID,
		timeout,
	); err != nil {
		return err
	}

	return r.StartContainer(ctx, containerID)
}

func (r *darwinContainerdRuntime) RemoveContainer(
	ctx context.Context,
	containerID string,
	force bool,
) error {
	container, err := r.client.LoadContainer(
		ctx,
		containerID,
	)
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
			_ = task.Kill(
				ctx,
				syscall.SIGKILL,
			)
		}

		_, _ = task.Delete(
			ctx,
			containerd.WithProcessKill,
		)
	}

	return container.Delete(
		ctx,
		containerd.WithSnapshotCleanup,
	)
}

func normalizeDarwinImageReference(reference string) string {
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

func sanitizeDarwinContainerID(value string) string {
	value = strings.TrimSpace(value)

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.', r == '_', r == '-':
			builder.WriteRune(r)
		case r == ' ':
			builder.WriteRune('-')
		}
	}

	return strings.Trim(builder.String(), ".-_")
}

func platformRuntimeCandidates() []RuntimeCandidateInfo {
	stateDir, err := dockivaVMMStateDir()
	if err != nil {
		stateDir = ""
	}

	socket := filepath.Join(stateDir, "containerd.sock")
	available := fileExists(socket)

	message := "Native macOS guest is not running."

	if available {
		message =
			"containerd is exposed from the native Virtualization.framework VM over virtio-vsock."
	}

	return []RuntimeCandidateInfo{
		{
			Provider:    "docker",
			DisplayName: "External Docker Engine",
			Available:   true,
			Detected:    true,
			Selectable:  true,
			Message:     "Docker Desktop or another external Docker endpoint.",
		},
		{
			Provider:     "containerd",
			DisplayName:  "containerd (native macOS guest)",
			Available:    available,
			Detected:     available,
			Selectable:   available,
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
			"runtime %q is not available on macOS",
			provider,
		)
	}

	stateDir, err := dockivaVMMStateDir()
	if err != nil {
		return nil, err
	}

	socket := filepath.Join(
		stateDir,
		"containerd.sock",
	)

	if !fileExists(socket) {
		return nil, errors.New(
			"native macOS guest containerd proxy is not running",
		)
	}

	return newDarwinContainerdRuntime(socket)
}

func dockivaDarwinGuestAssetsReady() bool {
	guestDir, err := dockivaGuestDir()
	if err != nil {
		return false
	}

	return fileExists(
		filepath.Join(
			guestDir,
			"vmlinux",
		),
	) &&
		fileExists(
			filepath.Join(
				guestDir,
				"rootfs.ext4",
			),
		)
}

func _unusedDarwinImportGuard() {
	_ = os.ErrNotExist
}
