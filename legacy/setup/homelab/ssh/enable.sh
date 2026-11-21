#!/usr/bin/env bash
set -euo pipefail

# Keep the installed configuration, authentication methods and active socket.
install -d -m 0755 /run/sshd
sshd -t
if systemctl is-active --quiet ssh.socket || systemctl is-enabled --quiet ssh.socket; then
    systemctl enable --now ssh.socket
else
    systemctl enable --now ssh.service
fi
