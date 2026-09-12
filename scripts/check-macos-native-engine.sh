#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Run this check on macOS."
  exit 1
fi

echo "Dockiva native macOS backend"
echo

echo "macOS:"
sw_vers
echo

echo "CPU:"
uname -m
echo

echo "VMM helper:"
for path in \
  "${DOCKIVA_VMM_PATH:-}" \
  "$HOME/.local/libexec/dockiva-vmm" \
  "/usr/local/libexec/dockiva-vmm" \
  "/opt/homebrew/libexec/dockiva-vmm"
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
GUEST="$HOME/Library/Application Support/Dockiva/guest"
ls -lh "$GUEST/vmlinux" "$GUEST/rootfs.ext4" 2>/dev/null || true

echo
echo "Runtime proxy:"
RUN="$HOME/Library/Application Support/Dockiva/run"
ls -l "$RUN/containerd.sock" 2>/dev/null || echo "  not running"

echo
echo "Docker-compatible native proxy:"
ls -l "$RUN/docker.sock" 2>/dev/null || echo "  not running"
