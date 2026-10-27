#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
work=$(mktemp -d /etc/homeprod-vault-tunnel.XXXXXXXX)
exec 2> >(tee -a "$work/configure.log" >&2)
phase='verificar dependências'
changed=false
on_error() {
  local status=$1 line=$2
  trap - ERR
  printf '\nFalha na etapa "%s", linha %s, código %s. Log: %s/configure.log\n' "$phase" "$line" "$status" "$work" >&2
  if $changed; then
    set +e
    rollback
  fi
  exit "$status"
}
trap 'on_error "$?" "$LINENO"' ERR
echo "Diagnóstico desta execução: $work/configure.log" >&2
directory=$(dirname "$(readlink -f "$0")")
for tool in docker python3 curl; do command -v "$tool" >/dev/null; done

container() {
  local prefix image id
  local -a matches=()
  while read -r id image; do
    image=${image#docker.io/}
    image=${image#index.docker.io/}
    for prefix in "$@"; do
      case "$image" in
        "$prefix"|"$prefix":*|"$prefix"@*) matches+=("$id"); break ;;
      esac
    done
  done < <(docker ps -a --format '{{.ID}} {{.Image}}')
  [[ ${#matches[@]} == 1 ]] || { echo "Esperado exatamente um app CasaOS com imagem $*; encontrados: ${#matches[@]}." >&2; return 1; }
  printf '%s\n' "${matches[0]}"
}
compose_path() {
  local path
  path=$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project.config_files" }}' "$1")
  [[ "$path" == /* && "$path" != *,* && -f "$path" && ! -L "$path" ]] || {
    echo "Compose ausente, múltiplo ou inválido para o container $1." >&2; return 1;
  }
  realpath -e "$path"
}
phase='identificar containers e arquivos Compose do CasaOS'
vault=$(container vaultwarden/server)
connector=$(container cloudflare/cloudflared wisdomsky/cloudflared-web)
vault_compose=$(compose_path "$vault")
tunnel_compose=$(compose_path "$connector")
[[ "$vault_compose" != "$tunnel_compose" ]]
vault_project=$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project" }}' "$vault")
tunnel_project=$(docker inspect -f '{{ index .Config.Labels "com.docker.compose.project" }}' "$connector")
for project in "$vault_project" "$tunnel_project"; do
  [[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || { echo 'Projeto Compose CasaOS ausente ou inválido.' >&2; exit 1; }
done
compose_run() {
  local file=$1 project=$2
  shift 2
  (cd "$(dirname "$file")" && AppID="$project" docker compose -p "$project" -f "$file" "$@")
}
phase='verificar suporte do cloudflared a token-file'
image=$(docker inspect -f '{{.Config.Image}}' "$connector")
help=$(docker run --rm --network none --entrypoint cloudflared "$image" tunnel run --help)
grep -q -- '--token-file' <<< "$help" || { echo 'Atualize cloudflared no CasaOS: requer suporte a --token-file (2025.4.0+).' >&2; exit 1; }

# Admin settings override the environment: refuse conflicting values before publishing.
phase='verificar configurações administrativas do Vaultwarden'
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
phase='confirmar preparação e ler token'
read -r -p 'Já criou sua conta Vaultwarden e preparou essas regras? Digite CONFIRMO: ' answer
[[ "$answer" == CONFIRMO ]] || { echo 'Cancelado: confirmação diferente de CONFIRMO.' >&2; exit 1; }
read -rs -p 'Cole somente o token do Tunnel (entrada oculta): ' token
printf '\n'
[[ "$token" =~ ^[A-Za-z0-9_=+/.-]+$ ]] || { echo 'Token inválido.' >&2; exit 1; }

# A unique credential file allows rollback without overwriting previous tokens.
phase='salvar credencial e backups locais'
token_file="$work/tunnel-token"
printf '%s' "$token" > "$token_file"
unset token
chown 65532:65532 "$token_file"
chmod 0400 "$token_file"
cp -a "$vault_compose" "$work/vault.original"
cp -a "$tunnel_compose" "$work/tunnel.original"
normalize() { compose_run "$1" "$3" config --format json > "$2"; }
phase='normalizar Compose do Vaultwarden'
normalize "$vault_compose" "$work/vault.json" "$vault_project"
phase='normalizar Compose do Cloudflared'
normalize "$tunnel_compose" "$work/tunnel.json" "$tunnel_project"
phase='verificar volume persistente do Vaultwarden'
docker inspect -f '{{json .Mounts}}' "$vault" > "$work/vault-mounts.json"
phase='transformar Compose para o Tunnel'
python3 "$directory/compose.py" vault "$work/vault.json" "$work/vault.new" "$token_file" "$work/vault-mounts.json"
python3 "$directory/compose.py" tunnel "$work/tunnel.json" "$work/tunnel.new" "$token_file"
phase='validar Compose do Vaultwarden'
compose_run "$work/vault.new" "$vault_project" config -q
phase='validar Compose do Cloudflared'
compose_run "$work/tunnel.new" "$tunnel_project" config -q
phase='preparar rede Docker'
docker network inspect homeprod-vault-tunnel >/dev/null 2>&1 || docker network create homeprod-vault-tunnel >/dev/null

rollback() {
  cp -a "$work/vault.original" "$vault_compose"
  cp -a "$work/tunnel.original" "$tunnel_compose"
  compose_run "$vault_compose" "$vault_project" up -d || true
  compose_run "$tunnel_compose" "$tunnel_project" up -d || true
  echo "Falha; Compose anteriores restaurados. Backup: $work" >&2
}
phase='aplicar Compose'
changed=true
install -m 0600 "$work/vault.new" "$vault_compose"
install -m 0600 "$work/tunnel.new" "$tunnel_compose"
phase='iniciar Vaultwarden'
compose_run "$vault_compose" "$vault_project" up -d
phase='iniciar Cloudflared'
compose_run "$tunnel_compose" "$tunnel_project" up -d
phase='testar HTTPS público do Vaultwarden'
curl --fail --silent --show-error --retry 12 --retry-all-errors --retry-delay 3 --connect-timeout 5 --max-time 10 https://vault.reesearch64.com/alive >/dev/null
trap - ERR
echo "Vaultwarden acessível em https://vault.reesearch64.com. Backup: $work"
echo 'Atualize a URL nos clientes Bitwarden e teste login e sincronização fora da LAN.'
