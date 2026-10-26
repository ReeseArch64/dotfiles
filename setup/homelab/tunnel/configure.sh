#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
directory=$(dirname "$(readlink -f "$0")")
for tool in docker python3 curl; do command -v "$tool" >/dev/null; done

container() {
  local prefix=$1
  local -a matches=()
  while read -r id image; do
    [[ "$image" == "$prefix":* ]] && matches+=("$id")
  done < <(docker ps -a --format '{{.ID}} {{.Image}}')
  [[ ${#matches[@]} == 1 ]] || { echo "Instale exatamente um app $prefix pelo CasaOS antes de continuar." >&2; return 1; }
  printf '%s\n' "${matches[0]}"
}
compose_path() {
  local path
  path=$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project.config_files" }}' "$1")
  [[ "$path" == /* && "$path" != *,* && -f "$path" && ! -L "$path" ]] || return 1
  realpath -e "$path"
}
vault=$(container vaultwarden/server)
connector=$(container cloudflare/cloudflared)
vault_compose=$(compose_path "$vault")
tunnel_compose=$(compose_path "$connector")
[[ "$vault_compose" != "$tunnel_compose" ]]
image=$(docker inspect -f '{{.Config.Image}}' "$connector")
help=$(docker run --rm --network none --entrypoint cloudflared "$image" tunnel run --help)
grep -q -- '--token-file' <<< "$help" || { echo 'Atualize cloudflared no CasaOS: requer suporte a --token-file (2025.4.0+).' >&2; exit 1; }

# Admin settings override the environment: refuse conflicting values before publishing.
work=$(mktemp -d /etc/homelab-vault-tunnel.XXXXXXXX)
docker exec "$vault" sh -c 'if [ -f /data/config.json ]; then cat /data/config.json; else printf "{}"; fi' > "$work/vault-admin.json"
python3 - "$work/vault-admin.json" <<'PY'
import json, pathlib, sys
p = pathlib.Path(sys.argv[1])
c = json.loads(p.read_text()) if p.exists() else {}
for key, expected in {'domain': 'https://vault.reesearch64.com', 'signups_allowed': False, 'signups_domains_whitelist': ''}.items():
    if key in c and c[key] != expected:
        raise SystemExit(f'Ajuste {key} no painel Vaultwarden antes de continuar; config.json sobrepõe o ambiente.')
PY
printf '\nNa Cloudflare, prepare um Tunnel dedicado ao Vaultwarden, sem rotas para outros serviços.\n'
printf 'Hostname: vault.reesearch64.com -> HTTP http://vaultwarden-origin:80\n'
printf 'Antes da rota HTTP, bloqueie /admin e subcaminhos com ingress path (?i)^/+admin(/.*)?$ -> http_status:403.\n'
printf 'Alternativamente, use uma aplicação Cloudflare Access específica para /admin (Block Everyone).\n'
printf 'Não coloque login Access no restante do hostname, usado pelos clientes Bitwarden.\n'
read -r -p 'Já criou sua conta Vaultwarden e preparou essas regras? Digite CONFIRMO: ' answer
[[ "$answer" == CONFIRMO ]] || exit 1
read -rs -p 'Cole somente o token do Tunnel (entrada oculta): ' token
printf '\n'
[[ "$token" =~ ^[A-Za-z0-9_=+/.-]+$ ]] || { echo 'Token inválido.' >&2; exit 1; }

# A unique credential file allows rollback without overwriting previous tokens.
token_file="$work/tunnel-token"
printf '%s' "$token" > "$token_file"
unset token
chown 65532:65532 "$token_file"
chmod 0400 "$token_file"
cp -a "$vault_compose" "$work/vault.original"
cp -a "$tunnel_compose" "$work/tunnel.original"
normalize() { (cd "$(dirname "$1")" && docker compose -f "$1" config --format json) > "$2"; }
normalize "$vault_compose" "$work/vault.json"
normalize "$tunnel_compose" "$work/tunnel.json"
python3 "$directory/compose.py" vault "$work/vault.json" "$work/vault.new" "$token_file"
python3 "$directory/compose.py" tunnel "$work/tunnel.json" "$work/tunnel.new" "$token_file"
docker compose -f "$work/vault.new" config -q
docker compose -f "$work/tunnel.new" config -q
docker network inspect homelab-vault-tunnel >/dev/null 2>&1 || docker network create homelab-vault-tunnel >/dev/null

rollback() {
  cp -a "$work/vault.original" "$vault_compose"
  cp -a "$work/tunnel.original" "$tunnel_compose"
  docker compose -f "$vault_compose" up -d || true
  docker compose -f "$tunnel_compose" up -d || true
  echo "Falha; Compose anteriores restaurados. Backup: $work" >&2
}
trap rollback ERR
install -m 0600 "$work/vault.new" "$vault_compose"
install -m 0600 "$work/tunnel.new" "$tunnel_compose"
docker compose -f "$vault_compose" up -d
docker compose -f "$tunnel_compose" up -d
curl --fail --silent --show-error --retry 12 --retry-all-errors --retry-delay 3 --connect-timeout 5 --max-time 10 https://vault.reesearch64.com/alive >/dev/null
trap - ERR
echo "Vaultwarden acessível em https://vault.reesearch64.com. Backup: $work"
echo 'Atualize a URL nos clientes Bitwarden e teste login e sincronização fora da LAN.'
