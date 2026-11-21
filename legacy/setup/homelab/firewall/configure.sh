#!/usr/bin/env bash
set -euo pipefail

# Keep the existing CLI argument order: SSH port, LAN, current SSH connection.
lan=$2
connection=${3:-}

# Trust only the detected LAN, before any existing deny rules.
# Additive rules only: never reset UFW or delete existing admin access.
ufw prepend allow from "$lan" to any comment 'Homelab LAN'
if [[ -n $connection ]]; then
    read -r client client_port server server_port <<< "$connection"
    ufw prepend allow from "$client" to any port "$server_port" proto tcp comment 'SSH admin atual'
fi
ufw default deny incoming
ufw default allow outgoing
systemctl enable --now ufw.service
ufw --force enable
