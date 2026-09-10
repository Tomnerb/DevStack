# DevStack — Milestone 16.1: Engine Auto-Recovery

Milestone 16.1 fixes offline/stale Docker endpoint handling.

A common example is Linux keeping this Docker Desktop context after Desktop is
stopped or removed:

```text
unix:///home/user/.docker/desktop/docker.sock
```

DevStack no longer treats that context as the only possible endpoint.

## Endpoint discovery

`Reconnect` checks reachable local endpoints in priority order, including:

Linux:

```text
DOCKER_HOST
active Docker CLI context
/var/run/docker.sock
/run/docker.sock
$XDG_RUNTIME_DIR/docker.sock
~/.docker/run/docker.sock
~/.docker/desktop/docker.sock
```

macOS also checks the normal Docker and Colima sockets.

Windows checks the standard Docker named pipe.

A candidate is selected only after a real Docker API ping succeeds.

## Start Engine on Linux

When the user explicitly clicks `Start Engine` DevStack does:

```text
1. Reconnect/discover an already-running Docker Engine
2. Try rootless Docker:
   systemctl --user start docker.service
3. Try system Docker:
   systemctl start docker.service
4. If administrator authorization is required and pkexec exists:
   pkexec systemctl start docker.service
5. Discover/reconnect the Docker API again
```

DevStack never runs `sudo` because a desktop app should not request a password
through a hidden terminal.

## Automatic recovery

Two settings are available:

```text
Auto reconnect engine
Start engine when DevStack starts
```

`Auto reconnect engine` defaults on.

Automatic Linux startup is deliberately non-privileged: it may start a
rootless user Docker service, but it will not show a surprise administrator
prompt.

macOS/Windows may start the DevStack-owned VZ/WSL2 backend when configured.

External Docker Desktop is never launched automatically.

## UI

When the engine is offline:

```text
[ Reconnect ] [ Start Engine ]
```

`Reconnect` only discovers/reconnects an existing Docker daemon.

`Start Engine` is the explicit operation allowed to start the Linux service.

## Apply

```bash
chmod +x devstack-milestone16.1/apply.sh

./devstack-milestone16.1/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then:

```bash
cd /home/darith/mbanq/devstack/devstack

go mod tidy
wails3 build -tags gtk3
./bin/devstack
```
