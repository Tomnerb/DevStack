#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-amd64}"

echo "Building DevStack for Windows/$ARCH..."
wails3 build GOOS=windows GOARCH="$ARCH"

mkdir -p "dist/windows-$ARCH"
cp -f bin/devstack.exe "dist/windows-$ARCH/devstack.exe"

echo
echo "Built: dist/windows-$ARCH/devstack.exe"
echo "For an NSIS installer, run scripts/package-windows.ps1 on Windows."
