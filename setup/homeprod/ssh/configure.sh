#!/usr/bin/env bash
set -Eeuo pipefail

[[ $EUID == 0 ]] || { echo 'Execute como root.' >&2; exit 1; }
printf '\nCole o conteúdo de id_pessoal_desktop.pub em uma única linha.\n'
read -r -p 'Chave pública (ssh-ed25519 ...): ' public_key
[[ "$public_key" == ssh-ed25519\ * ]] || {
  echo 'Forneça uma chave pública Ed25519, sem opções; nunca a chave privada.' >&2; exit 1;
}
account=$(getent passwd alsgy)
IFS=: read -r _ _ admin_uid admin_gid _ admin_home _ <<< "$account"
[[ "$admin_uid" != 0 && "$admin_home" == /home/alsgy ]] || exit 1
id -nG alsgy | tr ' ' '\n' | grep -Fxq sudo

apt-get update
apt-get install --yes openssh-server openssh-client
install -d -m 0755 /run/sshd
sshd -t

# Validate from stdin; no key file is needed in the repository.
printf '%s\n' "$public_key" | ssh-keygen -lf /dev/stdin

[[ ! -L "$admin_home/.ssh" && ! -L "$admin_home/.ssh/authorized_keys" ]] || exit 1
backup=$(mktemp -d /etc/ssh/homeprod-backup.XXXXXXXX)
chmod 0700 "$backup"
cp -a /etc/ssh/sshd_config "$backup/sshd_config"
if [[ -f "$admin_home/.ssh/authorized_keys" ]]; then
  cp -a "$admin_home/.ssh/authorized_keys" "$backup/authorized_keys"
fi
install -d -m 0700 -o "$admin_uid" -g "$admin_gid" "$admin_home/.ssh"
touch "$admin_home/.ssh/authorized_keys"
chown "$admin_uid:$admin_gid" "$admin_home/.ssh/authorized_keys"
chmod 0600 "$admin_home/.ssh/authorized_keys"
if ! grep -Fxq "$public_key" "$admin_home/.ssh/authorized_keys"; then
  printf '\n%s\n' "$public_key" >> "$admin_home/.ssh/authorized_keys"
fi
systemctl enable --now ssh.service

printf '\nMantenha esta sessão aberta. Em outro terminal, conecte como alsgy usando\n'
printf 'id_pessoal_desktop com PasswordAuthentication=no e IdentityAgent=none; teste sudo -v.\n'
printf 'Confirme também que possui console/rescue do provedor ou backup recuperável da chave.\n'
read -r -p 'Após testar acesso, sudo e recuperação, digite CONFIRMO: ' answer
[[ "$answer" == CONFIRMO ]] || { echo 'Restrições não aplicadas; chave adicionada para teste.'; exit 1; }

# A separate root-owned file makes this the only authorized key source.
[[ ! -L /etc/ssh/homeprod_authorized_keys ]] || exit 1
install -d -m 0755 /etc/ssh/homeprod_authorized_keys
[[ ! -L /etc/ssh/homeprod_authorized_keys/alsgy ]] || exit 1
if [[ -f /etc/ssh/homeprod_authorized_keys/alsgy ]]; then
  cp -a /etc/ssh/homeprod_authorized_keys/alsgy "$backup/managed_key"
fi
printf '%s\n' "$public_key" > /etc/ssh/homeprod_authorized_keys/alsgy
chmod 0644 /etc/ssh/homeprod_authorized_keys/alsgy
chown root:root /etc/ssh/homeprod_authorized_keys/alsgy

rollback() {
  cp -a "$backup/sshd_config" /etc/ssh/sshd_config
  if [[ -f "$backup/managed_key" ]]; then
    cp -a "$backup/managed_key" /etc/ssh/homeprod_authorized_keys/alsgy
  fi
  systemctl reload ssh.service || true
  echo "Falha: configuração SSH restaurada. Backup: $backup" >&2
}
trap rollback ERR
policy=$(dirname "$(readlink -f "$0")")/sshd.conf
candidate=$(mktemp "$backup/candidate.XXXXXXXX")
# Prepend global policy so distribution/cloud-init defaults cannot override it.
cat "$policy" /etc/ssh/sshd_config > "$candidate"
sshd -t -f "$candidate"
for user in alsgy root; do
  client_address=${SSH_CONNECTION:-}
  client_address=${client_address%% *}
  effective=$(sshd -T -f "$candidate" -C "user=$user,host=homeprod,addr=${client_address:-127.0.0.1}")
  while IFS= read -r expected; do
    [[ -z "$expected" || "$expected" == '#'* ]] && continue
    grep -Fxiq "$expected" <<< "$effective" || {
      echo "Política conflitante para $user: $expected" >&2; false;
    }
  done < "$policy"
done
install -m 0600 "$candidate" /etc/ssh/sshd_config
sshd -t
systemctl reload ssh.service
trap - ERR
printf '\nSSH restrito a alsgy e à chave indicada. Backup: %s\n' "$backup"
printf 'Teste uma NOVA conexão antes de fechar esta sessão.\n'
