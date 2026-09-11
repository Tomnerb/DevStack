//go:build darwin

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	devstackVMMGuestPort       = 10250
	devstackVMMDockerGuestPort = 10251
)

type darwinEngineBackend struct{}

func newPlatformEngineBackend() platformEngineBackend {
	return darwinEngineBackend{}
}

func (darwinEngineBackend) Name() string {
	return "vz"
}

func nativeRuntimeProvider() string {
	return "docker"
}

func (darwinEngineBackend) Options() []string {
	return []string{}
}

func (backend darwinEngineBackend) Status(
	service *DockerService,
	_ string,
) EngineStatus {
	status := EngineStatus{
		Backend:   "vz",
		Platform:  "darwin",
		Supported: true,
		Message:   "Native macOS Virtualization.framework backend.",
	}

	helper, err := devstackVMMPath()
	if err != nil {
		status.Message = err.Error()
		return status
	}

	status.HelperInstalled = true

	guestDir, err := devstackGuestDir()
	if err != nil {
		status.Message = err.Error()
		return status
	}

	kernel := filepath.Join(guestDir, "vmlinux")
	disk := filepath.Join(guestDir, "rootfs.ext4")
	bundledGuest := bundledGuestDir()
	bundledAssetsAvailable := bundledGuest != "" &&
		fileExists(filepath.Join(bundledGuest, "vmlinux")) &&
		fileExists(filepath.Join(bundledGuest, "rootfs.ext4"))

	if fileExists(kernel) && fileExists(disk) || bundledAssetsAvailable {
		status.EngineInstalled = true
	} else {
		status.Message = fmt.Sprintf(
			"Native helper is installed, but guest assets are missing under %s",
			guestDir,
		)
		return status
	}

	stateDir, _ := devstackVMMStateDir()
	proxySocket := filepath.Join(stateDir, "docker.sock")

	output, err := runCommand(
		5*time.Second,
		"",
		helper,
		"status",
		"--state-dir",
		stateDir,
	)
	if err == nil && strings.Contains(output, `"running":true`) {
		status.Running = true
		status.Endpoint = proxySocket
		status.Message = "Native Virtualization.framework VM with Docker-compatible API is running."
	} else {
		status.Message = "Native VM is prepared but not running."
	}

	return status
}

