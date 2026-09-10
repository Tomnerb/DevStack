# DevStack — Codex Handoff / Continuation Document

**Project:** DevStack  
**Goal:** Build a lightweight cross-platform OrbStack-like desktop container development app.  
**Primary stack:** Go + Wails v3 + Vue 3 Composition API + TypeScript + Tailwind CSS  
**Current Linux development path:** `/home/darith/mbanq/devstack/devstack`  
**Current Linux build mode:** GTK3 compatibility path  
**Recommended Linux build command:**

```bash
wails3 build -tags gtk3
./bin/devstack
```

> Important: `wails3 dev -tags gtk3` did not work in the current Linux environment. The reliable path has been `wails3 build -tags gtk3`.

---

# 1. Product Direction

DevStack is intended to become a lightweight cross-platform alternative to Docker Desktop / OrbStack-style developer environments.

The core architecture should remain:

```text
                        DevStack UI/Core
                              │
                       shared Go + Vue
                              │
                      ContainerRuntime
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
        Linux               macOS               Windows
          │                   │                   │
 native containerd    Apple Virtualization      WSL2
       + runc            .framework VM       dedicated guest
          │                   │                   │
          │              containerd/runc      containerd/runc
          │                   │                   │
          └───────────────────┴───────────────────┘
```

The long-term product rule is:

- DevStack should **not require Docker Desktop**.
- Docker Desktop should be treated only as an optional external engine.
- Platform-native lightweight engines are preferred.
- Shared UI and service APIs should remain runtime-neutral.
- Platform-specific engine/runtime details should be isolated behind Go interfaces and build tags.

---

# 2. Current Platform Strategy

## Linux

Preferred future engine:

```text
DevStack
  ↓
native containerd
  ↓
runc
  ↓
Linux kernel
```

Docker / Moby remains supported as an external runtime.

Current direct-containerd implementation uses a dedicated namespace:

```text
devstack
```

Networking from Milestone 12 uses:

```text
bridge:  devstack0
subnet:  10.89.0.0/16
gateway: 10.89.0.1
```

## macOS

Preferred engine:

```text
DevStack.app
  ↓
Apple Virtualization.framework
  ↓
minimal Linux guest
  ↓
containerd
  ↓
runc
```

No Lima should be required in the final native path.

No Docker Desktop should be required.

## Windows

Preferred engine:

```text
DevStack
  ↓
WSL2
  ↓
dedicated "DevStack" Linux distro/rootfs
  ↓
containerd
  ↓
runc
```

Do not modify or depend on the user's Ubuntu/Debian WSL distro in the final design.

---

# 3. Important Current UX Rules

These decisions should be preserved unless intentionally redesigned.

## Tray behavior

Clicking the tray icon should **only open the native tray menu**.

It must **not** toggle/show/hide the DevStack main window.

The main window should open only when the user explicitly selects:

```text
Show DevStack
```

Current desired tray structure:

```text
DevStack
├── Engine
│   ├── Start / Connect
│   ├── Stop Managed Engine
│   ├── Restart
│   └── Reconnect
│
├── Containers
│   ├── Start All Stopped
│   ├── Stop All Running
│   ├── Restart All Running
│   │
│   ├── compose-project-a
│   │   ├── Start All Stopped
│   │   ├── Stop All Running
│   │   ├── Restart Running
│   │   ├── api
│   │   ├── postgres
│   │   └── redis
│   │
│   ├── compose-project-b
│   │   └── ...
│   │
│   └── Standalone Containers
│       ├── nginx-test
│       └── redis-test
│
├── Show DevStack
├── Hide DevStack
├── Refresh
├── Start at Login
└── Quit
```

`Standalone Containers` means containers that do not have a Docker Compose project label.

## Tray refresh

Do not rebuild the entire tray every 5 seconds.

Current desired behavior is event-driven:

```text
initial creation
Docker/container event
tray lifecycle action completed
manual Refresh
```

Docker event bursts are debounced.

## Main app grouping

The frontend grouping model is:

