#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This packaging script is intended to run on macOS."
  echo "Use scripts/build-macos.sh for an unsigned cross-build."
  exit 1
fi

echo "Packaging universal Dockiva.app (Apple Silicon + Intel)..."
wails3 task darwin:package:universal

echo
echo "macOS application bundle is under bin/."
echo "Configure signing/notarization with: wails3 setup signing"
