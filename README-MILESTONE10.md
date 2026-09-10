# DevStack — Milestone 10

Milestone 10 introduces the runtime abstraction required for the lightweight
architecture.

## New

- `ContainerRuntime` Go interface
- Docker/Moby runtime adapter
- Container lifecycle delegated through the runtime
- Linux-only containerd adapter skeleton
- containerd socket and `ctr` detection
- Runtime status/capabilities on the Engine screen
- Build-tag separation for non-Linux containerd detection

## Important

Docker/Moby remains the active runtime.

containerd can be detected on Linux but is **not selectable yet**. This is
intentional: we do not want to regress Compose, ports, logs, exec, or networking
while direct containerd behavior is incomplete.

## Apply

```bash
chmod +x devstack-milestone10/apply.sh

./devstack-milestone10/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then:

```bash
cd /home/darith/mbanq/devstack/devstack
go mod tidy
wails3 build -tags gtk3
./bin/devstack
```

Verify:

```bash
./scripts/verify-runtime-architecture.sh
```

## Next milestone

Milestone 11 will implement the first real direct-containerd lifecycle on Linux
inside a dedicated `devstack` namespace.
