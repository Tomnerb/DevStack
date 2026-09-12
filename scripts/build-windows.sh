#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-amd64}"

echo "Building Dockiva for Windows/$ARCH..."
wails3 build GOOS=windows GOARCH="$ARCH"

mkdir -p "dist/windows-$ARCH"
cp -f bin/dockiva.exe "dist/windows-$ARCH/dockiva.exe"

echo
echo "Built: dist/windows-$ARCH/dockiva.exe"
echo "For an NSIS installer, run scripts/package-windows.ps1 on Windows."
