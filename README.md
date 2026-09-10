# DevStack

DevStack is a lightweight, cross-platform desktop environment for managing local containers. It combines a Go and Wails core with a Vue interface and supports both platform-native runtimes and existing Docker endpoints.

> [!IMPORTANT]
> DevStack is under active development. Native macOS and Windows engines still require target-platform validation before production use.

## Highlights

- Clean, native-feeling desktop control center
- Container start, stop, restart, delete, logs, terminal, and inspect tools
- Docker Compose import, grouping, project actions, builds, and rebuilds
- Live CPU, memory, PID, port, and networking information
- Image, volume, network, disk-usage, and prune tools
- Dynamic tray and menu-bar controls with event-driven refresh
- Explicit runtime selection with stable endpoint identity
- Optional external Docker support without silently switching engines

## Runtime architecture

```text
                       DevStack UI and Go core
                                  │
                          ContainerRuntime
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                   │
            Linux               macOS               Windows
              │                   │                   │
        containerd/runc     Virtualization.framework  WSL2
                                  │                   │
                           Linux guest          DevStack guest
                                  │                   │
                           containerd/runc      containerd/runc
```

External Docker and Moby endpoints remain available on supported platforms.

## Platform status

| Platform | Backend | Status |
| --- | --- | --- |
| Linux | Direct containerd in the `devstack` namespace | Implemented; CNI networking requires one-time helper installation |
| macOS | Apple `Virtualization.framework` VM | Implemented; requires Linux-generated kernel and root filesystem assets |
| Windows | Dedicated DevStack WSL2 guest | Experimental; native runtime parity remains in progress |
| All | External Docker/Moby endpoint | Supported |

Docker Compose orchestration currently requires the Docker runtime. Native containerd Compose orchestration is planned.

## Requirements

- Go 1.26.3 or newer
- Node.js and npm
- Wails v3 beta.19

Install the matching Wails CLI:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.19
```

Then install frontend dependencies:

```bash
cd frontend
npm ci
cd ..
```

## Development

Run the Wails development environment:

```bash
wails3 task dev
```

The reliable Linux GTK3 path is:

```bash
wails3 build -tags gtk3
./bin/devstack
```

Frontend-only development:

```bash
cd frontend
npm run dev
```

## Build

### macOS

An external-Docker-only build needs only macOS:

```bash
./scripts/build-macos.sh arm64 --external-only
```

The native engine additionally needs `vmlinux` and `rootfs.ext4`. Generate them once on Linux:

```bash
./scripts/build-macos-guest-assets-linux.sh
```

Copy the resulting `macos-guest` directory to the Mac, then build and optionally install:

```bash
./scripts/build-macos.sh arm64 \
  --guest-assets /path/to/macos-guest \
  --install
```

Output:

```text
dist/macos-arm64/DevStack.app
```

Development bundles are ad-hoc signed. Public distribution requires an Apple Developer ID signature and notarization.

### Linux

```bash
./scripts/build-linux.sh amd64
```

Output:

```text
dist/linux-amd64/devstack
```

Install direct-containerd networking once when using the native Linux runtime:

```bash
sudo ./scripts/install-containerd-networking.sh
```

### Windows

Cross-build the executable from macOS or Linux:

```bash
./scripts/build-windows.sh amd64
```

For a native Windows build and installer:

```powershell
wails3 build GOOS=windows GOARCH=amd64
./scripts/package-windows.ps1
```

The dedicated WSL2 guest root filesystem must currently be generated on Linux:

```bash
./scripts/build-windows-wsl-rootfs-linux.sh
```

## Validation

```bash
go test ./...

cd frontend
npm run build
```

Architecture checks are also available:

```bash
./scripts/verify-engine-architecture.sh
./scripts/verify-runtime-architecture.sh
```

## Project structure

```text
frontend/        Vue 3 and TypeScript desktop interface
cmd/             Guest and networking helper commands
native/          macOS VMM and Windows launcher
scripts/         Build, package, install, and validation scripts
build/           Wails platform packaging configuration
engine_*.go      Platform engine lifecycle implementations
runtime_*.go     Runtime-neutral Docker/containerd adapters
```

For architectural decisions and known limitations, see [DEVSTACK_CODEX_HANDOFF.md](DEVSTACK_CODEX_HANDOFF.md). Milestone-specific implementation notes are retained in the `README-MILESTONE*.md` files.
