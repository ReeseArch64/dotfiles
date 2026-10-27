#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

size_gib=${1:-2}
case "$size_gib" in
  2) ;;
  *) echo 'Informe 2 GiB.' >&2; exit 1 ;;
esac
[[ $EUID == 0 ]] || { echo 'Execute como root.' >&2; exit 1; }

swap_file=/swapfile-homelab
bytes=$((size_gib * 1024 * 1024 * 1024))
[[ -f /etc/fstab && ! -L /etc/fstab && ! -L "$swap_file" ]] || {
  echo 'fstab ausente ou caminho com link simbólico; nenhuma alteração.' >&2; exit 1;
}
# Serialize repeated runs, including the fstab update.
exec 9>/run/lock/dotfiles-homelab-swap.lock
flock -n 9 || { echo 'Outra configuração de swap está em execução.' >&2; exit 1; }

case $(findmnt -n -o FSTYPE --target /) in
  ext4|xfs) ;;
  *) echo 'Este instalador requer ext4 ou XFS; sistema de arquivos não alterado.' >&2; exit 1 ;;
esac

entry=$(awk -v path="$swap_file" '$1 == path {print}' /etc/fstab)
if [[ -n "$entry" && "$entry" != "$swap_file none swap sw,nofail 0 0" ]]; then
  echo 'Entrada fstab existente difere da gerenciada; revise antes de continuar.' >&2
  exit 1
fi

if [[ -e "$swap_file" ]]; then
  [[ -f "$swap_file" && $(stat -c %h "$swap_file") == 1 && $(stat -c %u "$swap_file") == 0 ]] || exit 1
  [[ $(stat -c %s "$swap_file") == "$bytes" && $(blkid -p -s TYPE -o value "$swap_file") == swap ]] || {
    echo 'Arquivo existente incompatível; não será sobrescrito nem redimensionado.' >&2; exit 1;
  }
else
  available=$(df -B1 --output=avail / | tail -n 1 | tr -d ' ')
  # Leave at least 4 GiB free after allocating the swap file.
  (( available >= bytes + 4 * 1024 * 1024 * 1024 )) || {
    echo 'Espaço insuficiente: é necessário reservar mais 4 GiB livres além da swap.' >&2; exit 1;
  }
  temporary=$(mktemp /swapfile-homelab.XXXXXXXX)
  trap 'echo "Criação interrompida; arquivo temporário preservado em $temporary" >&2' ERR
  # Fully write the file: sparse files and holes cannot be used for swap.
  dd if=/dev/zero of="$temporary" bs=1M count=$((size_gib * 1024)) conv=fsync status=progress
  chmod 0600 "$temporary"
  mkswap "$temporary"
  # Do not replace a file created concurrently by another program.
  ln "$temporary" "$swap_file"
  unlink "$temporary"
  trap - ERR
fi

chmod 0600 "$swap_file"
active=$(swapon --show=NAME --noheadings --raw)
if ! grep -Fxq "$swap_file" <<< "$active"; then
  swapon "$swap_file"
fi

# Persist only after the kernel has successfully activated this file.
if [[ -z "$entry" ]]; then
  backup=$(mktemp /etc/fstab.homelab-swap.XXXXXXXX)
  cp -p /etc/fstab "$backup"
  printf '\n%s none swap sw,nofail 0 0\n' "$swap_file" >> /etc/fstab
  printf 'Backup do fstab: %s\n' "$backup"
fi
systemctl daemon-reload
swapon --show
free -h
printf '\nSwap Homelab: %s GiB, ativa e configurada para o próximo boot.\n' "$size_gib"
