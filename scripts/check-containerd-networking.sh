#!/usr/bin/env bash
set -euo pipefail

echo "Dockiva containerd networking"
echo

echo "User:"
id
echo

echo "Helper socket:"
if [[ -S /run/dockiva/netd.sock ]]; then
  ls -l /run/dockiva/netd.sock
else
  echo "  missing: /run/dockiva/netd.sock"
fi

echo
echo "CNI config:"
if [[ -f /etc/cni/net.d/10-dockiva.conflist ]]; then
  echo "  ready: /etc/cni/net.d/10-dockiva.conflist"
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
ip -brief link show dockiva0 2>/dev/null ||
  echo "  dockiva0 will appear after the first networked direct-containerd container"

echo
echo "Service:"
systemctl --no-pager --full status dockiva-netd 2>/dev/null | head -20 || true