```text
container.composeProject exists
    → Compose project group

container.composeProject empty
    → Standalone Containers
```

The main app should render all groups and all containers.

There should be no arbitrary `slice()` / "first N groups" restriction.

---

# 4. Milestone History

## Phase 1 — Initial Docker Integration

Implemented basic Docker connectivity and container listing.

Foundation:

- Docker status
- list containers
- initial Wails/Vue integration

Artifact:

```text
devstack-phase1.zip
```

---

## Milestone 2 — Container Lifecycle

Added:

- Start container
- Stop container
- Restart container
- Delete container
- Logs

Artifact:

```text
devstack-milestone2.zip
```

---

## Milestone 3 — Compose Grouping

Added grouping from Docker Compose labels:

```text
com.docker.compose.project
com.docker.compose.service
```

Added:

- Compose project grouping
- Standalone Containers group
- port display
- localhost links

Artifact:

```text
devstack-milestone3.zip
```

---

## Milestone 4 — Group Actions + Resource Stats

Added project/group actions:

- Start all
- Stop all
- Restart all

Added container resource metrics:

- CPU
- memory

Artifact:

```text
devstack-milestone4.zip
```

---

## Milestone 5 — Resource Sidebar + Terminal Foundation

Added:

- Images
- Volumes
- Networks
- sidebar navigation
- basic terminal
- streaming logs

Artifact:

```text
devstack-milestone5.zip
```

---

## Milestone 6 — Developer Tools

Added:

- xterm.js terminal
- inspect/details
- Docker Compose:
  - up
  - down
  - build
  - rebuild
- image pull
- volume create
- network create
- disk usage
- prune

Artifact:

```text
devstack-milestone6.zip
```

---

## Milestone 7 — Cross-Platform Docker Endpoint Foundation

Added:

- cross-platform Docker endpoint discovery
- Docker event watcher
- resource search
- Compose import/drop
- build scripts
- GitLab cross-platform example

At this point macOS/Windows still managed an existing Docker engine rather than using a native DevStack engine.

Artifact:

```text
devstack-milestone7.zip
```

---

## Milestone 8 — First Managed Engine Attempt

Attempted:

```text
Linux   → native Docker
macOS   → Lima
Windows → WSL2
```

Important:

**Do not use Milestone 8 as a trusted base.**

The original Milestone 8 generation had an internal failed generation and may be inconsistent.

Artifact:

```text
devstack-milestone8.zip
```

---

## Milestone 9 — OS Engine Backend Refactor

Refactored engine management using build-tag-specific platform backends.

Files introduced/organized:

```text
engine_common.go
engine_linux.go
engine_darwin.go
engine_windows.go
```

Architecture:

```go
type platformEngineBackend interface {
    Name() string
    Status(...)
    Start(...)
    Stop(...)
    Delete(...)
    Provision(...)
    Options() []string
}
```

Platform selection happens at compile time using Go build tags.

Old monolithic `docker_engine.go` was removed.

Artifact:

```text
devstack-milestone9.zip
```

---

## Milestone 10 — ContainerRuntime Abstraction

Introduced runtime-neutral container lifecycle abstraction:

```go
type ContainerRuntime interface {
    Info(ctx context.Context) ContainerRuntimeInfo

    ListContainers(ctx context.Context) ([]ContainerInfo, error)

    StartContainer(...)
    StopContainer(...)
    RestartContainer(...)
    RemoveContainer(...)
}
```

Implemented:

- Docker/Moby runtime adapter
- initial Linux containerd detection
- runtime overview UI
- capability model

Docker-only features remained Docker-specific at this milestone.

Artifact:

```text
devstack-milestone10.zip
```

---

## Milestone 11 — Direct Linux containerd

Made Linux direct-containerd real and selectable.

Added:

- containerd v2 client
- namespace `devstack`
- image pull + unpack
- snapshotter selection
- OCI container creation
- task start/stop/restart/delete
- snapshot cleanup
- persistent runtime choice
- capability-aware frontend
- runtime image pull
- runtime container create
- rootless socket detection

