#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "Build the DevStack WSL rootfs on Linux or Linux CI."
  exit 1
fi

ALPINE_VERSION="${ALPINE_VERSION:-3.24.1}"
CONTAINERD_VERSION="${CONTAINERD_VERSION:-2.3.1}"
RUNC_VERSION="${RUNC_VERSION:-1.5.1}"
CNI_VERSION="${CNI_VERSION:-1.9.1}"

OUT="${OUT:-$PWD/dist/windows-wsl}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$OUT" "$WORK/rootfs"

curl -fL "https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/x86_64/alpine-minirootfs-${ALPINE_VERSION}-x86_64.tar.gz" -o "$WORK/alpine.tar.gz"
tar -xzf "$WORK/alpine.tar.gz" -C "$WORK/rootfs"

curl -fL "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz" -o "$WORK/containerd.tar.gz"
tar -xzf "$WORK/containerd.tar.gz" -C "$WORK"

mkdir -p "$WORK/rootfs/usr/local/bin" "$WORK/rootfs/usr/local/sbin" "$WORK/rootfs/opt/cni/bin"
for bin in containerd containerd-shim-runc-v2 ctr; do
  install -m 0755 "$WORK/bin/$bin" "$WORK/rootfs/usr/local/bin/$bin"
done

curl -fL "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.amd64" -o "$WORK/rootfs/usr/local/bin/runc"
chmod 0755 "$WORK/rootfs/usr/local/bin/runc"

curl -fL "https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-amd64-v${CNI_VERSION}.tgz" -o "$WORK/cni.tgz"
tar -xzf "$WORK/cni.tgz" -C "$WORK/rootfs/opt/cni/bin"

install -m 0755 native/windows/devstack-start "$WORK/rootfs/usr/local/sbin/devstack-start"

tar -C "$WORK/rootfs" -cf "$OUT/devstack-wsl-rootfs.tar" .

echo
echo "Created:"
ls -lh "$OUT/devstack-wsl-rootfs.tar"
