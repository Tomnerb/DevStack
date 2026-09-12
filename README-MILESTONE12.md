# Dockiva — Milestone 12: CNI networking + localhost ports

Milestone 12 makes direct containerd practical for local web/API development.

## New for Linux direct containerd

- persistent network namespaces
- CNI bridge network
- host-local IPv4 allocation
- outbound masquerading
- container IP display
- host DNS resolver inside containers
- localhost-only TCP port publishing
- port collision checks
- network teardown when the container is deleted
- network state persists across task stop/start/restart

## Network

```text
name:    dockiva-net
bridge:  dockiva0
subnet:  10.89.0.0/16
gateway: 10.89.0.1
```

The CNI chain uses loopback + bridge/host-local + portmap.

## Privilege design

The Wails app remains unprivileged.

A small root helper (`dockiva-netd`) owns the Linux operations that require
network privileges and exposes only a narrow Unix-socket API at:

```text
/run/dockiva/netd.sock
```

It accepts Dockiva container IDs and localhost TCP mappings only.

## Apply

```bash
chmod +x dockiva-milestone12/apply.sh

./dockiva-milestone12/apply.sh \
  /home/darith/mbanq/dockiva/dockiva
```

## One-time networking install

```bash
cd /home/darith/mbanq/dockiva/dockiva
./scripts/install-containerd-networking.sh
```

The installer may use sudo to install CNI plugins/iproute2, create the
`dockiva` group, install the helper, write the fixed CNI config, and start the
systemd service.

Then:

```bash
./scripts/check-containerd-networking.sh
```

Log out/in after first install for permanent `dockiva` group membership.

## Build

```bash
go mod tidy
wails3 build -tags gtk3
./bin/dockiva
```

## Test

Select:

```text
Engine → Container Runtime → containerd (direct)
```

Then:

```text
Image:   nginx:latest
Name:    test-web
Ports:   8080:80
```

Pull Image, then Create.

The container should get a `10.89.x.x` address and the UI should show:

```text
8080:80/tcp
```

Then open:

```text
http://localhost:8080
```

Multiple mappings:

```text
8080:80,8443:443
```

## Safety restrictions

Milestone 12 direct-containerd port publishing is fixed to:

- host IP `127.0.0.1`
- TCP
- host ports `1024-65535`

Rootless containerd remains supported for containers, but this host CNI bridge
integration is intentionally disabled for rootless containerd.

## DNS

Networked containers use containerd's `oci.WithHostResolvconf`, which mounts
the host resolver configuration read-only. This provides external DNS but not
Docker-style service-name discovery.

## Still pending for direct containerd

- live logs
- xterm exec
- CPU/RAM stats
- service-name DNS
- user-defined CNI networks
- UDP/LAN publishing
- Compose

## Next

Milestone 13: direct-containerd logs, exec terminal, and task metrics.
