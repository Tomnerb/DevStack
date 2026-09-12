#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This helper installer is macOS-only."
  exit 1
fi

BIN="native/macos/DockivaVMM/.build/release/dockiva-vmm"

if [[ ! -x "$BIN" ]]; then
  ./scripts/build-macos-vmm.sh
fi

DEST="$HOME/.local/libexec"
mkdir -p "$DEST"

cp -f "$BIN" "$DEST/dockiva-vmm"
chmod 0755 "$DEST/dockiva-vmm"

echo "Installed:"
echo "  $DEST/dockiva-vmm"
echo
echo "Dockiva automatically checks this path."