Containerd dependency:

```text
github.com/containerd/containerd/v2@v2.3.4
```

Known issue fixed during M11:

Bad Vue template:

```text
v-for="group in filteredGroups\"
```

Corrected artifact:

```text
devstack-milestone11-fixed.zip
```

Additional hotfix artifact:

```text
devstack-milestone11-hotfix.zip
```

Direct containerd still lacked networking/logs/terminal/stats/Compose at this point.

---

## Milestone 12 — Linux containerd CNI Networking

Added native CNI networking for direct-containerd.

Added:

```text
ContainerInfo.IPAddress
RuntimeCapabilities.PortPublishing
RuntimeCapabilities.DNS
RuntimePortMapping
```

Privileged helper:

```text
cmd/devstack-netd/main.go
```

Uses:

```text
github.com/containerd/go-cni@v1.1.14
```

Linux networking:

```text
CNI config: /etc/cni/net.d/10-devstack.conflist
network:    devstack-net
bridge:     devstack0
subnet:     10.89.0.0/16
gateway:    10.89.0.1
```

Persistent netns:

```text
/var/run/netns/devstack-<id>
```

Network state:

```text
/var/lib/devstack/networks/*.json
```

Added scripts:

```text
scripts/install-containerd-networking.sh
scripts/check-containerd-networking.sh
scripts/uninstall-containerd-networking.sh
```

UI port input format:

```text
8080:80,8443:443
```

Security constraints:

- localhost-only TCP publishing
- host ports >= 1024

Rootless containerd CNI bridge intentionally remained unsupported.

Artifact:

```text
devstack-milestone12.zip
```

---

## Milestone 13 — Native macOS Virtualization.framework Foundation

Replaced the transitional Lima architecture with a native Apple virtualization design.

Architecture:

```text
DevStack.app
  ↓
engine_darwin.go
  ↓
devstack-vmm (Swift)
  ↓
Virtualization.framework
  ↓
minimal Linux guest
  ↓
containerd / runc
```

State path:

```text
~/Library/Application Support/DevStack
```

Guest assets:

```text
guest/vmlinux
guest/rootfs.ext4
```

Runtime socket proxy:

```text
run/containerd.sock
```

Default VM resources:

```text
CPU:    4
Memory: 2048 MiB
```

Environment overrides:

```text
DEVSTACK_VM_CPUS
DEVSTACK_VM_MEMORY_MIB
```

Swift VMM includes:

- `VZLinuxBootLoader`
- `VZVirtioBlockDeviceConfiguration`
- `VZNATNetworkDeviceAttachment`
- `VZVirtioEntropyDeviceConfiguration`
- `VZVirtioSocketDeviceConfiguration`
- serial console
- status/pid state
- Unix socket → vsock proxy
- virtualization entitlement

Guest agent:

```text
cmd/devstack-guestd/main.go
```

Guest vsock port:

```text
10250
```

Guest init:

```text
native/macos/devstack-init
```

Guest builder script:

```text
scripts/build-macos-guest-assets-linux.sh
```

Pinned/default guest components used during generation included:

```text
Alpine      3.24.1
containerd  2.3.1
runc        1.5.1
CNI         1.9.1
```

Important validation limitation:

The Apple `Virtualization.framework` code cannot be fully compiled/boot-tested from the Linux development machine.

Artifact:

```text
devstack-milestone13.zip
```

---

## Milestone 14 — Advanced Tray / Menu Bar

Implemented a dynamic cross-platform Wails tray controller.

File:

```text
tray_controller.go
```

Tray included:

- engine status
- container running/stopped count
- engine start/stop/restart/reconnect
- global container actions
- individual container actions
- Show DevStack
- Hide DevStack
- Refresh
- Start at Login
- Quit

Initially included a tray `OnClick` show/hide toggle.

Artifact:

```text
devstack-milestone14.zip
```

---

## Milestone 15 — Grouped Tray Containers

Changed tray containers from a flat list to groups matching the main Containers page.

Tray groups:

```text
Compose project groups
Standalone Containers
```

