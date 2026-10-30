#!/usr/bin/env bash
# Single menu entry; the underlying scripts retain their independent recovery paths.
set -Eeuo pipefail
[[ $(id -u) == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
for tool in systemctl curl; do command -v "$tool" >/dev/null; done
homeprod=$(dirname "$(dirname "$(readlink -f "$0")")")

# Never touch an installation that is still awaiting an external connectivity test.
if [[ -e /etc/homeprod-firewall/pending ]]; then
  cat >&2 <<'INFO'
Já existe um teste de firewall pendente. Nenhum serviço foi alterado.
Após testar NOVA sessão SSH, Access e bloqueio externo, confirme na nova sessão:
  sudo bash /etc/homeprod-firewall/manage.sh confirm
Ou reverta: sudo bash /etc/homeprod-firewall/manage.sh rollback
Diagnóstico: sudo bash /etc/homeprod-firewall/manage.sh status
INFO
  exit 1
fi

if systemctl is-active --quiet cloudflared-homeprod.service && \
   systemctl is-enabled --quiet cloudflared-homeprod.service && \
   curl --fail --silent --noproxy '*' --connect-timeout 3 --max-time 5 http://127.0.0.1:49312/ready >/dev/null && \
   curl --fail --silent --noproxy '*' --connect-timeout 3 --max-time 5 http://127.0.0.1:80/ >/dev/null; then
  echo 'Tunnel existente ativo, conectado e habilitado no boot; credencial preservada.'
else
  bash "$homeprod/tunnel/configure.sh"
fi

if [[ -f /etc/homeprod-firewall/rules.nft ]] && \
   systemctl is-active --quiet homeprod-firewall.service && \
   systemctl is-enabled --quiet homeprod-firewall.service && \
   command -v nft >/dev/null && nft list table inet homeprod_guard >/dev/null 2>&1; then
  echo 'Firewall existente ativo e habilitado no boot; regras preservadas, sem novo teste.'
  echo 'A verificação local não substitui testes externos. SSH não foi alterado.'
else
  # Keeps the timed trial and explicit confirmation after testing a NEW SSH session.
  bash "$homeprod/firewall/configure.sh" apply
fi
