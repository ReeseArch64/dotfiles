#!/usr/bin/env bash
# Setup do servidor SSH (OpenSSH + UFW) para máquinas Arch-based (CachyOS / Omarchy).
# Fonte: MegaBrain/Workflows/Rede/Tutorial de Setup SSH no Linux.md
#
# Uso: setup-ssh.sh [opções]
#   (sem opções)   Setup padrão permanente: sshd habilitado no boot + UFW liberando 22/tcp
#   --socket       Usa sshd.socket (socket activation) em vez do daemon sempre rodando
#   --temp         SSH só para a sessão atual (start sem enable)
#   --stop         Encerra o SSH temporário e fecha a porta 22 no firewall
#   --lan-only     Libera a porta 22 só para a sub-rede local (default 192.168.1.0/24)
#   --subnet CIDR  Sub-rede usada por --lan-only
#   --harden       Desativa login por senha e login de root (exige chave em authorized_keys)
#   -h, --help     Mostra esta ajuda

set -euo pipefail

MODE="permanent"
LAN_ONLY=0
SUBNET="192.168.1.0/24"
HARDEN=0
HARDEN_FILE="/etc/ssh/sshd_config.d/10-hardening.conf"

info() { printf '\e[1;34m==>\e[0m %s\n' "$*"; }
warn() { printf '\e[1;33m==> AVISO:\e[0m %s\n' "$*" >&2; }
die()  { printf '\e[1;31m==> ERRO:\e[0m %s\n' "$*" >&2; exit 1; }

usage() { sed -n '2,/^$/{s/^# \{0,1\}//;p}' "$0"; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --socket)   MODE="socket" ;;
        --temp)     MODE="temp" ;;
        --stop)     MODE="stop" ;;
        --lan-only) LAN_ONLY=1 ;;
        --subnet)   SUBNET="${2:?--subnet requer um CIDR}"; LAN_ONLY=1; shift ;;
        --harden)   HARDEN=1 ;;
        -h|--help)  usage; exit 0 ;;
        *)          usage; die "opção desconhecida: $1" ;;
    esac
    shift
done

[[ $EUID -eq 0 ]] && die "rode como usuário normal; o script usa sudo quando necessário."
command -v pacman >/dev/null || die "pacman não encontrado (script feito para Arch-based)."

allow_ssh() {
    if [[ $LAN_ONLY -eq 1 ]]; then
        sudo ufw allow from "$SUBNET" to any port 22 proto tcp comment 'SSH LAN Only'
    else
        sudo ufw allow 22/tcp comment 'SSH'
    fi
}

deny_ssh() {
    sudo ufw delete allow 22/tcp 2>/dev/null || true
    sudo ufw delete allow from "$SUBNET" to any port 22 proto tcp 2>/dev/null || true
}

# --- Encerrar sessão temporária (Opção B, passos 2 e 3) ---
if [[ $MODE == "stop" ]]; then
    info "Parando sshd e fechando a porta 22"
    sudo systemctl stop sshd.service sshd.socket 2>/dev/null || true
    deny_ssh
    sudo ufw reload
    sudo ufw status verbose
    exit 0
fi

# --- 1 e 3. Instalar OpenSSH, UFW e backend nftables ---
missing=$(pacman -T openssh ufw iptables-nft || true)
if [[ -n $missing ]]; then
    info "Instalando: $missing"
    # shellcheck disable=SC2086
    sudo pacman -S --needed $missing
else
    info "openssh, ufw e iptables-nft já instalados"
fi

# --- 2. Serviço SSH ---
case "$MODE" in
    permanent)
        info "Habilitando sshd no boot"
        sudo systemctl disable --now sshd.socket 2>/dev/null || true
        sudo systemctl enable --now sshd.service
        ;;
    socket)
        info "Usando socket activation (sshd.socket)"
        sudo systemctl disable --now sshd.service 2>/dev/null || true
        sudo systemctl enable --now sshd.socket
        ;;
    temp)
        info "Iniciando sshd só para esta sessão (não persiste após reboot)"
        sudo systemctl start sshd.service
        ;;
esac

# --- Opção C.2: autenticação só por chave ---
if [[ $HARDEN -eq 1 ]]; then
    if [[ ! -s "$HOME/.ssh/authorized_keys" ]]; then
        warn "~/.ssh/authorized_keys vazio ou inexistente; pulando --harden para não perder o acesso."
    else
        info "Aplicando hardening em $HARDEN_FILE"
        printf 'PasswordAuthentication no\nPermitRootLogin no\n' | sudo tee "$HARDEN_FILE" >/dev/null
        sudo sshd -t || die "configuração do sshd inválida; revise $HARDEN_FILE"
        if [[ $MODE != "socket" ]]; then
            sudo systemctl restart sshd.service
        fi
    fi
fi

# --- 4. Firewall ---
# A regra é criada antes do `ufw enable` para não derrubar uma sessão SSH ativa.
info "Configurando UFW"
sudo systemctl enable --now ufw
allow_ssh
sudo ufw --force enable
sudo ufw reload

# --- Verificação ---
echo
sudo systemctl --no-pager status "$([[ $MODE == socket ]] && echo sshd.socket || echo sshd.service)" | head -n 5
echo
sudo ufw status verbose
echo
info "IPs desta máquina (conecte com: ssh $USER@<IP>):"
ip -br a | grep -v '^lo'
[[ $MODE == "temp" ]] && info "Ao terminar, rode: $0 --stop"
exit 0