Each group supports:

- Start All Stopped
- Stop All Running
- Restart Running

Initially had limits:

```text
10 groups
8 containers per group
```

Artifact:

```text
devstack-milestone15.zip
```

---

## Milestone 15 Tray Toggle Fix

Problem:

Clicking the tray icon opened the menu but also triggered the main window toggle.

Cause:

```go
t.tray.OnClick(...)
```

Fix:

Removed the tray-level window toggle.

Desired behavior:

```text
tray icon click → menu only
Show DevStack    → explicitly show main window
```

Artifact:

```text
devstack-milestone15-trayfix.zip
```

---

## Milestone 15 Event-Driven Tray Refresh

Problem:

Tray menu rebuilt every 5 seconds and felt like it constantly refreshed/flickered.

Fix:

Removed periodic refresh loop.

Refresh only on:

- initial creation
- Docker/container events
- tray actions
- manual Refresh

Added debounce around refresh.

Original event-driven package had a compile bug because these fields were missing:

```go
refreshMu    sync.Mutex
refreshTimer *time.Timer
```

Corrected cumulative package:

```text
devstack-milestone15-trayfix-eventdriven-fixed.zip
```

Small compile hotfix:

```text
devstack-tray-event-refresh-hotfix.zip
```

---

## Milestone 16 — Combined 16 + 17 + 18

The user chose to combine:

```text
16A macOS native usability
16B containerd developer tools
16C Windows dedicated WSL2
```

Artifact:

```text
devstack-milestone16.zip
```

### 16A — macOS

Planned/added foundation for:

- native `Virtualization.framework`
- VirtioFS sharing
- `~/Projects`
- guest mount path:
  `/mnt/devstack/host`
- CNI/guest networking direction
- native guest architecture

### 16B — direct-containerd developer tools

Added Linux direct-containerd integration for:

- logs
- xterm terminal
- CPU stats
- memory stats
- PID stats

Existing frontend methods were kept runtime-neutral:

```text
GetContainerStats
StartLogStream
StopLogStream
StartTerminal
SendTerminalInput
ResizeTerminal
CloseTerminal
```

Direct-containerd task stdout/stderr is captured into DevStack-owned log files.

Terminal uses containerd `Task.Exec` with `/bin/sh`.

Metrics use cgroup v2.

### 16C — Windows

Added dedicated WSL2 rootfs foundation.

Scripts include:

```text
scripts/build-windows-wsl-rootfs-linux.sh
scripts/install-devstack-wsl.ps1
native/windows/devstack-start
```

Intended distro:

```text
DevStack
```

Important:

macOS and Windows target-specific paths still require real target OS testing.

---

## Milestone 16.1 — Engine Auto-Recovery

Problem:

When Docker was offline, DevStack displayed errors such as:

```text
failed to connect to the docker API at
unix:///home/darith/.docker/desktop/docker.sock
```

Added:

- `Reconnect`
- `Start Engine`
- automatic reconnect setting
- start engine on launch setting
- Docker endpoint discovery
- rootless Docker startup
- system Docker startup
- optional `pkexec` administrator authorization

New settings:

```text
Auto reconnect engine
Start engine when DevStack starts
```

Artifact:

```text
devstack-milestone16.1-engine-autorecovery.zip
```

---

## Milestone 16.1.1 — Tray Show All Groups

The tray's old limits:

```text
trayGroupLimit = 10
trayContainerLimitPerGroup = 8
```

were removed.

Tray should now render all groups and all containers.

Artifacts:

```text
devstack-tray-show-all-groups-hotfix.zip
devstack-milestone16.1.1-full-tray-groups.zip
```

Important:

This only fixed the tray limit.

It did **not** explain the later missing groups in the main DevStack app.

---

## Milestone 16.1.2 — Main App / Docker Engine Identity Diagnosis

The user reported many container groups were missing in the main DevStack application.

Frontend inspection showed the main app already renders:

```vue
v-for="group in filteredGroups"
```

with no hard `slice()` limit.

Grouping logic is correct:

