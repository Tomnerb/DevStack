# DevStack — Milestone 14: Advanced tray / menu-bar controls

Milestone 14 turns the system tray / macOS menu-bar item into a real DevStack
control center.

## What it adds

The tray menu is now dynamic and refreshes every 5 seconds.

Example:

```text
Engine: Running · containerd
Containers: 4 running · 1 stopped
──────────────────────────────
Engine
  Engine: Running · containerd
  ────────────────────────────
  Stop Engine
  Restart Engine

Containers (5)
  Start All Stopped (1)
  Stop All Running (4)
  ────────────────────────────
  ● api
      running
      ───────
      Stop
      Restart

  ● postgres
      running
      ───────
      Stop
      Restart

  ○ worker
      exited
      ───────
      Start

──────────────────────────────
Show/Hide DevStack
Refresh
✓ Start at Login
──────────────────────────────
Quit DevStack
```

## Engine controls

From the menu bar / system tray:

- Start / Connect Engine
- Stop Engine
- Restart Engine
- Reconnect external engine

Managed engine stop/restart is only shown where DevStack actually owns the
engine:

```text
macOS   → native Virtualization.framework VM
Windows → WSL2 managed engine
```

For Linux host Docker/containerd and explicitly external Docker endpoints,
DevStack does not attempt to stop the host service from the tray.

## Container controls

The Containers submenu supports:

- Start All Stopped
- Stop All Running
- Start one stopped container
- Stop one running container
- Restart one running container

This works through the active `ContainerRuntime`, so the same tray UI can
control Docker/Moby containers or direct-containerd containers.

To keep the native menu manageable, DevStack shows up to 8 containers directly.
If more exist, the menu offers:

```text
Open DevStack to view N more…
```

## Live state

The tray rebuilds itself every 5 seconds using Wails' supported dynamic menu
rebuild pattern.

After a tray action, DevStack also emits:

```text
devstack:tray-refresh
```

The Vue UI listens for this event and refreshes the currently visible screen.

## Busy/error feedback

Long operations do not block the tray callback.

While an operation runs:

```text
Working: Stopping engine…
```

If an operation fails:

```text
Last error: ...
```

appears in the next menu state.

## Start at Login

The tray now contains a native checkbox:

```text
✓ Start at Login
```

It uses the existing cross-platform Wails autostart integration.

## Apply

```bash
chmod +x devstack-milestone14/apply.sh

./devstack-milestone14/apply.sh \
  /home/darith/mbanq/devstack/devstack
```

Then on your current Linux machine:

```bash
cd /home/darith/mbanq/devstack/devstack

go mod tidy
wails3 build -tags gtk3
./bin/devstack
```

## macOS

This is exactly the menu shown from the DevStack status item in the macOS menu
bar.

The native macOS engine work from Milestone 13 is unchanged.

## Windows/Linux

The same controller uses Wails' system tray API, so Windows notification-area
and supported Linux desktop tray menus receive the same controls.

Linux tray availability still depends on the desktop environment/system-tray
support.
