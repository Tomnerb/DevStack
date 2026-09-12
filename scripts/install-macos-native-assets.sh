#!/usr/bin/env bash
set -euo pipefail

SOURCE="${1:-}"

if [[ -z "$SOURCE" ]]; then
  echo "Usage: ./scripts/install-macos-native-assets.sh /path/to/macos-guest"
  exit 1
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This installer copies guest assets into the macOS Dockiva data directory."
  exit 1
fi

if [[ ! -f "$SOURCE/vmlinux" ]] || [[ ! -f "$SOURCE/rootfs.ext4" ]]; then
  echo "Expected:"
  echo "  $SOURCE/vmlinux"
  echo "  $SOURCE/rootfs.ext4"
  exit 1
fi

DEST="$HOME/Library/Application Support/Dockiva/guest"

mkdir -p "$DEST"

cp -f "$SOURCE/vmlinux" "$DEST/vmlinux"
cp -f "$SOURCE/rootfs.ext4" "$DEST/rootfs.ext4"

if [[ -f "$SOURCE/manifest.txt" ]]; then
  cp -f "$SOURCE/manifest.txt" "$DEST/manifest.txt"
fi

echo "Installed Dockiva native macOS guest assets:"
ls -lh "$DEST/vmlinux" "$DEST/rootfs.ext4"
