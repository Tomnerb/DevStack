#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This asset builder is designed to run on Linux."
  echo "Build the guest image in Linux CI or on a Linux machine, then copy it to the Mac."
  exit 1
fi

for cmd in curl tar gzip zstd mke2fs go sha256sum; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd"
    exit 1
  fi
done

ALPINE_VERSION="${ALPINE_VERSION:-3.24.1}"
CONTAINERD_VERSION="${CONTAINERD_VERSION:-2.3.1}"
RUNC_VERSION="${RUNC_VERSION:-1.5.1}"
CNI_VERSION="${CNI_VERSION:-1.9.1}"

OUT="${OUT:-$PWD/dist/macos-guest}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$OUT" "$WORK/rootfs"

echo "Building devstack-guestd for linux/arm64..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build \
  -trimpath \
  -ldflags="-s -w" \
  -o "$WORK/devstack-guestd" \
  ./cmd/devstack-guestd

echo "Downloading Alpine minirootfs $ALPINE_VERSION..."
curl -fL \
  "https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/aarch64/alpine-minirootfs-${ALPINE_VERSION}-aarch64.tar.gz" \
  -o "$WORK/alpine.tar.gz"

curl -fL \
  "https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/aarch64/alpine-minirootfs-${ALPINE_VERSION}-aarch64.tar.gz.sha256" \
  -o "$WORK/alpine.sha256"

(
  cd "$WORK"
  sha256sum -c alpine.sha256
)

tar -xzf "$WORK/alpine.tar.gz" -C "$WORK/rootfs"

echo "Downloading containerd $CONTAINERD_VERSION..."
curl -fL \
  "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-arm64.tar.gz" \
  -o "$WORK/containerd.tar.gz"

tar -xzf "$WORK/containerd.tar.gz" -C "$WORK"

mkdir -p "$WORK/rootfs/usr/local/bin"

for binary in \
  containerd \
  containerd-shim-runc-v2 \
  ctr
do
  install -m 0755 \
    "$WORK/bin/$binary" \
    "$WORK/rootfs/usr/local/bin/$binary"
done

echo "Downloading runc $RUNC_VERSION..."
curl -fL \
  "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.arm64" \
  -o "$WORK/rootfs/usr/local/bin/runc"
chmod 0755 "$WORK/rootfs/usr/local/bin/runc"

echo "Downloading CNI plugins $CNI_VERSION..."
mkdir -p "$WORK/rootfs/opt/cni/bin"

curl -fL \
  "https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-arm64-v${CNI_VERSION}.tgz" \
  -o "$WORK/cni.tgz"

tar -xzf "$WORK/cni.tgz" -C "$WORK/rootfs/opt/cni/bin"

install -m 0755 \
  "$WORK/devstack-guestd" \
  "$WORK/rootfs/usr/local/bin/devstack-guestd"

install -m 0755 \
  native/macos/devstack-init \
  "$WORK/rootfs/sbin/devstack-init"

mkdir -p \
  "$WORK/rootfs/proc" \
  "$WORK/rootfs/sys" \
  "$WORK/rootfs/dev" \
  "$WORK/rootfs/run" \
  "$WORK/rootfs/var/log" \
  "$WORK/rootfs/var/lib/containerd" \
  "$WORK/rootfs/etc/containerd"

echo "Creating sparse ext4 rootfs..."
ROOTFS="$OUT/rootfs.ext4"

rm -f "$ROOTFS"
truncate -s "${DEVSTACK_GUEST_DISK_SIZE:-2G}" "$ROOTFS"

mke2fs \
  -q \
  -t ext4 \
  -F \
  -L devstack-root \
  -d "$WORK/rootfs" \
  "$ROOTFS"

echo "Fetching Apple-style optimized Kata kernel..."
KATA_VERSION="${KATA_VERSION:-3.28.0}"
KATA_URL="${KATA_URL:-https://github.com/kata-containers/kata-containers/releases/download/${KATA_VERSION}/kata-static-${KATA_VERSION}-arm64.tar.zst}"
KATA_BINARY="${KATA_BINARY:-opt/kata/share/kata-containers/vmlinux-6.18.15-186}"

curl -fL "$KATA_URL" -o "$WORK/kata.tar.zst"

mkdir -p "$WORK/kata"

tar \
  --use-compress-program=unzstd \
  -xf "$WORK/kata.tar.zst" \
  -C "$WORK/kata" \
  "$KATA_BINARY"

cp \
  "$WORK/kata/$KATA_BINARY" \
  "$OUT/vmlinux"

chmod 0644 "$OUT/vmlinux" "$ROOTFS"

cat >"$OUT/manifest.txt" <<EOF
DevStack macOS guest assets

Alpine:     $ALPINE_VERSION
containerd: $CONTAINERD_VERSION
runc:       $RUNC_VERSION
CNI:        $CNI_VERSION
Kata:       $KATA_VERSION
Kernel:     $KATA_BINARY

Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo
echo "Guest assets ready:"
ls -lh "$OUT/vmlinux" "$OUT/rootfs.ext4" "$OUT/manifest.txt"

echo
echo "Copy these files to the Mac with:"
echo "  scripts/install-macos-native-assets.sh $OUT"