```ts
const key = container.composeProject
  ? `compose:${container.composeProject}`
  : 'standalone'
```

The actual problem was Docker daemon identity.

On Linux:

```text
Docker Desktop:
~/.docker/desktop/docker.sock

System Docker:
 /var/run/docker.sock
```

These are different daemons with different container stores.

Milestone 16.1's broad recovery could connect to System Docker when Docker Desktop was offline.

Result:

```text
DevStack reports connected
but
many expected Compose projects disappear
```

because DevStack was reading another Docker daemon.

A fix package was generated to preserve Docker engine identity:

```text
devstack-milestone16.1.2-main-app-groups-fix.zip
```

Core rule introduced:

**Do not silently switch between Docker Desktop, System Docker, rootless Docker, or another external endpoint during reconnect.**

---

## Milestone 16.1.3 — Docker Desktop Start UX Fix

Screenshot showed:

```text
Engine stopped / unavailable
Engine: not provisioned
Docker Desktop socket selected
```

This exposed two issues:

1. "installed" and "running" were incorrectly treated as the same state.
2. Docker Desktop startup could leave the UI showing `Starting…` too long.

Generated fix:

```text
devstack-milestone16.1.3-docker-desktop-start-fix.zip
```

Changes included:

- detect Docker Desktop systemd user unit separately from API reachability
- show installed/stopped independently
- launch Docker Desktop with:

```bash
systemctl --user start --no-block docker-desktop
```

- poll the same original socket with a bounded timeout
- do not switch to System Docker

---

# 5. Most Important Current Design Correction

After Milestone 16.1.3, the user asked:

> Why does DevStack need to start Docker Desktop? What if the user never installs Docker Desktop?

This is the correct product-level concern.

The next implementation should **not make Docker Desktop part of the required DevStack runtime**.

## Desired next architecture

```text
Engine Backend

● DevStack Native
  Lightweight runtime owned by DevStack

○ External Docker
  Docker Desktop / System Docker / Rootless Docker / DOCKER_HOST
```

## Recommended platform defaults

### Linux

```text
1. DevStack native containerd
2. optional System Docker
3. optional Rootless Docker
4. optional Docker Desktop
```

### macOS

```text
1. Native DevStack Virtualization.framework VM
2. optional external Docker
```

### Windows

```text
1. Dedicated DevStack WSL2 engine
2. optional external Docker
```

Docker Desktop must be optional.

DevStack should never start Docker Desktop unless the user explicitly selected:

```text
External Docker → Docker Desktop
```

---

# 6. Recommended Next Milestone

## Milestone 16.1.4 — Docker Desktop Optional / Native Engine Default

This is the next milestone Codex should implement.

### Goal

Make DevStack its own container environment first, with external Docker as an optional compatibility mode.

### Required behavior

#### Engine selector

Change the current conceptual options into something like:

```text
DevStack Native
External Docker
```

Advanced external options may include:

```text
Docker Desktop
System Docker
Rootless Docker
Custom DOCKER_HOST
```

#### Auto mode

Do not interpret `auto` as:

```text
any reachable Docker socket
```

Instead prefer the DevStack-owned runtime.

Suggested order:

```text
Linux:
  DevStack native containerd
  ↓ fallback only if explicitly configured
  external Docker

macOS:
  DevStack native VZ
  ↓
  external Docker

Windows:
  DevStack WSL2
  ↓
  external Docker
```

#### Migration of current settings

Existing users may have:

```json
{
  "engineBackend": "auto",
  "runtimeProvider": "docker",
  "dockerEndpoint": "unix:///home/user/.docker/desktop/docker.sock"
}
```

Do not destroy this setting.

Migration suggestion:

- preserve the external endpoint
- store it as the external Docker preference
- default new installs to DevStack Native
- existing installs may remain External Docker until the user switches, or show a one-time migration choice

#### Offline UI

For DevStack native backend:

```text
Engine stopped

[ Start DevStack Engine ]
```

Do not display Docker Desktop-specific instructions.

For External Docker:

