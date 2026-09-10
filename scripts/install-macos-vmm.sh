#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This helper installer is macOS-only."
  exit 1
fi

BIN="native/macos/DevStackVMM/.build/release/devstack-vmm"

if [[ ! -x "$BIN" ]]; then
  ./scripts/build-macos-vmm.sh
fi

DEST="$HOME/.local/libexec"
mkdir -p "$DEST"

cp -f "$BIN" "$DEST/devstack-vmm"
chmod 0755 "$DEST/devstack-vmm"

echo "Installed:"
echo "  $DEST/devstack-vmm"
echo
echo "DevStack automatically checks this path."
