# DevStack — Milestone 13: Native macOS Virtualization.framework backend

Milestone 13 replaces the transitional Lima backend with a native macOS VM
backend built directly on Apple's `Virtualization.framework`.

## Target architecture

```text
DevStack.app
   │
   ├── shared Go / Vue UI
   │
   ├── engine_darwin.go
   │
   └── devstack-vmm (Swift)
          │
          ▼
   Virtualization.framework
          │
          ▼
   one minimal Linux VM
          │
          ├── containerd
          ├── runc
          └── devstack-guestd
                 │
                 │ virtio-vsock
                 ▼
      macOS local Unix proxy
                 │
                 ▼
       Go containerd client
```

No Lima is used by the native backend.

No Docker Desktop is required by the native backend.

## Why a Swift helper?

`Virtualization.framework` is an Apple native Objective-C/Swift framework and
requires the `com.apple.security.virtualization` entitlement.

Instead of forcing the whole Wails/Go desktop app to own VM lifecycle details,
Milestone 13 isolates the native VMM in:

```text
native/macos/DevStackVMM
```

The Go macOS backend starts/stops that helper and connects to the guest
containerd service through a local Unix socket.

## Host ↔ guest transport

The VM has a `VZVirtioSocketDeviceConfiguration`.

Inside Linux:

```text
devstack-guestd
  vsock port 10250
      ↓
/run/containerd/containerd.sock
```

On macOS, `devstack-vmm` exposes:

```text
~/Library/Application Support/DevStack/run/containerd.sock
```

Every local Unix-socket connection is relayed to guest vsock port `10250`.

That lets the existing Go containerd client talk to the guest without exposing
containerd over TCP.

## VM resources

Defaults:

```text
CPU: 4
RAM: 2048 MiB
disk: 2 GiB sparse ext4 image
```

Override the Go backend defaults before launching DevStack:

```bash
export DEVSTACK_VM_CPUS=2
export DEVSTACK_VM_MEMORY_MIB=1024
```

For a small development workload, 1–2 GiB is a reasonable starting point.

## VM guest

The provided guest asset builder creates a minimal ARM64 Alpine root filesystem
containing:

- BusyBox/Alpine base
- containerd
- containerd-shim-runc-v2
- runc
- CNI binaries
- devstack-guestd
- custom `/sbin/devstack-init`

There is no desktop environment and no Docker daemon.

## Current pinned guest components

The asset builder currently pins:

```text
Alpine Linux 3.24.1
containerd 2.3.1
runc 1.5.1
CNI plugins 1.9.1
```

The kernel fetch follows the current Apple-container-style optimized Kata kernel
layout and produces the file DevStack expects as `vmlinux`.

## Step 1 — apply Milestone 13

```bash
chmod +x devstack-milestone13/apply.sh

./devstack-milestone13/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

## Step 2 — build guest assets on Linux

On your Linux machine:

```bash
cd /home/darith/mbanq/devstack/devstack

./scripts/build-macos-guest-assets-linux.sh
```

This produces:

```text
dist/macos-guest/
├── vmlinux
├── rootfs.ext4
└── manifest.txt
```

Copy that folder to your Mac.

## Step 3 — build the self-contained macOS app

On the Mac:

```bash
./scripts/build-macos.sh arm64 --guest-assets /path/to/macos-guest
```

Use `amd64` for an Intel Mac. Add `--install` to copy the completed app to
`~/Applications/DevStack.app`.

The build script packages:

```text
DevStack.app/Contents/Resources/devstack-vmm
DevStack.app/Contents/Resources/guest/vmlinux
DevStack.app/Contents/Resources/guest/rootfs.ext4
```

The helper is ad-hoc signed during development with:

```text
com.apple.security.virtualization = true
```

For a production release, sign the embedded helper with your Developer ID
identity as part of the final app signing pipeline.

## Step 4 — run DevStack

```bash
open dist/macos-arm64/DevStack.app
```

On the first native-engine start, DevStack copies the bundled guest assets to
the writable runtime location:

```text
~/Library/Application Support/DevStack/guest/
├── vmlinux
└── rootfs.ext4
```

Then select:

```text
Engine
  → Native DevStack VM
  → Start / Connect
```

The backend starts `devstack-vmm`.

When the guest's vsock proxy is ready, DevStack automatically selects:

```text
containerd (native macOS guest)
```

## macOS direct-containerd capabilities in Milestone 13

Implemented:

- native VM start/stop
- no Lima dependency
- no Docker Desktop dependency
- containerd over virtio-vsock
- dedicated `devstack` namespace
- OCI image pull/unpack
- container create/start/stop/restart/delete

Not yet implemented on the macOS guest:

- CNI host-port forwarding to macOS
- container IP UI
- logs
- xterm exec
- CPU/RAM stats
- shared folders
- dynamic memory balloon control
- VM snapshots

Those should follow after validating this native transport on Apple Silicon.

## Linux and Windows

Linux direct-containerd from Milestones 11/12 is unchanged.

Windows remains on the WSL2 backend for now.

## Development note

Milestone 13's Swift helper must be compiled/tested on macOS because
`Virtualization.framework` is not available to the Linux build host.

The package includes static validation and build scripts, but the actual native
VM boot path needs to be exercised on your Mac.

## Check native backend

```bash
./scripts/check-macos-native-engine.sh
```

## Next milestone

Milestone 14 should bring macOS guest parity with Linux direct-containerd:

1. guest CNI networking
2. macOS localhost port forwarding
3. shared project folders via VirtioFS
4. VM CPU/RAM settings in UI
5. guest health/boot timing
