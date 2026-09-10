# DevStack — Milestone 2 patch

This ZIP is a drop-in patch for the DevStack project you already have running.

## Features added

- Start stopped containers
- Stop running containers
- Restart running containers
- Delete containers
- Force-delete running containers after confirmation
- Keep named volumes when deleting
- View the latest container logs
- Choose log tail size (100–5000 lines)
- Decode Docker multiplexed stdout/stderr log streams
- Better loading/error states

## Apply

From your existing `devstack` project:

```bash
# Optional backup
cp docker_service.go docker_service.go.bak
cp frontend/src/App.vue frontend/src/App.vue.bak
```

Extract this ZIP somewhere, then copy:

```bash
cp devstack-milestone2/docker_service.go /path/to/your/devstack/docker_service.go
cp devstack-milestone2/frontend/src/App.vue /path/to/your/devstack/frontend/src/App.vue
```

Or, if this extracted folder is next to your existing project, copy manually.

Then regenerate/check dependencies:

```bash
cd /path/to/your/devstack
go mod tidy
```

### Build with GTK3

Your machine currently works with Wails' GTK3 build path:

```bash
wails3 build -tags gtk3
./bin/devstack
```

If your custom Taskfile is already configured for GTK3 development mode, you can use that instead.

## Notes

Deleting a container does **not** delete its named volumes in this milestone.

Running containers are force-removed only after a confirmation dialog.

## Next milestone

- Container CPU %
- RAM usage
- Ports
- Open localhost URL
- Interactive terminal / exec
- Compose project grouping
