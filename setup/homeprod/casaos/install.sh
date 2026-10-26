#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo 'Execute como root.' >&2; exit 1; }

work=$(mktemp -d /tmp/dotfiles-casaos.XXXXXXXX)
stage='preparação'
trap 'code=$?; echo "Falha na etapa: $stage (código $code). Arquivos e log: $work" >&2; exit "$code"' ERR
printf 'Arquivos e log desta instalação: %s\n' "$work"

download() {
  curl --fail --show-error --silent --location --retry 3 --connect-timeout 20 --max-time 180 "$1" -o "$2"
}

stage='download do instalador oficial'
download https://get.casaos.io "$work/install.original.sh"
bash -n "$work/install.original.sh"
# Apply the workaround only to the exact upstream declaration we inspected.
expected='readonly CASA_UNINSTALL_URL="https://get.casaos.io/uninstall/v0.4.16"'
[[ $(grep -Fxc "$expected" "$work/install.original.sh") == 1 ]] || {
  echo 'O instalador oficial mudou; revise o workaround antes de executar.' >&2
  exit 1
}

stage='download do desinstalador (somente armazenamento)'
if ! download https://get.casaos.io/uninstall/v0.4.16 "$work/uninstall.sh"; then
  echo 'Endpoint indisponível; usando cópia fixada do repositório oficial IceWhaleTech/get.'
  download https://raw.githubusercontent.com/IceWhaleTech/get/15d5563aef5e3628f2b2ac65d619d81228ab2f3e/uninstall.sh "$work/uninstall.sh"
  printf '%s  %s\n' f11108f77db04c2936ef65acb578add1f01b09f8f3c907c0a611b5583444334e "$work/uninstall.sh" | sha256sum --check --status
fi
[[ -s "$work/uninstall.sh" ]]
grep -q '^#!.*bash' "$work/uninstall.sh"
bash -n "$work/uninstall.sh"

stage='preparação da cópia local do instalador'
sed "s|^${expected}$|readonly CASA_UNINSTALL_URL=\"file://$work/uninstall.sh\"|" "$work/install.original.sh" > "$work/install.sh"
bash -n "$work/install.sh"
# Ghostty terminfo is not necessarily installed on a fresh VPS.
if ! infocmp "${TERM:-dumb}" >/dev/null 2>&1; then
  export TERM=xterm
fi

stage='execução do instalador CasaOS'
bash "$work/install.sh" 2>&1 | tee "$work/install.log"
printf '\nInstalador CasaOS concluído. Log: %s/install.log\n' "$work"
