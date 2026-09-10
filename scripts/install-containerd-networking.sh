#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

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

echo "Building devstack-netd..."
go build -o /tmp/devstack-netd ./cmd/devstack-netd

echo "Creating devstack group..."
sudo groupadd -f devstack
sudo usermod -aG devstack "$USER"

echo "Installing helper..."
sudo install -d -m 0755 /usr/local/libexec
sudo install -m 0755 /tmp/devstack-netd /usr/local/libexec/devstack-netd

echo "Installing CNI config..."
sudo install -d -m 0755 /etc/cni/net.d

cat >/tmp/10-devstack.conflist <<'EOF'
{
  "cniVersion": "1.0.0",
  "name": "devstack-net",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "devstack0",
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

sudo install -m 0644 \
  /tmp/10-devstack.conflist \
  /etc/cni/net.d/10-devstack.conflist

echo "Installing systemd unit..."

cat >/tmp/devstack-netd.service <<'EOF'
[Unit]
Description=DevStack containerd CNI network helper
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/libexec/devstack-netd
Restart=on-failure
RestartSec=2
RuntimeDirectory=devstack
RuntimeDirectoryMode=0755
StateDirectory=devstack
StateDirectoryMode=0750
ProtectHome=true
ProtectSystem=strict
PrivateTmp=true
ReadWritePaths=/run /var/lib/devstack /var/lib/cni
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_NET_RAW CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_NET_RAW CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER

[Install]
WantedBy=multi-user.target
EOF

sudo install -m 0644 \
  /tmp/devstack-netd.service \
  /etc/systemd/system/devstack-netd.service

sudo systemctl daemon-reload
sudo systemctl enable --now devstack-netd

if command -v setfacl >/dev/null 2>&1 &&
   [[ -S /run/devstack/netd.sock ]]; then
  sudo setfacl -m "u:$USER:rw" /run/devstack/netd.sock || true
fi

echo
echo "Networking helper installed."
echo "Your user was added to group: devstack"
echo
echo "Log out/in for permanent group access."
echo "For this login session, setfacl was used when available."
echo
echo "Verify:"
echo "  ./scripts/check-containerd-networking.sh"
