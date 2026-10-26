#!/usr/bin/env bash
set -euo pipefail

domain=${1:-homelab.reesearch64.com}
lan=${2:?informe a rede local em CIDR}
backup_root=${3:-/mnt/backups}
transformer=${4:?informe o transformador de Compose}
caddyfile_template=${5:?informe o Caddyfile}

caddy_version=2.11.7
cloudflare_module=github.com/caddy-dns/cloudflare@v0.2.4
network=homelab-proxy

fail() {
    printf 'erro: %s\n' "$*" >&2
    exit 1
}

for command in docker curl hostname python3 realpath ss ufw; do
    command -v "$command" >/dev/null 2>&1 || fail "comando obrigatório ausente: $command"
done
[[ $EUID == 0 ]] || fail 'esta ação precisa ser executada com sudo'
[[ -f $transformer && ! -L $transformer ]] || fail "transformador inválido: $transformer"
[[ -f $caddyfile_template && ! -L $caddyfile_template ]] || fail "Caddyfile inválido: $caddyfile_template"
python3 - "$lan" <<'PY' || fail "LAN IPv4 privada inválida: $lan"
import ipaddress, sys
network = ipaddress.ip_network(sys.argv[1], strict=False)
raise SystemExit(not isinstance(network, ipaddress.IPv4Network) or not network.is_private)
PY

