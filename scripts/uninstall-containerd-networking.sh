#!/usr/bin/env bash
set -euo pipefail

echo "Delete DevStack direct-containerd containers before removing networking."
echo "This script does not uninstall containerd or the OS CNI plugin package."
echo

sudo systemctl disable --now devstack-netd 2>/dev/null || true
sudo rm -f /etc/systemd/system/devstack-netd.service
sudo systemctl daemon-reload

sudo rm -f /usr/local/libexec/devstack-netd
sudo rm -f /etc/cni/net.d/10-devstack.conflist

echo "Removed DevStack networking helper/config."
echo "Existing network namespaces/state are intentionally not force-deleted."
