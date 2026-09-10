#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This helper is for macOS."
  exit 1
fi

if command -v limactl >/dev/null 2>&1; then
  limactl --version
  exit 0
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "Homebrew is not installed. Install Lima manually from lima-vm.io or install Homebrew first."
  exit 1
fi

brew install lima
limactl --version
