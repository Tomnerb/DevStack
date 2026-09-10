#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "Platform:"
go env GOOS GOARCH

echo
echo "ContainerRuntime interface:"
grep -n "type ContainerRuntime interface" runtime_common.go

echo
echo "Selected runtime files:"
go list -f '{{range .GoFiles}}{{println .}}{{end}}' . 2>/dev/null \
  | grep '^runtime_' \
  | sort || true

if [[ "$(go env GOOS)" == "linux" ]]; then
  echo
  echo "containerd:"
  found=0
  for socket in \
    /run/containerd/containerd.sock \
    /var/run/containerd/containerd.sock \
    "${XDG_RUNTIME_DIR:-/nonexistent}/containerd/containerd.sock"
  do
    if [[ -S "$socket" ]]; then
      echo "  socket: $socket"
      found=1
    fi
  done

  if command -v ctr >/dev/null 2>&1; then
    echo "  ctr: $(command -v ctr)"
    found=1
  fi

  if [[ "$found" == "0" ]]; then
    echo "  not detected"
  fi
fi
