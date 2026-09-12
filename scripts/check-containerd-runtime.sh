#!/usr/bin/env bash
set -euo pipefail

echo "Dockiva containerd runtime check"
echo

echo "User:"
id
echo

echo "Known sockets:"
found=0

if [[ -n "${XDG_RUNTIME_DIR:-}" ]] && [[ -f "$XDG_RUNTIME_DIR/containerd-rootless/child_pid" ]]; then
  pid="$(cat "$XDG_RUNTIME_DIR/containerd-rootless/child_pid" 2>/dev/null || true)"
  if [[ -n "$pid" ]]; then
    rootless="/proc/$pid/root/run/containerd/containerd.sock"
    if [[ -S "$rootless" ]]; then
      echo "  rootless: $rootless"
      found=1
    fi
  fi
fi

for socket in \
  /run/containerd/containerd.sock \
  /var/run/containerd/containerd.sock
do
  if [[ -S "$socket" ]]; then
    echo "  system:   $socket"
    ls -l "$socket"
    found=1
  fi
done

if [[ "$found" == "0" ]]; then
  echo "  none detected"
fi

echo
if command -v ctr >/dev/null 2>&1; then
  echo "ctr: $(command -v ctr)"
  echo "version:"
  ctr version 2>/dev/null || true

  echo
  echo "namespaces (may require socket permission):"
  ctr namespaces list 2>/dev/null || true
else
  echo "ctr: not found"
fi

echo
echo "Dockiva direct runtime always uses namespace: dockiva"