```text
External Docker offline

Docker Desktop
unix:///home/user/.docker/desktop/docker.sock

[ Reconnect ]
[ Start Docker Desktop ]   // only when explicit external choice is Desktop
```

---

# 7. Current Main Containers Page Logic

The main app grouping logic currently resembles:

```ts
const groups = computed<ContainerGroup[]>(() => {
  const map = new Map<string, Container[]>()

  for (const container of containers.value) {
    const key = container.composeProject
      ? `compose:${container.composeProject}`
      : 'standalone'

    const existing = map.get(key) ?? []
    existing.push(container)
    map.set(key, existing)
  }

  // build groups...
})
```

Filtered groups are rendered as:

```vue
<section
  v-for="group in filteredGroups"
  :key="group.key"
>
```

There is no intended main-app group count limit.

If groups are missing in the main application, first check:

```text
1. How many containers ListContainers() returned
2. Which Docker/containerd runtime is active
3. Which Docker endpoint is active
4. composeProject label values
5. frontend search query
```

Do not assume the main Vue grouping code is the cause.

---

# 8. Docker Engine Identity Rule

This is critical.

These must be treated as separate engines:

```text
Docker Desktop
System Docker
Rootless Docker
Custom DOCKER_HOST
DevStack containerd
```

Never reconnect from one to another silently.

Example:

```text
selected:
unix:///home/darith/.docker/desktop/docker.sock

offline

WRONG:
automatically connect /var/run/docker.sock

RIGHT:
show Docker Desktop offline
reconnect/start that same selected engine
or let user explicitly select another engine
```

Reason:

Switching endpoints changes the visible container/image/Compose project store.

---

# 9. Important Files

## Core application

```text
main.go
app_settings.go
docker_service.go
docker_extra_service.go
docker_platform.go
```

## Engine abstraction

```text
engine_common.go
engine_linux.go
engine_darwin.go
engine_windows.go
```

## Runtime abstraction

```text
runtime_common.go
runtime_docker.go
runtime_containerd_linux.go
runtime_containerd_darwin.go
runtime_candidates_other.go
```

Later runtime tooling files may include:

```text
runtime_developer_tools.go
runtime_stream_bridge.go
runtime_containerd_tools_linux.go
```

depending on the exact currently-applied cumulative package.

## Tray

```text
tray_controller.go
```

## Linux containerd networking

```text
network_helper_client_linux.go
cmd/devstack-netd/main.go
```

## macOS native engine

```text
native/macos/DevStackVMM/Package.swift
native/macos/DevStackVMM/devstack-vmm.entitlements
native/macos/DevStackVMM/Sources/DevStackVMM/main.swift
native/macos/devstack-init
cmd/devstack-guestd/main.go
```

## Windows

```text
engine_windows.go
native/windows/devstack-start
scripts/build-windows-wsl-rootfs-linux.sh
scripts/install-devstack-wsl.ps1
```

## Frontend

```text
frontend/src/App.vue
frontend/src/components/XtermTerminal.vue
frontend/src/components/EngineSettingsPanel.vue
frontend/src/components/ContainerDetailsModal.vue
```

---

# 10. Linux Development Environment Notes

Current Linux machine:

```text
amd64
zsh
```

Project:

```text
/home/darith/mbanq/devstack/devstack
```

GTK4/WebKit path crashed in the current environment.

Use:

```bash
wails3 build -tags gtk3
./bin/devstack
```

Do not use:

```bash
wails3 dev -tags gtk3
```

unless Wails behavior has changed and is explicitly re-tested.

---

# 11. Important Validation Rules

Before giving the user another milestone ZIP:

## Go

Run:

```bash
gofmt -w <changed .go files>
```

Whenever possible, run the actual project build after applying to the user's project:

```bash
wails3 build -tags gtk3
```

Do not treat `gofmt` success as equivalent to a full build.

## Vue

Check the previous malformed quote does not return:

```text
v-for="group in filteredGroups\"
```

Correct form:

```text
v-for="group in filteredGroups"
```

If frontend dependencies are installed, also run the actual frontend build.

