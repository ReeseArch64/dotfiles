#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
action=${1:-apply}
case "$action" in apply|confirm|rollback|status) ;; *) echo 'Uso: configure.sh apply|confirm|rollback|status' >&2; exit 1 ;; esac
state=/etc/homeprod-firewall
pending="$state/pending"
unit=/etc/systemd/system/homeprod-firewall.service
service=homeprod-firewall.service
timer=homeprod-firewall-rollback
marker='# Managed by dotfiles: homeprod-firewall'
directory=$(dirname "$(readlink -f "$0")")
[[ ! -L "$state" && ! -L "$unit" ]] || { echo 'Caminho simbólico recusado.' >&2; exit 1; }
for tool in systemctl flock; do command -v "$tool" >/dev/null; done

# Never hold the lock while waiting for a human: the rollback must remain free to run.
if [[ "$action" == apply ]]; then
  cat <<'INFO'
ETAPA 2 — bloqueio de entradas públicas, inclusive portas Docker, sem alterar SSH.
Requer Tunnel + Access + MFA testados e console/rescue do provedor disponível.
A sessão atual continuará aberta, mas teste uma NOVA sessão SSH antes de confirmar.
O teste expira em 10 minutos e NÃO é persistente até a confirmação separada.
Não reinicie a VPS durante o teste. HTTP/Samba diretos serão bloqueados.
INFO
  read -r -p 'Validou o Tunnel e possui console de recuperação? Digite CONFIRMO: ' answer
  [[ "$answer" == CONFIRMO ]] || { echo 'Cancelado.'; exit 1; }
elif [[ "$action" == confirm ]]; then
  [[ -d "$pending" ]] || { echo 'Nenhum teste pendente.' >&2; exit 1; }
  cat <<'INFO'
Execute esta confirmação em uma NOVA sessão SSH, com a anterior ainda aberta.
Antes de confirmar: teste sudo, HomeProd via Access/MFA e bloqueio externo das
portas diretas do host e Docker (IPv4 e IPv6, quando disponível).
INFO
  read -r -p 'Todos os testes passaram? Digite CONFIRMO: ' answer
  [[ "$answer" == CONFIRMO ]] || { echo 'Não confirmado; rollback continua programado.'; exit 1; }
fi

exec 9>/run/dotfiles-homeprod-firewall.lock
flock 9
if [[ "$action" == status ]]; then
  systemctl --no-pager status "$service" "$timer.timer" || true
  if [[ -d "$pending" ]]; then echo 'Teste PENDENTE: ainda não confirmado.'; fi
  if command -v nft >/dev/null; then nft list table inet homeprod_guard || true; fi
  exit 0
fi
if [[ "$action" == rollback && ! -d "$pending" ]]; then
  echo 'Nenhum teste pendente; nada foi alterado.'
  exit 0
fi

stop_if_loaded() {
  local load
  if ! load=$(systemctl show --property=LoadState --value "$1"); then
    [[ "$load" == not-found ]] || return 1
  fi
  if [[ "$load" != not-found ]]; then systemctl stop "$1"; fi
}
stop_timer() {
  stop_if_loaded "$timer.timer" || return 1
  # The timer may already have fired; its process waits for this same lock.
  if [[ "$action" != rollback ]]; then stop_if_loaded "$timer.service" || return 1; fi
}
restore() {
  local failed=false
  [[ -f "$pending/ready" ]] || { echo 'Snapshot incompleto; recuperação manual necessária.' >&2; return 1; }
  echo 'Restaurando somente o firewall HomeProd ao estado anterior...' >&2
  # Keep going on unit errors: restoring runtime networking must still be attempted.
  if [[ -f "$pending/persisting" ]]; then
    if systemctl cat "$service" >/dev/null 2>&1; then
      systemctl stop "$service" || failed=true
      systemctl disable "$service" || failed=true
    fi
    if [[ -f "$pending/unit.original" ]]; then cp -a "$pending/unit.original" "$unit" || failed=true; else rm -f "$unit" || failed=true; fi
    if [[ -f "$pending/rules.original" ]]; then cp -a "$pending/rules.original" "$state/rules.nft" || failed=true; else rm -f "$state/rules.nft" || failed=true; fi
    systemctl daemon-reload || failed=true
    if [[ -f "$pending/enabled" ]]; then systemctl enable "$service" || failed=true; fi
    if [[ -f "$pending/active" ]]; then systemctl start "$service" || failed=true; fi
  fi
  : > "$pending/restore.nft" || return 1
  if nft list table inet homeprod_guard >/dev/null 2>&1; then
    printf 'delete table inet homeprod_guard\n' >> "$pending/restore.nft" || return 1
  fi
  if [[ -f "$pending/runtime.original" ]]; then
    cat "$pending/runtime.original" >> "$pending/restore.nft" || return 1
  fi
  nft -f "$pending/restore.nft" || return 1
  $failed && { echo 'Rede restaurada, mas persistência incompleta; snapshot preservado.' >&2; return 1; }
  stop_timer || return 1
  mv "$pending" "$(mktemp -d "$state/rolled-back.XXXXXXXX")/snapshot" || return 1
  echo 'Rollback concluído. Regras externas a homeprod_guard não foram alteradas.'
}
if [[ "$action" == rollback ]]; then
  restore
  exit 0
fi

