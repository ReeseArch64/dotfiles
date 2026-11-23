# Shared by privileged legacy setup scripts.
dotfiles_backup_category() {
  local root=/mnt/backups/dotfiles
  local category="$root/$1" path
  for path in /mnt "${root%/*}" "$root" "$category"; do
    if [[ -L "$path" || ( -e "$path" && ! -d "$path" ) ]]; then
      printf 'Diretório de backup inválido: %s\n' "$path" >&2
      return 1
    fi
  done
  (umask 022; mkdir -p -- "$root") || return 1
  if [[ ! -e "$category" ]]; then
    mkdir -m 0700 -- "$category" || return 1
  fi
  if [[ -L "$category" || $(stat -c '%u:%a' -- "$category") != "$EUID:700" ]]; then
    printf 'Categoria de backup precisa ser privada e pertencer ao usuário atual: %s\n' "$category" >&2
    return 1
  fi
  printf '%s\n' "$category"
}

dotfiles_lock_directory() {
  local directory="/run/dotfiles-legacy-$1"
  if [[ ! -e "$directory" ]]; then
    mkdir -m 0700 -- "$directory" || [[ -d "$directory" ]] || return 1
  fi
  if [[ -L "$directory" || ! -d "$directory" || $(stat -c '%u:%a' -- "$directory") != "$EUID:700" ]]; then
    printf 'Diretório de execução inválido: %s\n' "$directory" >&2
    return 1
  fi
  exec 9<"$directory"
}
