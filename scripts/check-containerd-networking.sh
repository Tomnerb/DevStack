#!/usr/bin/env bash
set -euo pipefail

echo "DevStack containerd networking"
echo

echo "User:"
id
echo

echo "Helper socket:"
if [[ -S /run/devstack/netd.sock ]]; then
  ls -l /run/devstack/netd.sock
else
  echo "  missing: /run/devstack/netd.sock"
fi

echo
echo "CNI config:"
if [[ -f /etc/cni/net.d/10-devstack.conflist ]]; then
  echo "  ready: /etc/cni/net.d/10-devstack.conflist"
else
  echo "  missing"
fi

echo
echo "CNI plugins:"
for plugin in bridge host-local loopback portmap; do
  found=""
  for dir in /opt/cni/bin /usr/lib/cni /usr/libexec/cni /usr/local/lib/cni; do
    if [[ -x "$dir/$plugin" ]]; then
      found="$dir/$plugin"
      break
    fi
  done
  printf "  %-12s %s\n" "$plugin" "${found:-missing}"
done

echo
echo "Bridge:"
ip -brief link show devstack0 2>/dev/null ||
  echo "  devstack0 will appear after the first networked direct-containerd container"

echo
echo "Service:"
systemctl --no-pager --full status devstack-netd 2>/dev/null | head -20 || true
