#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "Preparing Wails cross-platform toolchain..."
wails3 task setup:docker

echo
echo "=== Windows amd64 ==="
wails3 build GOOS=windows GOARCH=amd64
mkdir -p dist/windows-amd64
cp -f bin/devstack.exe dist/windows-amd64/devstack.exe

echo
echo "=== macOS Apple Silicon ==="
wails3 build GOOS=darwin GOARCH=arm64
mkdir -p dist/macos-arm64
if [[ -f bin/devstack ]]; then
  cp -f bin/devstack dist/macos-arm64/devstack
fi

echo
echo "=== Linux amd64 / GTK3 ==="
wails3 build -tags gtk3 GOOS=linux GOARCH=amd64
mkdir -p dist/linux-amd64
cp -f bin/devstack dist/linux-amd64/devstack

echo
echo "Done:"
find dist -maxdepth 2 -type f -print