## Shell

Run:

```bash
bash -n script.sh
```

## macOS

Do not claim the VZ engine is fully verified from Linux.

A real Mac test is required for:

- Swift compilation
- signing/entitlements
- VM boot
- guest rootfs
- vsock
- VirtioFS
- containerd guest lifecycle

## Windows

A real Windows test is required for:

- WSL2 import
- dedicated DevStack distro
- containerd startup
- localhost forwarding
- filesystem mounts

---

# 12. Known Limitations / Incomplete Areas

## Compose on direct containerd

Docker Compose orchestration is still fundamentally Docker/Moby-specific.

Future work should implement a Compose-to-containerd orchestration layer or another controlled compatibility strategy.

## Rootless containerd networking

The privileged CNI bridge from Milestone 12 is intentionally not suitable for rootless containerd.

Future options:

```text
RootlessKit
pasta
slirp4netns
```

## macOS port forwarding / networking

The native VZ architecture has the foundation, but real guest networking/port forwarding needs target-platform validation.

Recommended model:

```text
macOS localhost:8080
        ↓
DevStack host listener
        ↓
vsock
        ↓
guest agent
        ↓
container 10.89.x.x:80
```

Avoid complicated host routing when a controlled localhost tunnel is sufficient.

## Logs for old direct-containerd tasks

If log capture is attached when a task is created, already-running tasks created before that implementation may not have DevStack-managed log files.

A restart/recreate may be necessary.

---

# 13. Future Milestones After 16.1.4

Suggested sequence:

## Milestone 17 / next major runtime milestone

Depending on what remains after the combined M16 work:

- stabilize native runtime startup
- containerd event subscriptions
- cross-platform log/terminal/stats parity
- runtime health diagnostics

## Compose on containerd

Implement:

```text
Compose YAML
   ↓
DevStack parser/planner
   ↓
networks
volumes
containers
dependencies
health/start order
   ↓
containerd
```

Do not just shell out to Docker Compose if the goal is a Docker-Desktop-independent product.

## Local developer domains

Potential later feature:

```text
api.devstack.local
web.devstack.local
```

with:

- local DNS
- automatic reverse proxy
- certificates
- service discovery

## Better file sharing

macOS:

```text
VirtioFS
```

Windows:

```text
WSL filesystem / controlled Windows mount mapping
```

Linux:

```text
native bind mounts
```

---

# 14. Milestone Artifact Reference

Historical artifacts generated during development:

```text
devstack-phase1.zip

devstack-milestone2.zip
devstack-milestone3.zip
devstack-milestone4.zip
devstack-milestone5.zip
devstack-milestone6.zip
devstack-milestone7.zip
devstack-milestone8.zip
devstack-milestone9.zip
devstack-milestone10.zip

devstack-milestone11.zip
devstack-milestone11-fixed.zip
devstack-milestone11-hotfix.zip

devstack-milestone12.zip
devstack-milestone13.zip
devstack-milestone14.zip
devstack-milestone15.zip

devstack-milestone15-trayfix.zip
devstack-milestone15-trayfix-eventdriven.zip
devstack-milestone15-trayfix-eventdriven-fixed.zip
devstack-tray-event-refresh-hotfix.zip

devstack-milestone16.zip

devstack-milestone16.1-engine-autorecovery.zip

devstack-tray-show-all-groups-hotfix.zip
devstack-milestone16.1.1-full-tray-groups.zip

devstack-milestone16.1.2-main-app-groups-fix.zip

devstack-milestone16.1.3-docker-desktop-start-fix.zip
```

Important:

Do not rebuild the project from older milestone ZIPs unless intentionally recovering something.

The actual user's current project directory is the source of truth after patches have been applied.

Codex should inspect the current files before changing anything.

---

# 15. Recommended Codex Workflow

When continuing development in Codex:

