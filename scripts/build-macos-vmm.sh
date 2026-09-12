#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This helper must be built on macOS because it links Virtualization.framework."
  exit 1
fi

ROOT="$(pwd)"
PKG="$ROOT/native/macos/DockivaVMM"

echo "Building native macOS VMM helper..."
swift build \
  --package-path "$PKG" \
  -c release

BIN="$PKG/.build/release/dockiva-vmm"

echo "Ad-hoc signing with virtualization entitlement..."
codesign \
  --force \
  --sign - \
  --timestamp=none \
  --entitlements "$PKG/dockiva-vmm.entitlements" \
  "$BIN"

echo
echo "Built:"
echo "  $BIN"

echo
echo "Validate:"
"$BIN" status
