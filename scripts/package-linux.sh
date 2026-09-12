#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-amd64}"

echo "Packaging Dockiva for Linux/$ARCH..."
echo "Using GTK3 because this is the stack confirmed working on the current Linux machine."

wails3 package GOOS=linux GOARCH="$ARCH" EXTRA_TAGS=gtk3

echo
echo "Linux packages are under bin/ and the Wails Linux packaging directories."