func (backend darwinEngineBackend) Start(
	service *DockerService,
	_ string,
) (EngineActionResult, error) {
	helper, err := devstackVMMPath()
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	if err := stageBundledGuestAssets(); err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, fmt.Errorf("stage bundled macOS guest assets: %w", err)
	}

	guestDir, err := devstackGuestDir()
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	kernel := filepath.Join(guestDir, "vmlinux")
	disk := filepath.Join(guestDir, "rootfs.ext4")

	if !fileExists(kernel) || !fileExists(disk) {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, fmt.Errorf(
			"macOS guest assets are missing; copy vmlinux and rootfs.ext4 into %s (see scripts/build-macos-guest-assets-linux.sh)",
			guestDir,
		)
	}

	stateDir, err := devstackVMMStateDir()
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	if status := backend.Status(service, ""); status.Running {
		if err := service.ConfigureNativeDockerEndpoint("unix://" + status.Endpoint); err != nil {
			return EngineActionResult{Status: status}, err
		}
		if err := service.SelectContainerRuntime("docker"); err != nil {
			return EngineActionResult{Status: status}, err
		}
		return EngineActionResult{Status: status}, nil
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	logDir, err := os.UserHomeDir()
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	logDir = filepath.Join(logDir, "Library", "Logs", "DevStack")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	logFile, err := os.OpenFile(
		filepath.Join(logDir, "vmm.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}
	defer logFile.Close()

	cpuCount := defaultDarwinVMCPUCount()
	memoryMiB := defaultDarwinVMMemoryMiB()

	cmd := exec.Command(
		helper,
		"serve",
		"--kernel",
		kernel,
		"--disk",
		disk,
		"--state-dir",
		stateDir,
		"--cpu",
		strconv.Itoa(cpuCount),
		"--memory-mib",
		strconv.Itoa(memoryMiB),
		"--guest-port",
		strconv.Itoa(devstackVMMGuestPort),
		"--docker-guest-port",
		strconv.Itoa(devstackVMMDockerGuestPort),
	)

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	_ = cmd.Process.Release()

	proxySocket := filepath.Join(stateDir, "docker.sock")

	var lastErr error
	for attempt := 0; attempt < 60; attempt++ {
		if fileExists(proxySocket) {
			if err := service.ConfigureNativeDockerEndpoint("unix://" + proxySocket); err == nil {
				if err := service.SelectContainerRuntime("docker"); err != nil {
					lastErr = err
					continue
				}
				status := backend.Status(service, "")
				status.Running = true
				status.Endpoint = proxySocket

				return EngineActionResult{
					Status: status,
					Output: fmt.Sprintf(
						"Native macOS VM started with %d CPU(s), %d MiB RAM.",
						cpuCount,
						memoryMiB,
					),
				}, nil
			} else {
				lastErr = err
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	return EngineActionResult{
		Status: backend.Status(service, ""),
	}, fmt.Errorf(
		"native VM started but containerd proxy did not become ready: %w",
		lastErr,
	)
}

func (backend darwinEngineBackend) Stop(
	service *DockerService,
	_ string,
) (EngineActionResult, error) {
	helper, err := devstackVMMPath()
	if err != nil {
		return EngineActionResult{
			Status: backend.Status(service, ""),
		}, err
	}

	stateDir, _ := devstackVMMStateDir()

	output, err := runCommand(
		15*time.Second,
		"",
		helper,
		"stop",
		"--state-dir",
		stateDir,
	)

	if err == nil {
		service.useOfflineNativeRuntime(backend.Status(service, ""))
	}

	return EngineActionResult{
		Status: backend.Status(service, ""),
		Output: output,
	}, err
}

func (backend darwinEngineBackend) Delete(
	service *DockerService,
	_ string,
) (EngineActionResult, error) {
	status := backend.Status(service, "")

	if status.Running {
		return EngineActionResult{
			Status: status,
		}, errors.New("stop the native VM before deleting its guest disk")
	}

	guestDir, err := devstackGuestDir()
	if err != nil {
		return EngineActionResult{Status: status}, err
	}

	disk := filepath.Join(guestDir, "rootfs.ext4")
	if !fileExists(disk) {
		return EngineActionResult{Status: status}, nil
	}

	if err := os.Remove(disk); err != nil {
		return EngineActionResult{Status: status}, err
	}

	return EngineActionResult{
		Status: backend.Status(service, ""),
		Output: "Removed DevStack macOS guest rootfs.ext4. Kernel asset was preserved.",
	}, nil
}

func (backend darwinEngineBackend) Provision(
	service *DockerService,
	option string,
) (EngineActionResult, error) {
	return EngineActionResult{
		Status: backend.Status(service, option),
	}, errors.New(
		"macOS native guest assets are prepared outside the running app; use scripts/build-macos-guest-assets-linux.sh and scripts/install-macos-native-assets.sh",
	)
}

func devstackVMMPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("DEVSTACK_VMM_PATH")); explicit != "" {
		if fileExists(explicit) {
			return explicit, nil
		}
	}

	executable, _ := os.Executable()
	if executable != "" {
		candidates := []string{
			filepath.Join(
				filepath.Dir(executable),
				"..",
				"Resources",
				"devstack-vmm",
			),
			filepath.Join(
				filepath.Dir(executable),
				"devstack-vmm",
			),
		}

		for _, candidate := range candidates {
			candidate = filepath.Clean(candidate)
			if fileExists(candidate) {
				return candidate, nil
			}
		}
	}

	home, _ := os.UserHomeDir()

	candidates := []string{
		"/usr/local/libexec/devstack-vmm",
		"/opt/homebrew/libexec/devstack-vmm",
		filepath.Join(
			home,
			".local",
			"libexec",
			"devstack-vmm",
		),
		filepath.Join(
			"native",
			"macos",
			"DevStackVMM",
			".build",
			"release",
			"devstack-vmm",
		),
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	return "", errors.New(
		"native macOS VMM helper was not found; build it with scripts/build-macos-vmm.sh",
	)
}

func devstackGuestDir() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("DEVSTACK_GUEST_DIR")); explicit != "" {
		return filepath.Clean(explicit), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(
		home,
		"Library",
		"Application Support",
		"DevStack",
		"guest",
	), nil
}

func bundledGuestDir() string {
	if explicit := strings.TrimSpace(os.Getenv("DEVSTACK_GUEST_ASSETS_PATH")); explicit != "" {
		return filepath.Clean(explicit)
	}

	executable, err := os.Executable()
	if err != nil || executable == "" {
		return ""
	}

	return filepath.Clean(filepath.Join(
		filepath.Dir(executable),
		"..",
		"Resources",
		"guest",
	))
}

// stageBundledGuestAssets copies immutable assets out of the signed app bundle
// before first use. The guest rootfs is writable, so running it directly from
// Contents/Resources would invalidate the bundle and may fail on read-only
// installations.
func stageBundledGuestAssets() error {
	destination, err := devstackGuestDir()
	if err != nil {
		return err
	}

	kernelDestination := filepath.Join(destination, "vmlinux")
	diskDestination := filepath.Join(destination, "rootfs.ext4")
	if fileExists(kernelDestination) && fileExists(diskDestination) {
		return nil
	}

	source := bundledGuestDir()
	if source == "" ||
		!fileExists(filepath.Join(source, "vmlinux")) ||
		!fileExists(filepath.Join(source, "rootfs.ext4")) {
		return nil
	}

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}

	for _, name := range []string{"vmlinux", "rootfs.ext4", "manifest.txt"} {
		sourcePath := filepath.Join(source, name)
		destinationPath := filepath.Join(destination, name)
		if !fileExists(sourcePath) || fileExists(destinationPath) {
			continue
		}
		if err := copyFileAtomically(sourcePath, destinationPath); err != nil {
			return err
		}
	}

	return nil
}

func copyFileAtomically(source string, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".devstack-asset-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, destination)
}

func devstackVMMStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(
		home,
		"Library",
		"Application Support",
		"DevStack",
		"run",
	), nil
}

func defaultDarwinVMCPUCount() int {
	value := strings.TrimSpace(os.Getenv("DEVSTACK_VM_CPUS"))
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 1 && parsed <= 16 {
			return parsed
		}
	}

	return 4
}

func defaultDarwinVMMemoryMiB() int {
	value := strings.TrimSpace(os.Getenv("DEVSTACK_VM_MEMORY_MIB"))
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 512 && parsed <= 32768 {
			return parsed
		}
	}

	return 2048
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