```text
1. Read this document completely.
2. Inspect the CURRENT project files.
3. Do not assume an old milestone ZIP is the current state.
4. Run git status / git diff if the project is in Git.
5. Identify the currently-active Docker endpoint/runtime.
6. Run the Linux GTK3 build before changing code.
7. Make one coherent milestone change.
8. Run formatting + compile/build tests.
9. Do not silently change runtime/engine identity.
10. Preserve tray behavior.
```

Recommended first commands:

```bash
cd /home/darith/mbanq/devstack/devstack

git status
git diff --stat

grep -R "dockerEndpoint" -n .
grep -R "DockerEndpoint" -n .
grep -R "ReconnectExternalDocker" -n .
grep -R "DiscoverDockerEndpoint" -n .
grep -R "docker-desktop" -n .
grep -R "filteredGroups" -n frontend/src

wails3 build -tags gtk3
```

Also inspect:

```bash
docker context ls
docker context show
docker context inspect "$(docker context show)"
```

and, on Linux:

```bash
ls -l \
  /var/run/docker.sock \
  "$HOME/.docker/desktop/docker.sock" \
  "$HOME/.docker/run/docker.sock" \
  2>/dev/null || true
```

The purpose is to determine exactly which engine owns the expected Compose projects before changing recovery logic.

---

# 16. Copy-Paste Prompt for Codex

Use this prompt after adding this document to the repository:

```text
You are continuing development of DevStack, a lightweight cross-platform
OrbStack-like desktop container application.

Read DEVSTACK_CODEX_HANDOFF.md completely before editing code.

The current project directory is the source of truth. Do not restore old
milestone files blindly.

Primary stack:
- Go
- Wails v3
- Vue 3 Composition API
- TypeScript
- Tailwind

Current Linux build:
  wails3 build -tags gtk3

Product goal:
- Linux: native DevStack containerd/runc
- macOS: Apple Virtualization.framework + minimal Linux + containerd/runc
- Windows: dedicated DevStack WSL2 guest + containerd/runc
- Docker Desktop must be optional, never a requirement.

Critical UX rules:
- tray icon click opens tray menu only
- main window opens only via Show DevStack
- tray is event-driven, not periodically rebuilt
- tray and main app group Compose projects and Standalone Containers
- do not hide groups with arbitrary limits

Critical engine rule:
Docker Desktop, System Docker, Rootless Docker, custom DOCKER_HOST, and
DevStack containerd are different engines with different container stores.
Never silently switch from one to another during reconnect/recovery.

The next milestone should be Milestone 16.1.4:
"Docker Desktop Optional / Native Engine Default".

Before implementing it:
1. inspect current code
2. run the GTK3 build
3. inspect current engine/runtime settings and endpoint
4. confirm which daemon owns the expected Compose projects

Milestone 16.1.4 goals:
- make DevStack Native the preferred/default engine
- make External Docker an explicit compatibility option
- Docker Desktop only starts if explicitly selected
- users without Docker Desktop must work normally
- preserve existing external Docker settings during migration
- no silent daemon fallback
- keep all current tray behavior
- keep all main-app container groups visible
- provide clear Engine UI showing selected engine/runtime

After implementation:
- gofmt changed Go files
- build frontend
- run wails3 build -tags gtk3
- fix every compile error before reporting completion
- document target-specific macOS/Windows validation limitations
```

---

# 17. Final Current State Summary

The project has progressed from a Docker GUI into a runtime/engine abstraction with:

```text
✓ Docker/Moby container management
✓ Compose grouping
✓ logs
✓ terminal
✓ resource stats
✓ images/volumes/networks
✓ Compose lifecycle commands
✓ tray/menu-bar control
✓ grouped tray containers
✓ event-driven tray refresh
✓ Linux direct containerd
✓ Linux CNI helper/networking
✓ runtime capabilities
✓ native macOS Virtualization.framework foundation
✓ VirtioFS direction
✓ Windows dedicated WSL2 rootfs direction
✓ engine auto-recovery experiments
```

The most important architectural correction now is:

```text
DevStack must stop behaving as though Docker Desktop is the primary engine.

DevStack Native should be primary.
External Docker should be optional.
```

That should be the starting point for the next Codex session.
