# DevStack

DevStack is a lightweight, cross-platform desktop environment for managing local containers. It combines a Go and Wails core with a Vue interface and supports both platform-native runtimes and existing Docker endpoints.

> [!IMPORTANT]
> DevStack is under active development. Native macOS and Windows engines still require target-platform validation before production use.

## Highlights

- Clean, native-feeling desktop control center
- Persistent light and dark appearance modes
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
                      dockerd/containerd/runc   dockerd/containerd/runc
```

External Docker and Moby endpoints remain available on supported platforms.
DevStack Native exposes its own Docker-compatible API endpoint; it is a
separate engine and never overwrites the selected External Docker endpoint.

## Platform status

| Platform | Backend | Status |
| --- | --- | --- |
| Linux | Direct containerd in the `devstack` namespace | Implemented; CNI networking requires one-time helper installation |
| macOS | Apple `Virtualization.framework` VM | Implemented; requires Linux-generated kernel and root filesystem assets |
| Windows | Dedicated DevStack WSL2 guest | Experimental; native runtime parity remains in progress |
| All | External Docker/Moby endpoint | Supported |

Docker Compose orchestration currently requires the Docker runtime. The macOS
native guest now includes the Docker Engine foundation (`dockerd` over its own
local socket); target-macOS validation is still required before release.

## Docker contexts and migration

DevStack Native and an existing Docker Desktop/System Docker installation are
separate container stores. Use separate Docker contexts rather than trying to
merge live engines:

```text
docker context: devstack       -> DevStack Native
docker context: desktop-linux  -> Docker Desktop
```

The planned **Migrate Docker Data to DevStack** flow is explicitly opt-in. It
will inventory images, containers, volumes, and Compose projects; show disk and
compatibility checks; copy supported data to DevStack Native; and verify the
result. It will never delete, alter, or silently switch the source Docker
engine. Persistent-data migration requires macOS/Windows target validation and
is not claimed as complete by this Linux build.

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

Use `amd64` instead of `arm64` when building for an Intel Mac. The `--install`
option copies the completed application to `~/Applications/DevStack.app`; without
it, the application remains in `dist/` and can be copied to `Applications`
manually.

#### Create a macOS DMG installer

Build the application first, then create the DMG from the completed app bundle:

```bash
./scripts/build-macos.sh arm64 \
  --guest-assets /path/to/macos-guest
wails3 task darwin:create:dmg
```

For an external-Docker-only DMG:

```bash
./scripts/build-macos.sh arm64 --external-only
wails3 task darwin:create:dmg
```

Output:

```text
bin/devstack.dmg
```

Use `darwin:create:dmg` after `build-macos.sh`. The higher-level
`darwin:package:dmg` task rebuilds the application and can replace the native app
bundle before the VMM helper and guest assets are injected. A native DMG contains
`devstack-vmm`, `vmlinux`, and `rootfs.ext4`; an external-only DMG is smaller but
requires an existing Docker or Moby endpoint.

Verify and open the image on macOS with:

```bash
hdiutil verify bin/devstack.dmg
open bin/devstack.dmg
```

Development bundles are ad-hoc signed. This is suitable for local testing, but a
downloadable release must use an Apple Developer ID Application certificate,
hardened runtime, notarization, and stapling. Sign the nested VMM helper with its
Virtualization entitlement before signing the outer application and DMG.

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

To build the current NSIS application installer, run the following commands on
Windows with Wails and NSIS installed:

```powershell
wails3 build GOOS=windows GOARCH=amd64
.\scripts\package-windows.ps1
```

The generated installer is written under `bin/`; its name follows the Wails
application configuration, normally `devstack-amd64-installer.exe`. This
installer installs the desktop application, WebView2 runtime when needed, Start
Menu and desktop shortcuts, and an uninstaller. It does not currently provision
the native WSL2 guest automatically.

The dedicated WSL2 guest root filesystem must currently be generated on Linux:

```bash
./scripts/build-windows-wsl-rootfs-linux.sh
```

Copy `dist/windows-wsl/devstack-wsl-rootfs.tar` to the Windows computer and
provision the dedicated `DevStack` WSL2 distribution from PowerShell:

```powershell
.\scripts\check-wsl-windows.ps1
.\scripts\install-devstack-wsl.ps1 `
  -Rootfs C:\path\to\devstack-wsl-rootfs.tar
```

Run these scripts only after WSL2 is enabled. The setup imports an isolated
distribution named `DevStack` under `%LOCALAPPDATA%\DevStack\wsl`; it does not
modify an existing Ubuntu or other user-managed WSL distribution.

#### Planned Windows installer variants

The intended release packaging has two explicit choices:

- `DevStack-External-Setup.exe` installs the desktop application for use with an
  existing Docker or Moby endpoint and does not require WSL2.
- `DevStack-Native-Setup.exe` bundles the DevStack root filesystem and offers to
  provision the dedicated WSL2 runtime with the user's consent.

The native installer should detect WSL2, explain any required Windows feature or
restart, preserve the DevStack distribution and container data during normal app
upgrades, and never unregister the distribution during uninstall without a
separate explicit confirmation. Public releases should Authenticode-sign both
the application executable and installer.

Until that integrated native installer is implemented and validated on Windows,
use the NSIS application installer and the separate WSL provisioning command
above. The Windows native container runtime remains experimental.

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
