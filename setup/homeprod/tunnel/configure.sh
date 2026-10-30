#!/usr/bin/env bash
# Stage 1 only: no firewall, SSH, Docker or Vaultwarden changes.
set +x
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
for tool in systemctl curl flock; do command -v "$tool" >/dev/null; done
exec 9>/run/lock/dotfiles-homeprod-tunnel.lock
flock -n 9 || { echo 'Outra configuração do Tunnel está em execução.' >&2; exit 1; }

directory=$(dirname "$(readlink -f "$0")")
state=/etc/homeprod-cloudflared
unit=/etc/systemd/system/cloudflared-homeprod.service
service=cloudflared-homeprod.service
[[ ! -L "$state" && ! -L "$unit" ]] || { echo 'Caminho de configuração simbólico recusado.' >&2; exit 1; }
if [[ -e "$unit" ]] && ! grep -qx '# Managed by dotfiles: homeprod-tunnel' "$unit"; then
  echo 'Já existe uma unidade HomeProd não gerenciada por este script.' >&2; exit 1
fi
if systemctl cat cloudflared.service >/dev/null 2>&1; then
  echo 'Já existe cloudflared.service. Revise o conector existente antes de instalar outro.' >&2; exit 1
fi
if command -v docker >/dev/null; then
  images=$(docker ps --format '{{.Image}}')
  if grep -Eq '(^|/)(cloudflare/cloudflared|wisdomsky/cloudflared-web)(:|@|$)' <<< "$images"; then
    echo 'Conector Cloudflared em execução no Docker; não será criada uma segunda instância.' >&2; exit 1
  fi
fi
curl --fail --silent --show-error --noproxy '*' --connect-timeout 5 --max-time 10 http://127.0.0.1:80/ >/dev/null
cat <<'INFO'
ETAPA 1 — HomeProd Tunnel. O IP público CONTINUARÁ acessível nesta etapa.
Antes de continuar, prepare no painel Cloudflare Zero Trust:
1. Aplicação Access para homeprod.reesearch64.com, permitindo somente sua
   identidade com autenticação forte; sem Bypass nem Allow Everyone.
2. Tunnel gerenciado remotamente, dedicado ao HomeProd.
3. Rota homeprod.reesearch64.com -> http://127.0.0.1:80 (CasaOS).
   Regra final: http_status:404. DNS deste hostname apontando para o Tunnel.
Não use vault.reesearch64.com nem publique SSH ou Samba neste Tunnel.
Este script não cria nem audita as políticas Access/DNS na sua conta.
INFO
read -r -p 'Preparou Access, Tunnel e DNS acima? Digite CONFIRMO: ' answer
[[ "$answer" == CONFIRMO ]] || { echo 'Cancelado sem alterações.'; exit 1; }
read -rs -p 'Cole somente o token do Tunnel (entrada oculta): ' token
printf '\n'
[[ "$token" =~ ^[A-Za-z0-9_=+/.-]+$ ]] || { unset token; echo 'Token vazio ou inválido.' >&2; exit 1; }

install -d -m 0700 -o root -g root "$state"
work=$(mktemp -d "$state/backup.XXXXXXXX")
phase='preparar instalação'
changed=false
committed=false
was_active=false
was_enabled=false
systemctl is-active --quiet "$service" && was_active=true
systemctl is-enabled --quiet "$service" && was_enabled=true
for file in "$unit" "$state/tunnel-token"; do
  [[ ! -L "$file" ]] || { echo 'Arquivo simbólico recusado.' >&2; exit 1; }
done
[[ ! -e "$unit" ]] || cp -a "$unit" "$work/unit"
[[ ! -e "$state/tunnel-token" ]] || cp -a "$state/tunnel-token" "$work/token"
rollback() {
  echo 'Restaurando unidade e credencial anteriores...' >&2
  systemctl stop "$service"
  if [[ -f "$work/unit" ]]; then cp -a "$work/unit" "$unit"; else rm -f "$unit"; fi
  if [[ -f "$work/token" ]]; then cp -a "$work/token" "$state/tunnel-token"; else rm -f "$state/tunnel-token"; fi
  if ! $was_enabled; then systemctl disable "$service"; fi
  systemctl daemon-reload
  if $was_enabled; then systemctl enable "$service"; fi
  if $was_active; then systemctl start "$service"; fi
}
cleanup() {
  local code=$?
  trap - EXIT
  unset token
  rm -f "$work/new-token"
  if ! $committed; then
    echo "Falha/cancelamento na etapa: $phase. Backup privado: $work" >&2
    if $changed; then
      set +e
      rollback
    fi
  fi
  exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

phase='instalar pacote oficial Cloudflared'
if [[ ! -x /usr/bin/cloudflared ]]; then
  apt-get update
  apt-get install --yes ca-certificates curl
  curl --fail --silent --show-error --location --retry 3 --connect-timeout 15 --max-time 90 \
    https://pkg.cloudflare.com/cloudflare-main.gpg -o "$work/cloudflare-main.gpg"
  install -m 0644 "$work/cloudflare-main.gpg" /usr/share/keyrings/cloudflare-main.gpg
  printf '%s\n' 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main' \
    > /etc/apt/sources.list.d/cloudflared.list
  chmod 0644 /etc/apt/sources.list.d/cloudflared.list
  apt-get update
  apt-get install --yes cloudflared
fi
help=$(/usr/bin/cloudflared tunnel run --help)
grep -q -- '--token-file' <<< "$help" || { echo 'Atualize cloudflared: requer --token-file (2025.4.0+).' >&2; exit 1; }
phase='instalar credencial e serviço HomeProd'
printf '%s' "$token" > "$work/new-token"
unset token
changed=true
install -m 0600 -o root -g root "$work/new-token" "$state/tunnel-token"
rm -f "$work/new-token"
install -m 0644 -o root -g root "$directory/cloudflared-homeprod.service" "$unit"
systemctl daemon-reload
systemctl enable "$service"
systemctl restart "$service"

phase='aguardar conexão do Tunnel com a Cloudflare'
connected=false
for ((attempt=0; attempt<30; attempt++)); do
  if systemctl is-active --quiet "$service" && \
     curl --fail --silent --noproxy '*' --max-time 2 http://127.0.0.1:49312/ready >/dev/null; then
    connected=true
    break
  fi
  sleep 2
done
$connected || { echo 'Conector sem conexão. Consulte journalctl -u cloudflared-homeprod.service (não compartilhe credenciais).' >&2; exit 1; }
committed=true
cat <<INFO
Conector conectado à Cloudflare. Backup privado: $work
Isso NÃO comprova a proteção do Access nem fecha o acesso direto pelo IP.
Agora teste em outra rede/navegador privado:
- https://homeprod.reesearch64.com deve exigir Access antes de mostrar o CasaOS.
- Após autenticar, teste login, navegação e terminal/WebSocket.
- Teste novamente após: sudo systemctl restart cloudflared-homeprod.service
SSH, firewall, Docker e Vaultwarden não foram alterados.
NÃO considere a VPS protegida ainda: a etapa 2 deverá fechar as entradas
HTTP/Samba em IPv4/IPv6 e impedir bypass por portas Docker, preservando SSH.
INFO
