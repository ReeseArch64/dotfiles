#!/usr/bin/env bash
set -euo pipefail

port=$1
lan=$2
connection=${3:-}
ports=("$port")
if command -v sshd >/dev/null 2>&1; then
    config=$(sshd -T)
    while read -r key value rest; do
        if [[ $key == port ]]; then
            ports+=("$value")
        fi
    done <<< "$config"
fi

# Additive rules only: never reset UFW or delete existing admin access.
for ssh_port in "${ports[@]}"; do
    ufw prepend allow from "$lan" to any port "$ssh_port" proto tcp comment 'SSH LAN'
done
ufw prepend allow from "$lan" to any port 80 proto tcp comment 'CasaOS LAN'
for service_port in 53 3001 3428 5000 5050 7575 8887 8888 9001; do
    ufw prepend allow from "$lan" to any port "$service_port" proto tcp comment 'Homelab LAN'
done
ufw prepend allow from "$lan" to any port 53 proto udp comment 'DNS LAN'
if [[ -n $connection ]]; then
    read -r client client_port server server_port <<< "$connection"
    ufw prepend allow from "$client" to any port "$server_port" proto tcp comment 'SSH admin atual'
fi
ufw default deny incoming
ufw default allow outgoing
systemctl enable --now ufw.service
ufw --force enable
