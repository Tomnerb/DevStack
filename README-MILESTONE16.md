# DevStack — Milestone 16 Combined Runtime Release

This cumulative package combines the planned Milestones 16, 17, and 18 work
into one development release.

## 16A — macOS native engine usability

The native `Virtualization.framework` backend from Milestone 13 is preserved.

Added:

- VirtioFS sharing for `~/Projects`
- guest mount point `/mnt/devstack/host`
- existing minimal Linux + containerd design remains
- no Lima requirement for native mode
- no Docker Desktop requirement for native mode

The macOS VMM still needs a real compile/boot validation on macOS because
`Virtualization.framework` is unavailable on the Linux build host.

## 16B — direct containerd developer tools

Linux direct-containerd now integrates with the existing DevStack UI APIs for:

- live stdout/stderr logs for newly started direct-containerd tasks
- interactive xterm `/bin/sh`
- terminal input and resize
- CPU usage from cgroup v2
- memory usage / limit
- PID count

The existing Docker/Moby implementations remain unchanged.

Important: containers whose tasks were already running before this milestone
may not have DevStack-managed log files. Restart them once after applying the
milestone to enable the new log capture path.

## 16C — dedicated Windows WSL2 foundation

The package adds a DevStack-owned WSL2 rootfs workflow so future Windows
containerd support does not need to modify a user's Ubuntu/Debian distro.

Build the rootfs on Linux:

```bash
./scripts/build-windows-wsl-rootfs-linux.sh
```

This creates:

```text
dist/windows-wsl/devstack-wsl-rootfs.tar
```

Install it on Windows:

```powershell
.\scripts\install-devstack-wsl.ps1 `
  -Rootfs C:\path\devstack-wsl-rootfs.tar
```

The dedicated distro name is:

```text
DevStack
```

This Windows path is still experimental and needs a real WSL2 target test.

## Tray fixes retained

Milestone 16 is based on the latest Milestone 15 tray-fixed package:

- clicking the tray icon does not open/toggle DevStack
- `Show DevStack` is explicit
- Compose projects are grouped in the tray
- non-Compose containers are under `Standalone Containers`
- tray refresh is event-driven rather than every 5 seconds

## Apply

```bash
chmod +x devstack-milestone16/apply.sh

./devstack-milestone16/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then:

```bash
cd /home/darith/mbanq/devstack/devstack

go mod tidy
wails3 build -tags gtk3
./bin/devstack
```

## Validation performed on the package

- all Go source files passed `gofmt` parsing
- all shell scripts passed `bash -n`
- the previous Vue malformed `filteredGroups` quote is not present
- the latest event-driven/grouped tray implementation is retained

The macOS Swift helper and Windows WSL2 provisioning must still be tested on
their real target operating systems.
