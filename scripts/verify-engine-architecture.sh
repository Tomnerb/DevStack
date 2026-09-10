#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "Platform engine files:"
for file in engine_linux.go engine_darwin.go engine_windows.go; do
  printf "  %-24s " "$file"
  head -n 1 "$file"
done

echo
echo "Current Go platform:"
go env GOOS GOARCH

echo
echo "Files selected for current platform:"
go list -f '{{range .GoFiles}}{{println .}}{{end}}' . \
  | grep -E '^(engine_|docker_)' \
  | sort
