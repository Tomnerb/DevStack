#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# pkexec runs this script as root and may strip both $USER and PKEXEC_UID.
# Resolve the logged-in graphical account so the helper socket works in the
# current GUI session, without requiring a logout after installation.
desktop_user="${SUDO_USER:-}"
if [[ -z "$desktop_user" || "$desktop_user" == "root" ]]; then
  desktop_user=""
  while read -r _session _uid candidate _seat _rest; do
    if [[ -n "$candidate" && "$candidate" != "root" ]]; then
      desktop_user="$candidate"
      break
    fi
  done < <(loginctl list-sessions --no-legend 2>/dev/null || true)
fi
if [[ -z "$desktop_user" && -n "${PKEXEC_UID:-}" ]]; then
  desktop_user="$(id -nu "$PKEXEC_UID")"
fi
if [[ -z "$desktop_user" ]]; then
  echo "Could not determine the desktop user for Dockiva helper access."
  exit 1
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This helper is Linux-only."
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "Milestone 12 networking installer currently requires systemd."
  exit 1
fi

plugins_ready() {
  for dir in /opt/cni/bin /usr/lib/cni /usr/libexec/cni /usr/local/lib/cni; do
    if [[ -x "$dir/bridge" ]] &&
       [[ -x "$dir/host-local" ]] &&
       [[ -x "$dir/loopback" ]] &&
       [[ -x "$dir/portmap" ]]; then
      return 0
    fi
  done
  return 1
}

if plugins_ready; then
  echo "CNI plugins already installed."
else
  echo "Installing CNI plugins and iproute2..."

  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y containernetworking-plugins iproute2
  elif command -v dnf >/dev/null 2>&1; then
    sudo dnf install -y containernetworking-plugins iproute
  elif command -v pacman >/dev/null 2>&1; then
    sudo pacman -S --needed cni-plugins iproute2
  else
    echo "Unsupported package manager."
    echo "Install bridge, host-local, loopback, portmap CNI plugins and iproute2 manually."
    exit 1
  fi
fi

echo "Building dockiva-netd..."
# The installer may be launched through pkexec, where root cannot necessarily
# read the checkout's Git metadata. The helper does not need VCS stamping.
helper_build="$(mktemp /tmp/dockiva-netd.XXXXXX)"
trap 'rm -f "$helper_build"' EXIT
go build -buildvcs=false -o "$helper_build" ./cmd/dockiva-netd

echo "Creating dockiva group..."
sudo groupadd -f dockiva
sudo usermod -aG dockiva "$desktop_user"

echo "Installing helper..."
sudo install -d -m 0755 /usr/local/libexec
sudo install -m 0755 "$helper_build" /usr/local/libexec/dockiva-netd

echo "Installing CNI config..."
sudo install -d -m 0755 /etc/cni/net.d
# The hardened helper service explicitly permits this CNI state path. Ensure it
# exists before systemd creates the helper's mount namespace.
sudo install -d -m 0755 /var/lib/cni

cat <<'EOF' | sudo tee /etc/cni/net.d/10-dockiva.conflist >/dev/null
{
  "cniVersion": "1.0.0",
  "name": "dockiva-net",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "dockiva0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "promiscMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [
          [
            {
              "subnet": "10.89.0.0/16",
              "gateway": "10.89.0.1"
            }
          ]
        ],
        "routes": [
          {
            "dst": "0.0.0.0/0"
          }
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": {
        "portMappings": true
      }
    }
  ]
}
EOF
	sudo chmod 0644 /etc/cni/net.d/10-dockiva.conflist

echo "Installing systemd unit..."

cat <<'EOF' | sudo tee /etc/systemd/system/dockiva-netd.service >/dev/null
[Unit]
Description=Dockiva containerd CNI network helper
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/libexec/dockiva-netd
Restart=on-failure
RestartSec=2
RuntimeDirectory=dockiva
RuntimeDirectoryMode=0755
StateDirectory=dockiva
StateDirectoryMode=0750
ProtectHome=true
ProtectSystem=strict
PrivateTmp=true
ReadWritePaths=/run /var/lib/dockiva /var/lib/cni
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_NET_RAW CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_NET_RAW CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER

[Install]
WantedBy=multi-user.target
EOF
	sudo chmod 0644 /etc/systemd/system/dockiva-netd.service

sudo systemctl daemon-reload
# `enable --now` does not replace an already-running helper after an upgrade.
# Restart so the newly installed binary and API are immediately active.
sudo systemctl enable dockiva-netd
sudo systemctl restart dockiva-netd

if command -v setfacl >/dev/null 2>&1; then
  # systemd can return from restart before the helper has created its socket.
  # Wait briefly so a first-run GUI session gets access immediately.
  for _attempt in 1 2 3 4 5; do
    if [[ -S /run/dockiva/netd.sock ]]; then
      sudo setfacl -m "u:$desktop_user:rw" /run/dockiva/netd.sock || true
      break
    fi
    sleep 1
  done
fi

echo
echo "Networking helper installed."
echo "User $desktop_user was added to group: dockiva"
echo
echo "Log out/in for permanent group access."
echo "For this login session, setfacl was used when available."
echo
echo "Verify:"
echo "  ./scripts/check-containerd-networking.sh"
