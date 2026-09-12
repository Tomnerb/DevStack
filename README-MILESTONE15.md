# Dockiva — Milestone 15: Grouped tray containers

Milestone 15 changes the tray from a flat container list to the same grouping
model used by the main Containers screen.

## Tray structure

Example:

```text
Engine: Running · docker
Containers: 8 running · 2 stopped
────────────────────────────────

Engine
  ...

Containers (10)
  Start All Stopped (2)
  Stop All Running (8)
  Restart All Running (8)

  school-system (5)
    4 running · 1 stopped · Compose project
    ─────────────────────────────
    Start All Stopped (1)
    Stop All Running (4)
    Restart Running (4)
    ─────────────────────────────
    ● api
       running
       Stop
       Restart
    ● postgres
       running
       Stop
       Restart
    ○ worker
       exited
       Start

  poker-game (3)
    3 running · 0 stopped · Compose project
    ...

  Standalone Containers (2)
    1 running · 1 stopped · standalone
    ─────────────────────────────
    Start All Stopped (1)
    Stop All Running (1)
    Restart Running (1)
    ─────────────────────────────
    ● nginx-test
       running
       Stop
       Restart
    ○ redis-test
       exited
       Start
```

## Grouping rules

Containers with Docker Compose label:

```text
com.docker.compose.project
```

are grouped by project name.

Containers without a Compose project are grouped under:

```text
Standalone Containers
```

That includes ordinary `docker run` containers and direct-containerd resources
that do not belong to a Compose project.

## Group actions

Every group supports:

- Start All Stopped
- Stop All Running
- Restart Running

The top Containers menu also retains global:

- Start All Stopped
- Stop All Running
- Restart All Running

## Individual containers

Each individual container submenu shows:

- status
- published ports/localhost URL when present
- Start, or
- Stop + Restart

## Limits

To keep native menus usable:

```text
max project/standalone groups shown: 10
max containers shown per group:      8
```

When a group has more containers, the tray offers:

```text
Open Dockiva to view N more…
```

The full Containers page remains the place for large environments.

## Apply

```bash
chmod +x dockiva-milestone15/apply.sh

./dockiva-milestone15/apply.sh \
  /home/darith/mbanq/dockiva/dockiva
```

Then:

```bash
cd /home/darith/mbanq/dockiva/dockiva

go mod tidy
wails3 build -tags gtk3
./bin/dockiva
```
