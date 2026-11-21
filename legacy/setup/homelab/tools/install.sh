#!/usr/bin/env bash
set -Eeuo pipefail

[[ $EUID == 0 ]] || { echo 'Execute como root ou via sudo.' >&2; exit 1; }
. /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo 'Estas ferramentas requerem Ubuntu Server.' >&2; exit 1; }

# Official upstream release, pinned and verified independently of the download.
version=0.25.2
case $(dpkg --print-architecture) in
  amd64)
    arch=x86_64
    sha256=0d9dbfc26068b218e7ed84b104748cadc6e3cf733c0afd35465306fb39b9523c ;;
  arm64)
    arch=arm64
    sha256=005c38b685aaa557e7d646d83a3dadb5024340eeed8c6a2e1949eee6f530de23 ;;
  *) echo 'Arquitetura não suportada: use Ubuntu amd64 ou arm64.' >&2; exit 1 ;;
esac

apt-get update
apt-get install --yes jq ripgrep fd-find fzf bat git ncdu curl ca-certificates tar

# Ubuntu names these binaries fdfind and batcat. Never replace existing commands.
install -d -m 0755 /usr/local/bin
if ! command -v fd >/dev/null 2>&1; then
  ln -s /usr/bin/fdfind /usr/local/bin/fd
fi
if ! command -v bat >/dev/null 2>&1; then
  ln -s /usr/bin/batcat /usr/local/bin/bat
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
archive="lazydocker_${version}_Linux_${arch}.tar.gz"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "https://github.com/jesseduffield/lazydocker/releases/download/v${version}/${archive}" \
  --output "$work/$archive"
(cd "$work" && printf '%s  %s\n' "$sha256" "$archive" | sha256sum --check --status)
tar -xzf "$work/$archive" -C "$work" lazydocker
install -m 0755 "$work/lazydocker" /usr/local/bin/lazydocker

for tool in lazydocker jq rg fd fzf bat git ncdu; do
  command -v "$tool" >/dev/null
done
echo 'Ferramentas instaladas: lazydocker, jq, rg, fd, fzf, bat, git e ncdu.'
echo 'LazyDocker usa o Docker existente do CasaOS; não altera o daemon nem as permissões do socket.'
echo 'Se o acesso ao Docker for negado, confira o grupo docker e entre novamente na sessão.'
