#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Run this check on macOS."
  exit 1
fi

echo "DevStack native macOS backend"
echo

echo "macOS:"
sw_vers
echo

echo "CPU:"
uname -m
echo

echo "VMM helper:"
for path in \
  "${DEVSTACK_VMM_PATH:-}" \
  "$HOME/.local/libexec/devstack-vmm" \
  "/usr/local/libexec/devstack-vmm" \
  "/opt/homebrew/libexec/devstack-vmm"
do
  [[ -z "$path" ]] && continue
  if [[ -x "$path" ]]; then
    echo "  $path"
    "$path" status || true
    break
  fi
done

echo
echo "Guest assets:"
GUEST="$HOME/Library/Application Support/DevStack/guest"
ls -lh "$GUEST/vmlinux" "$GUEST/rootfs.ext4" 2>/dev/null || true

echo
echo "Runtime proxy:"
RUN="$HOME/Library/Application Support/DevStack/run"
ls -l "$RUN/containerd.sock" 2>/dev/null || echo "  not running"

echo
echo "Docker-compatible native proxy:"
ls -l "$RUN/docker.sock" 2>/dev/null || echo "  not running"
