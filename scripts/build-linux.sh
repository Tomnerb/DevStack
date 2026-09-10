#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-amd64}"

echo "Building DevStack for Linux/$ARCH with legacy GTK3..."
wails3 build -tags gtk3 GOOS=linux GOARCH="$ARCH"

mkdir -p "dist/linux-$ARCH"
cp -f bin/devstack "dist/linux-$ARCH/devstack"

echo
echo "Built: dist/linux-$ARCH/devstack"
