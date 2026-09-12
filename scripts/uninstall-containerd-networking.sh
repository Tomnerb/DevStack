#!/usr/bin/env bash
set -euo pipefail

echo "Delete Dockiva direct-containerd containers before removing networking."
echo "This script does not uninstall containerd or the OS CNI plugin package."
echo

sudo systemctl disable --now dockiva-netd 2>/dev/null || true
sudo rm -f /etc/systemd/system/dockiva-netd.service
sudo systemctl daemon-reload

sudo rm -f /usr/local/libexec/dockiva-netd
sudo rm -f /etc/cni/net.d/10-dockiva.conflist

echo "Removed Dockiva networking helper/config."
echo "Existing network namespaces/state are intentionally not force-deleted."
