# DevStack — Milestone 9: Native-per-platform engine architecture

This milestone is an architectural refactor for the lightweight goal:

> One shared Wails/Vue application and Go core, with an engine backend compiled
> specifically for Linux, macOS, or Windows.

## Main change

The old:

```text
docker_engine.go
    runtime.GOOS switches
    Linux code
    macOS code
    Windows code
```

is replaced by:

```text
engine_common.go
engine_linux.go
engine_darwin.go
engine_windows.go
```

using Go build constraints.

### Linux

```go
//go:build linux
```

Only the native Linux backend is compiled.

### macOS

```go
//go:build darwin
```

Only the macOS backend is compiled.

The current implementation still uses Lima as a working transitional runtime.
The important Milestone 9 change is that it is now isolated behind the macOS
backend boundary.

### Windows

```go
//go:build windows
```

Only the WSL2 implementation is compiled.

## Why do this before adding more features?

It gives us a clean path toward:

```text
Linux   → direct containerd
macOS   → Apple Virtualization.framework + minimal guest
Windows → DevStack-owned minimal WSL2 rootfs
```

without maintaining three complete UI/application implementations.

See:

```text
ARCHITECTURE.md
```

for the full design.

## Existing features remain

Milestone 9 is cumulative from Milestone 8:

- Containers
- Compose grouping
- Compose up/down/build/rebuild
- live Docker events
- live stats
- live logs
- xterm terminal
- details/env/mounts
- images
- volumes
- networks
- storage/prune
- Compose import/drop
- system tray
- autostart
- persistent settings
- macOS Lima managed engine
- Windows WSL2 managed engine
- Linux native engine
- Windows/macOS/Linux build scripts

## Apply

```bash
chmod +x devstack-milestone9/apply.sh

./devstack-milestone9/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then on your Linux machine:

```bash
cd /home/darith/mbanq/devstack/devstack

go mod tidy

wails3 build -tags gtk3

./bin/devstack
```

## Important upgrade behavior

The apply script backs up and removes the old:

```text
docker_engine.go
```

If it remains in the project, its methods would conflict with the new shared
engine API.

## Cross-platform builds

Windows:

```bash
./scripts/build-windows.sh
```

macOS:

```bash
./scripts/build-macos.sh
```

All:

```bash
./scripts/build-all-from-linux.sh
```

## Next recommended milestone

Milestone 10:

1. Add `ContainerRuntime` interface.
2. Keep Docker/Moby as the first runtime implementation.
3. Add direct containerd implementation for Linux.
4. Stop making the application core depend directly on Docker Engine semantics.
5. Prepare the same interface for the future minimal macOS/Windows guests.

That is the next major step toward an actually lightweight runtime rather than
only a lightweight desktop frontend.
