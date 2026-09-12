#!/usr/bin/env bash
set -euo pipefail

PATCH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="${1:-}"

if [[ -z "$TARGET" ]]; then
  echo "Usage: ./apply.sh /path/to/dockiva"
  exit 1
fi

if [[ ! -f "$TARGET/docker_service.go" ]] || [[ ! -f "$TARGET/frontend/src/App.vue" ]]; then
  echo "Error: '$TARGET' does not look like the Dockiva project."
  exit 1
fi

cp "$TARGET/docker_service.go" "$TARGET/docker_service.go.bak"
cp "$TARGET/frontend/src/App.vue" "$TARGET/frontend/src/App.vue.bak"

cp "$PATCH_DIR/docker_service.go" "$TARGET/docker_service.go"
cp "$PATCH_DIR/frontend/src/App.vue" "$TARGET/frontend/src/App.vue"

echo "Milestone 2 patch applied."
echo "Backups:"
echo "  $TARGET/docker_service.go.bak"
echo "  $TARGET/frontend/src/App.vue.bak"
echo
echo "Next:"
echo "  cd \"$TARGET\""
echo "  go mod tidy"
echo "  wails3 build -tags gtk3"
echo "  ./bin/dockiva"