container_for_image() {
    local prefix=$1 line id image name
    local -a matches=()
    while read -r id image name; do
        [[ $image == "$prefix":* ]] && matches+=("$id $name")
    done < <(docker ps --format '{{.ID}} {{.Image}} {{.Names}}')
    ((${#matches[@]} == 1)) || fail "esperado um container $prefix em execução; encontrados: ${#matches[@]}"
    printf '%s\n' "${matches[0]}"
}

read -r caddy_id caddy_name < <(container_for_image caddy)
read -r vault_id vault_name < <(container_for_image vaultwarden/server)

compose_path() {
    local path
    path=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.config_files" }}' "$1")
    [[ -n $path && $path != '<no value>' ]] || fail "container $2 não informa seu arquivo Compose"
    [[ $path != *,* ]] || fail "container $2 usa múltiplos Compose; configuração automática recusada: $path"
    path=$(realpath -e -- "$path") || fail "Compose de $2 não encontrado"
    [[ -f $path && ! -L $path ]] || fail "Compose de $2 deve ser arquivo regular: $path"
    printf '%s\n' "$path"
}

caddy_compose=$(compose_path "$caddy_id" "$caddy_name")
vault_compose=$(compose_path "$vault_id" "$vault_name")
[[ $caddy_compose != "$vault_compose" ]] || fail 'Caddy e Vaultwarden não podem compartilhar o mesmo Compose nesta ação'

mount_source() {
    local source
    source=$(docker inspect --format "{{range .Mounts}}{{if eq .Destination \"$2\"}}{{.Source}}{{end}}{{end}}" "$1")
    [[ -n $source ]] || fail "container $3 não possui volume persistente em $2"
    source=$(realpath -e -- "$source") || fail "volume de $3 não encontrado: $source"
    [[ -d $source && ! -L $source ]] || fail "volume de $3 deve ser diretório real: $source"
    printf '%s\n' "$source"
}

caddy_config=$(mount_source "$caddy_id" /config "$caddy_name")
caddy_config_dir=$(mount_source "$caddy_id" /etc/caddy/caddyfile "$caddy_name")
caddy_root=$(dirname -- "$caddy_config")
[[ $(dirname -- "$caddy_config_dir") == "$caddy_root" ]] || fail 'volumes do Caddy não pertencem ao mesmo diretório CasaOS'

server_ip=$(python3 - "$lan" $(hostname -I) <<'PY'
import ipaddress, sys
network = ipaddress.ip_network(sys.argv[1], strict=False)
found = []
for value in sys.argv[2:]:
    address = ipaddress.ip_address(value)
    if address in network and not address.is_loopback:
        found.append(str(address))
found = sorted(set(found))
if len(found) != 1:
    print("", end="")
    raise SystemExit(1)
print(found[0])
PY
) || fail "não foi possível detectar exatamente um IPv4 do servidor em $lan"

if ss -H -ltn 'sport = :443' | grep -q . && ! docker port "$caddy_id" 443/tcp 2>/dev/null | grep -Eq ':443$'; then
    fail 'porta 443/TCP já está ocupada no host'
fi

umask 077
work=$(mktemp -d /var/tmp/dotfiles-homelab-https.XXXXXX)
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT

normalize_compose() {
    local source=$1 destination=$2 directory
    directory=$(dirname -- "$source")
    (cd "$directory" && docker compose -f "$source" config --format json) > "$destination"
}
normalize_compose "$caddy_compose" "$work/caddy.original.json"
normalize_compose "$vault_compose" "$work/vaultwarden.original.json"
python3 - "$work/vaultwarden.original.json" <<'PY' || fail 'Vaultwarden já usa vault.reesearch64.com; não reaplique o fluxo HTTPS LAN antigo.'
import json, sys
c = json.load(open(sys.argv[1]))
for service in c.get('services', {}).values():
    env = service.get('environment') or {}
    if isinstance(env, list):
        env = dict(item.split('=', 1) for item in env)
    if env.get('DOMAIN') == 'https://vault.reesearch64.com':
        raise SystemExit(1)
PY
python3 "$transformer" caddy "$work/caddy.original.json" "$work/caddy.json" --domain "$domain" --app-data "$caddy_root" --server-ip "$server_ip"
python3 "$transformer" vaultwarden "$work/vaultwarden.original.json" "$work/vaultwarden.json" --domain "$domain" --app-data "$caddy_root"

token_file=$caddy_config/cloudflare.env
[[ -f $token_file && ! -L $token_file ]] || fail "crie previamente o arquivo de credencial: $token_file"
[[ $(stat -c %u "$token_file") == 0 && $(stat -c %a "$token_file") == 600 ]] || fail "$token_file deve pertencer ao root e ter permissão 0600"
IFS= read -r token_line < "$token_file"
[[ $token_line == CF_API_TOKEN=* ]] || fail "formato inválido em $token_file"
token=${token_line#CF_API_TOKEN=}
[[ $token =~ ^[A-Za-z0-9._-]+$ ]] || fail 'token Cloudflare vazio ou com caracteres inválidos'
printf 'Authorization: Bearer %s\nContent-Type: application/json\n' "$token" > "$work/cloudflare.headers"

curl -fsS -H "@$work/cloudflare.headers" https://api.cloudflare.com/client/v4/user/tokens/verify > "$work/token.json"
python3 - "$work/token.json" <<'PY' || fail 'token Cloudflare recusado'
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
raise SystemExit(not data.get("success") or data.get("result", {}).get("status") != "active")
PY
unset token token_line

python3 - "$domain" "$server_ip" <<'PY' || fail "DNS deve apontar somente $domain para $server_ip antes desta ação"
import socket, sys
try:
    addresses = {item[4][0] for item in socket.getaddrinfo(sys.argv[1], 443, socket.AF_INET)}
except socket.gaierror:
    raise SystemExit(1)
raise SystemExit(addresses != {sys.argv[2]})
PY

mkdir -p -- "$caddy_config/bin"
chmod 700 -- "$caddy_config/bin"
binary=$caddy_config/bin/caddy
binary_ok=false
if [[ -x $binary ]]; then
    if docker run --rm --entrypoint /config/bin/caddy -v "$caddy_config:/config:ro" "caddy:$caddy_version" version | grep -Eq '^v2\.11\.7([[:space:]]|$)' &&
       docker run --rm --entrypoint /config/bin/caddy -v "$caddy_config:/config:ro" "caddy:$caddy_version" list-modules | grep -qx 'dns.providers.cloudflare'; then
        binary_ok=true
    fi
fi
if ! $binary_ok; then
    rm -f -- "$caddy_config/bin/caddy.new"
    docker run --rm --entrypoint xcaddy -v "$caddy_config/bin:/output" "caddy:$caddy_version-builder" \
        build "v$caddy_version" --with "$cloudflare_module" --output /output/caddy.new
    chmod 755 -- "$caddy_config/bin/caddy.new"
    docker run --rm --entrypoint /output/caddy.new -v "$caddy_config/bin:/output:ro" "caddy:$caddy_version" version | grep -Eq '^v2\.11\.7([[:space:]]|$)'
    docker run --rm --entrypoint /output/caddy.new -v "$caddy_config/bin:/output:ro" "caddy:$caddy_version" list-modules | grep -qx 'dns.providers.cloudflare'
fi

for path in "$backup_root" "$backup_root/homelab-https"; do
    if [[ -e $path || -L $path ]]; then
        [[ -d $path && ! -L $path ]] || fail "backup requer diretório real: $path"
        [[ $(stat -c %a "$path") == 700 ]] || fail "backup requer permissão 0700: $path"
    else
        mkdir -- "$path"
        chmod 700 -- "$path"
    fi
done
backup=$(mktemp -d "$backup_root/homelab-https/$(date -u +%Y%m%dT%H%M%S.%NZ)-XXXXXX")
chmod 700 -- "$backup"

backup_item() {
    local path=$1 name=$2
    printf '%s\n' "$path" > "$backup/$name.path"
    if [[ -e $path || -L $path ]]; then
        cp -a --no-target-directory -- "$path" "$backup/$name"
    else
        : > "$backup/$name.absent"
    fi
}
backup_item "$caddy_compose" caddy-compose
backup_item "$vault_compose" vaultwarden-compose
backup_item "$caddy_config_dir/Caddyfile" Caddyfile
backup_item "$binary" caddy-binary

install_atomic() {
    local source=$1 destination=$2 mode=$3 temporary
    temporary=$(mktemp "$(dirname -- "$destination")/.https-XXXXXX")
    install -m "$mode" -- "$source" "$temporary"
    mv -fT -- "$temporary" "$destination"
}
restore_item() {
    local path=$1 name=$2 temporary
    if [[ -f $backup/$name.absent ]]; then
        rm -f -- "$path"
    else
        temporary=$(mktemp "$(dirname -- "$path")/.restore-XXXXXX")
        cp -a --no-target-directory -- "$backup/$name" "$temporary"
        mv -fT -- "$temporary" "$path"
    fi
}

changed=false
finish() {
    local status=$?
    trap - EXIT
    if ((status != 0)) && $changed; then
        set +e
        restore_item "$caddy_compose" caddy-compose
        restore_item "$vault_compose" vaultwarden-compose
        restore_item "$caddy_config_dir/Caddyfile" Caddyfile
        restore_item "$binary" caddy-binary
        docker compose -f "$vault_compose" up -d
        docker compose -f "$caddy_compose" up -d
        printf 'Configuração anterior restaurada. Backup preservado em %s\n' "$backup" >&2
    fi
    cleanup
    exit "$status"
}
trap finish EXIT

if ! $binary_ok; then
    mv -fT -- "$caddy_config/bin/caddy.new" "$binary"
fi
install_atomic "$caddyfile_template" "$caddy_config_dir/Caddyfile" 600
install_atomic "$work/caddy.json" "$caddy_compose" 600
install_atomic "$work/vaultwarden.json" "$vault_compose" 600
changed=true

docker network inspect "$network" >/dev/null 2>&1 || docker network create "$network" >/dev/null
docker compose -f "$vault_compose" config -q
docker compose -f "$caddy_compose" config -q
docker compose -f "$vault_compose" up -d
docker compose -f "$caddy_compose" up -d

for _ in {1..30}; do
    vault_status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$vault_name" 2>/dev/null || true)
    [[ $vault_status == healthy || $vault_status == running ]] && break
    sleep 2
done
[[ $vault_status == healthy || $vault_status == running ]] || fail "Vaultwarden não ficou saudável: $vault_status"

caddy_name=$(docker ps --filter "ancestor=caddy:$caddy_version" --format '{{.Names}}' | head -n1)
[[ -n $caddy_name ]] || fail 'container Caddy não reiniciou'
docker exec "$caddy_name" caddy list-modules | grep -qx 'dns.providers.cloudflare'

for _ in {1..45}; do
    if curl -fsS --resolve "$domain:443:$server_ip" "https://$domain/alive" >/dev/null 2>&1; then
        tls_ready=true
        break
    fi
    sleep 2
done
${tls_ready:-false} || { docker logs --tail 80 "$caddy_name" >&2; fail 'HTTPS não ficou disponível'; }

if ! ufw status | grep -Eq "443/tcp +ALLOW IN +$lan"; then
    ufw insert 1 allow from "$lan" to any port 443 proto tcp comment 'Vaultwarden HTTPS LAN'
fi

changed=false
printf 'Vaultwarden disponível em https://%s; backup em %s\n' "$domain" "$backup"
