# DevStack — Milestone 11: Direct containerd on Linux

Milestone 11 makes the Linux containerd adapter genuinely usable.

## Runtime support

### Docker / Moby

Still the full-feature runtime:

- containers
- lifecycle
- Compose
- ports
- images
- volumes
- networks
- stats
- logs
- terminal
- inspect

### containerd (Linux, experimental)

Now selectable when DevStack can access a containerd socket.

Implemented:

- dedicated `devstack` namespace
- direct containerd v2 client
- image pull + unpack
- snapshotter selection
- create OCI container
- create/start task
- list container/task state
- stop with SIGTERM then SIGKILL timeout
- restart
- delete task/container
- snapshot cleanup
- persistent runtime choice

Not implemented yet:

- CNI networking
- host port publishing
- live logs
- exec terminal
- stats UI
- Compose
- Docker-style volumes/networks

Those features are hidden/disabled when containerd is active.

## containerd version

The patch adds:

```bash
go get github.com/containerd/containerd/v2@v2.3.4
```

Milestone 11 targets the current v2 client API.

## Rootless first

DevStack prefers an accessible rootless containerd socket when present.

It also checks standard rootful sockets:

```text
/run/containerd/containerd.sock
/var/run/containerd/containerd.sock
```

DevStack should not be run as root just to manage containers.

If a system socket exists but your normal user cannot connect, containerd will
appear unavailable in the Engine screen.

## Apply

```bash
chmod +x devstack-milestone11/apply.sh

./devstack-milestone11/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then:

```bash
cd /home/darith/mbanq/devstack/devstack

go mod tidy
wails3 build -tags gtk3
./bin/devstack
```

## Try containerd

Open:

```text
Engine
  → Container Runtime
  → containerd (direct)
  → Use Runtime
```

When containerd is active, Containers gets a new Direct containerd panel.

Example:

```text
Image: alpine:latest
Name: test-alpine
Command: sleep 3600
Start immediately: yes
```

Click:

```text
Pull Image
Create
```

Then Start / Stop / Restart / Delete use containerd directly.

## Namespace isolation

DevStack uses only:

```text
containerd namespace: devstack
```

It will not list or manage Kubernetes `k8s.io` containers or Docker's internal
containerd namespaces.

## Snapshotter

DevStack tries:

1. `DEVSTACK_CONTAINERD_SNAPSHOTTER`, when set
2. `overlayfs`
3. `native`

The first snapshotter reported as supported by containerd is used.

## Important networking limitation

Milestone 11 does not configure CNI.

Containers created through the direct containerd runtime should be treated as
local process-isolation experiments for now. Do not expect Docker-style bridge
networking or `-p host:container` behavior.

## Verify

```bash
./scripts/check-containerd-runtime.sh
```

The helper shows:

- containerd sockets
- `ctr`
- namespaces when permission allows
- DevStack namespace

## Next milestone

Milestone 12 should add CNI networking and port publishing:

```text
DevStack containerd
   ↓
CNI bridge
   ├── container IP
   ├── DNS
   └── host port mappings
```

After that, logs/exec/stats can be migrated.
