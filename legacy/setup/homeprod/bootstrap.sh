#!/usr/bin/env bash
set -Eeuo pipefail

if (( EUID != 0 )); then
  printf 'erro: execute a preparação inicial como root\n' >&2
  exit 1
fi

admin_user="${1:-alsgy}"
if [[ "$admin_user" != "alsgy" ]]; then
  printf 'erro: o administrador do HomeProd deve ser alsgy\n' >&2
  exit 1
fi

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get upgrade --yes
DEBIAN_FRONTEND=noninteractive apt-get install --yes sudo

if ! getent passwd "$admin_user" >/dev/null; then
  printf '\nCrie agora a senha de sudo para %s. Ela não será armazenada pelo dotfiles.\n\n' "$admin_user"
  adduser --gecos '' --shell /bin/bash "$admin_user"
fi

usermod --append --groups sudo "$admin_user"

if ! id --groups --name "$admin_user" | tr ' ' '\n' | grep -Fxq sudo; then
  printf 'erro: não foi possível adicionar %s ao grupo sudo\n' "$admin_user" >&2
  exit 1
fi

printf '\nUsuário %s preparado com acesso administrativo via sudo.\n' "$admin_user"

DEBIAN_FRONTEND=noninteractive apt-get install --yes curl ca-certificates
printf '\nInstalando CasaOS pelo instalador oficial...\n'
bash "$(dirname "$(readlink -f "$0")")/casaos/install.sh"
