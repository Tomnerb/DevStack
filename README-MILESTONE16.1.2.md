# DevStack — Milestone 16.1.2: Main App Container Group Fix

This fixes the issue where many Compose groups appeared to disappear in the
main DevStack Containers page.

## Root cause

The frontend was not limiting the groups. It already rendered every group from
`ListContainers()`.

The problem was engine auto-recovery.

On Linux these are different Docker daemons:

```text
Docker Desktop
  ~/.docker/desktop/docker.sock

System Docker Engine
  /var/run/docker.sock
```

They have different images, containers, and Compose projects.

Milestone 16.1 could see Docker Desktop offline and silently reconnect DevStack
to System Docker. The API became healthy, but the main Containers page was then
showing the system daemon's smaller/different container store.

## Fix

DevStack now remembers its configured Docker endpoint and preserves that engine
identity.

If Docker Desktop was selected:

```text
Reconnect
  -> same Docker Desktop socket only

Start Engine
  -> systemctl --user start docker-desktop
  -> wait for the same Docker Desktop socket
```

It will not silently jump to `/var/run/docker.sock`.

If System Docker is selected, Start Engine starts `docker.service`.

If rootless Docker is selected, Start Engine starts the user's rootless
`docker.service`.

## Engine page

The Engine page now shows the actual selected engine:

```text
Docker Desktop · unix:///home/user/.docker/desktop/docker.sock
```

or:

```text
System Docker Engine · unix:///var/run/docker.sock
```

This makes it clear which daemon owns the containers shown in the main app.

## Apply

```bash
chmod +x devstack-milestone16.1.2/apply.sh

./devstack-milestone16.1.2/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then:

```bash
cd /home/darith/mbanq/devstack/devstack
go mod tidy
wails3 build -tags gtk3
./bin/devstack
```

After starting DevStack, open Engine and verify it says `Docker Desktop` if
those are the containers/projects you expect.
