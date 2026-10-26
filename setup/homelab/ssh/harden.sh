#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
admin=${SUDO_USER:-}
[[ -n "$admin" && "$admin" != root && "$admin" =~ ^[a-z_][a-z0-9_-]*$ ]] || {
  echo 'Execute o menu como o usuário administrador (alsgy), não como root.' >&2; exit 1;
}
account=$(getent passwd "$admin")
IFS=: read -r _ _ uid _ _ user_home _ <<< "$account"
[[ "$uid" != 0 ]]
ssh-keygen -lf "$user_home/.ssh/authorized_keys"
printf '\nMantenha esta sessão aberta. Em outro terminal, teste SSH como %s\n' "$admin"
printf 'com id_pessoal_desktop, PasswordAuthentication=no e IdentityAgent=none, e teste sudo -v.\n'
read -r -p 'Após o teste, digite CONFIRMO para exigir chave pública: ' answer
[[ "$answer" == CONFIRMO ]] || { echo 'Política SSH não alterada.'; exit 1; }
backup=$(mktemp -d /etc/ssh/homelab-backup.XXXXXXXX)
cp -a /etc/ssh/sshd_config "$backup/sshd_config"
policy="$backup/policy"
printf '%s\n' "AllowUsers $admin" 'PermitRootLogin no' 'PubkeyAuthentication yes' \
  'AuthenticationMethods publickey' 'PasswordAuthentication no' \
  'KbdInteractiveAuthentication no' "AuthorizedKeysFile $user_home/.ssh/authorized_keys" \
  'AuthorizedKeysCommand none' 'TrustedUserCAKeys none' > "$policy"
cat "$policy" /etc/ssh/sshd_config > "$backup/candidate"
sshd -t -f "$backup/candidate"
connection=${SSH_CONNECTION:-127.0.0.1}
effective=$(sshd -T -f "$backup/candidate" -C "user=$admin,host=homelab,addr=${connection%% *}")
while IFS= read -r expected; do
  grep -Fxiq "$expected" <<< "$effective" || { echo "Conflito SSH: $expected" >&2; exit 1; }
done < "$policy"
rollback() {
  cp -a "$backup/sshd_config" /etc/ssh/sshd_config
  systemctl reload ssh.service || true
  echo "Política anterior restaurada. Backup: $backup" >&2
}
trap rollback ERR
install -m 0600 "$backup/candidate" /etc/ssh/sshd_config
# With socket activation, an active connection already starts ssh.service.
systemctl start ssh.service
systemctl reload ssh.service
trap - ERR
echo "SSH restrito a $admin por chave. Teste nova conexão antes de sair. Backup: $backup"
