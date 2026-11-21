#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Execute via sudo.' >&2; exit 1; }
admin=${SUDO_USER:-}
[[ -n "$admin" && "$admin" != root && "$admin" =~ ^[a-z_][a-z0-9_-]*$ ]] || {
  echo 'Execute o menu como o usuário administrador (alsgy), não como root.' >&2; exit 1;
}
account=$(getent passwd "$admin")
IFS=: read -r _ _ uid gid _ user_home _ <<< "$account"
[[ "$uid" != 0 ]]
[[ "$user_home" == /* && "$user_home" != / && -d "$user_home" ]]
printf '\nCole o conteúdo de id_pessoal_desktop.pub em uma única linha. Nunca cole a chave privada.\n'
read -r -p 'Chave pública (ssh-ed25519 ...): ' public_key
[[ "$public_key" == ssh-ed25519\ * ]] || { echo 'Chave pública Ed25519 esperada.' >&2; exit 1; }
printf '%s\n' "$public_key" | ssh-keygen -lf /dev/stdin
[[ ! -L "$user_home/.ssh" && ! -L "$user_home/.ssh/authorized_keys" ]]
backup=$(mktemp -d /etc/ssh/homelab-backup.XXXXXXXX)
cp -a /etc/ssh/sshd_config "$backup/sshd_config"
if [[ -e "$user_home/.ssh/authorized_keys" ]]; then
  [[ -f "$user_home/.ssh/authorized_keys" ]]
  cp -a "$user_home/.ssh/authorized_keys" "$backup/authorized_keys.original"
fi
install -d -m 0700 -o "$uid" -g "$gid" "$user_home/.ssh"
touch "$user_home/.ssh/authorized_keys"
chmod 0600 "$user_home/.ssh/authorized_keys"
chown "$uid:$gid" "$user_home/.ssh/authorized_keys"
if ! grep -Fxq "$public_key" "$user_home/.ssh/authorized_keys"; then
  printf '\n%s\n' "$public_key" >> "$user_home/.ssh/authorized_keys"
fi
cp -a "$user_home/.ssh/authorized_keys" "$backup/authorized_keys.tested"
printf '\nMantenha esta sessão aberta. Em outro terminal, teste SSH como %s\n' "$admin"
printf 'com id_pessoal_desktop, PasswordAuthentication=no e IdentityAgent=none, e teste sudo -v.\n'
read -r -p 'Após o teste, digite CONFIRMO para exigir chave pública: ' answer
[[ "$answer" == CONFIRMO ]] || { echo "Restrições não aplicadas; chave adicionada para teste. Backup: $backup"; exit 1; }
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
  cp -a "$backup/authorized_keys.tested" "$user_home/.ssh/authorized_keys"
  systemctl reload ssh.service || true
  echo "Política anterior restaurada. Backup: $backup" >&2
}
trap rollback ERR
printf '%s\n' "$public_key" > "$user_home/.ssh/authorized_keys"
install -m 0600 "$backup/candidate" /etc/ssh/sshd_config
# With socket activation, an active connection already starts ssh.service.
systemctl start ssh.service
systemctl reload ssh.service
trap - ERR
echo "SSH restrito a $admin por chave. Teste nova conexão antes de sair. Backup: $backup"