rollback_needed=false
cleanup() {
  local code=$?
  trap - EXIT
  if $rollback_needed; then
    echo "Falha/cancelamento; iniciando rollback. Snapshot: $pending" >&2
    if restore; then :; else
      echo 'Rollback falhou. Use console/rescue e examine /etc/homeprod-firewall/pending.' >&2
    fi
  fi
  exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

check_tunnel() {
  systemctl is-active --quiet cloudflared-homeprod.service
  curl --fail --silent --show-error --noproxy '*' --connect-timeout 3 --max-time 5 http://127.0.0.1:49312/ready >/dev/null
  curl --fail --silent --show-error --noproxy '*' --connect-timeout 3 --max-time 5 http://127.0.0.1:80/ >/dev/null
}
check_conflicts() {
  local other output
  for other in firewalld.service nftables.service; do
    if systemctl is-active --quiet "$other" || systemctl is-enabled --quiet "$other"; then
      echo "Gerenciador de firewall ativo/habilitado: $other. Revise antes de prosseguir; não será desativado automaticamente." >&2
      return 1
    fi
  done
  if command -v ufw >/dev/null; then
    output=$(LC_ALL=C ufw status)
    if grep -q '^Status: active' <<< "$output"; then echo 'UFW ativo; configuração recusada.' >&2; return 1; fi
  fi
}

if [[ "$action" == apply ]]; then
  [[ ! -e "$pending" ]] || { echo 'Há um teste pendente. Confirme ou reverta antes de repetir.' >&2; exit 1; }
  for tool in ip ss python3 curl systemd-run; do command -v "$tool" >/dev/null; done
  check_tunnel
  check_conflicts
  if ! command -v nft >/dev/null; then
    apt-get update
    apt-get install --yes --no-install-recommends nftables
    check_conflicts
  fi
  [[ ! -e "$unit" ]] || grep -qx "$marker" "$unit"
  if nft list table inet homeprod_guard >/dev/null 2>&1; then
    [[ -f "$state/rules.nft" ]] && grep -qx "$marker" "$state/rules.nft" || {
      echo 'Tabela homeprod_guard existente não reconhecida; não será sobrescrita.' >&2; exit 1;
    }
  fi
  install -d -m 0700 -o root -g root "$state"
  # Stable root-owned recovery path, independent of checkout location/session.
  if [[ "$(readlink -f "$0")" != "$state/manage.sh" ]]; then
    install -m 0700 -o root -g root "$0" "$state/manage.sh"
  fi
  work=$(mktemp -d "$state/preparing.XXXXXXXX")
  python3 "$directory/rules.py" "$work/inventory.json" "$work/candidate.nft"
  nft --check -f "$work/candidate.nft"
  install -m 0600 "$directory/homeprod-firewall.service" "$work/unit.candidate"
  [[ ! -e "$unit" ]] || cp -a "$unit" "$work/unit.original"
  [[ ! -e "$state/rules.nft" ]] || cp -a "$state/rules.nft" "$work/rules.original"
  if nft list table inet homeprod_guard >/dev/null 2>&1; then
    nft -s list table inet homeprod_guard > "$work/runtime.original"
  fi
  if systemctl is-active --quiet "$service"; then touch "$work/active"; fi
  if systemctl is-enabled --quiet "$service"; then touch "$work/enabled"; fi
  touch "$work/ready"
  mv "$work" "$pending"
  rollback_needed=true
  # Do not re-use an earlier failed transient unit or an expired timer.
  stop_if_loaded "$timer.timer"
  stop_if_loaded "$timer.service"
  systemctl reset-failed "$timer.service" 2>/dev/null || true
  systemd-run --unit="$timer" --on-active=10m --timer-property=AccuracySec=1s \
    /bin/bash "$state/manage.sh" rollback
  systemctl is-active --quiet "$timer.timer"
  nft -f "$pending/candidate.nft"
  check_tunnel
  rollback_needed=false
  cat <<'INFO'
Teste aplicado. Rollback automático em 10 minutos; ainda NÃO persistente.
1. Mantenha esta sessão e abra OUTRA conexão SSH. Teste sudo -v.
2. Teste https://homeprod.reesearch64.com (Access/MFA, CasaOS e terminal).
3. De outra rede, teste IP/hostname antigo: HTTP, Samba e uma porta Docker
   publicada devem falhar, inclusive por IPv6 quando houver endereço público.
4. Na NOVA sessão SSH, antes de expirar:
   sudo bash /etc/homeprod-firewall/manage.sh confirm
Para reverter agora: sudo bash /etc/homeprod-firewall/manage.sh rollback
Para inspecionar: sudo bash /etc/homeprod-firewall/manage.sh status
Não reinicie antes de confirmar. Reboot durante teste descarta a proteção nova.
INFO
elif [[ "$action" == confirm ]]; then
  [[ -f "$pending/ready" ]] || { echo 'Snapshot de teste ausente/incompleto.' >&2; exit 1; }
  systemctl is-active --quiet "$timer.timer" || { echo 'Prazo expirou; execute rollback antes de repetir.' >&2; exit 1; }
  check_tunnel
  nft --check -f "$pending/candidate.nft"
  rollback_needed=true
  touch "$pending/persisting"
  install -m 0600 -o root -g root "$pending/candidate.nft" "$state/rules.nft"
  install -m 0644 -o root -g root "$pending/unit.candidate" "$unit"
  systemctl daemon-reload
  systemctl enable "$service"
  systemctl start "$service"
  # Reload applies the table atomically, without stopping the guard service.
  systemctl reload "$service"
  check_tunnel
  stop_timer
  mv "$pending" "$(mktemp -d "$state/confirmed.XXXXXXXX")/snapshot"
  rollback_needed=false
  echo 'Firewall confirmado e habilitado no boot. Teste novamente após reboot com console disponível.'
fi
