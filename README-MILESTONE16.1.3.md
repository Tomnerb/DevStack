# Dockiva — Milestone 16.1.3: Docker Desktop Start UX Fix

This fixes two issues visible when Docker Desktop is installed but stopped.

## Fixed: installed vs running

Previously:

```text
Docker Desktop stopped
→ Engine: not provisioned
```

That was incorrect because `EngineInstalled` was derived from Docker API
reachability.

Milestone 16.1.3 checks the Linux systemd unit independently:

```text
systemctl --user show docker-desktop.service \
  --property=LoadState --value
```

So a stopped but installed Docker Desktop now shows:

```text
Engine stopped / unavailable
Engine: installed
Docker Desktop is stopped.
```

## Fixed: long Starting… state

Docker Desktop is now launched with:

```bash
systemctl --user start --no-block docker-desktop
```

The command returns immediately, then Dockiva polls only the configured Docker
Desktop socket for up to ~20 seconds.

It does not fall back to `/var/run/docker.sock`.

## Apply

```bash
chmod +x dockiva-milestone16.1.3/apply.sh

./dockiva-milestone16.1.3/apply.sh \
  /home/darith/mbanq/dockiva/dockiva
```

Then:

```bash
cd /home/darith/mbanq/dockiva/dockiva
go mod tidy
wails3 build -tags gtk3
./bin/dockiva
```

If Docker Desktop still does not become ready, Dockiva returns an error instead
of staying in an indefinite-looking `Starting…` state.
