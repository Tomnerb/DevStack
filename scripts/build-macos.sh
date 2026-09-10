#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="arm64"
GUEST_ASSETS=""
INSTALL=false
EXTERNAL_ONLY=false

usage() {
  cat <<'EOF'
Usage: ./scripts/build-macos.sh [arm64|amd64] [options]

Options:
  --guest-assets DIR  Bundle DIR/vmlinux and DIR/rootfs.ext4.
                      Defaults to dist/macos-guest when present.
  --install           Install the completed bundle to ~/Applications/DevStack.app.
  --external-only     Build without the native VMM helper and Linux guest assets.
  -h, --help          Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    arm64|amd64)
      ARCH="$1"
      shift
      ;;
    --guest-assets)
      [[ $# -ge 2 ]] || { echo "--guest-assets requires a directory" >&2; exit 2; }
      GUEST_ASSETS="$2"
      shift 2
      ;;
    --install)
      INSTALL=true
      shift
      ;;
    --external-only)
      EXTERNAL_ONLY=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v wails3 >/dev/null 2>&1; then
  echo "wails3 is required." >&2
  echo "Install the project version with:" >&2
  echo "  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.19" >&2
  exit 1
fi

if [[ -z "$GUEST_ASSETS" && -f dist/macos-guest/vmlinux && -f dist/macos-guest/rootfs.ext4 ]]; then
  GUEST_ASSETS="dist/macos-guest"
fi

if [[ "$EXTERNAL_ONLY" == false ]]; then
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "A native-engine macOS build must run on macOS because devstack-vmm links Virtualization.framework." >&2
    echo "Use --external-only for an unsigned cross-build." >&2
    exit 1
  fi

  if [[ -z "$GUEST_ASSETS" || ! -f "$GUEST_ASSETS/vmlinux" || ! -f "$GUEST_ASSETS/rootfs.ext4" ]]; then
    echo "Native macOS guest assets are required." >&2
    echo "Generate them on Linux with scripts/build-macos-guest-assets-linux.sh, then pass:" >&2
    echo "  --guest-assets /path/to/macos-guest" >&2
    echo "Use --external-only to build without DevStack Native." >&2
    exit 1
  fi

  GUEST_ASSETS="$(cd "$GUEST_ASSETS" && pwd)"
  ./scripts/build-macos-vmm.sh
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Configuring Wails cross-build tooling..."
  wails3 task setup:docker
fi

echo "Building and packaging DevStack for macOS/$ARCH..."
wails3 task darwin:package ARCH="$ARCH"

SOURCE_APP="bin/devstack.app"
if [[ ! -d "$SOURCE_APP" ]]; then
  echo "Expected Wails bundle was not created at $SOURCE_APP" >&2
  exit 1
fi

if [[ "$EXTERNAL_ONLY" == false ]]; then
  RESOURCES="$SOURCE_APP/Contents/Resources"
  mkdir -p "$RESOURCES/guest"

  cp -f native/macos/DevStackVMM/.build/release/devstack-vmm "$RESOURCES/devstack-vmm"
  chmod 0755 "$RESOURCES/devstack-vmm"
  cp -f "$GUEST_ASSETS/vmlinux" "$RESOURCES/guest/vmlinux"
  cp -f "$GUEST_ASSETS/rootfs.ext4" "$RESOURCES/guest/rootfs.ext4"
  if [[ -f "$GUEST_ASSETS/manifest.txt" ]]; then
    cp -f "$GUEST_ASSETS/manifest.txt" "$RESOURCES/guest/manifest.txt"
  fi

  codesign \
    --force \
    --sign - \
    --timestamp=none \
    --entitlements native/macos/DevStackVMM/devstack-vmm.entitlements \
    "$RESOURCES/devstack-vmm"
  codesign --force --sign - --timestamp=none "$SOURCE_APP"
fi

DIST_DIR="dist/macos-$ARCH"
DIST_APP="$DIST_DIR/DevStack.app"
mkdir -p "$DIST_DIR"
rm -rf -- "$DIST_APP"
ditto "$SOURCE_APP" "$DIST_APP"

if [[ "$INSTALL" == true ]]; then
  INSTALL_DIR="$HOME/Applications"
  INSTALL_APP="$INSTALL_DIR/DevStack.app"
  mkdir -p "$INSTALL_DIR"
  rm -rf -- "$INSTALL_APP"
  ditto "$DIST_APP" "$INSTALL_APP"
  echo "Installed: $INSTALL_APP"
fi

echo
echo "Built: $DIST_APP"
if [[ "$EXTERNAL_ONLY" == false ]]; then
  echo "Included: devstack-vmm, vmlinux, rootfs.ext4"
  echo "The guest assets are copied to Application Support on first native-engine start."
else
  echo "External Docker-only build; native VM assets were not included."
fi
echo "This development bundle is ad-hoc signed. Configure Developer ID signing/notarization before distribution."
