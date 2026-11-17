# Laptop — Omarchy Linux

Perfil reconhecido pelo identificador de hostname `laptop-alan`, com Omarchy Linux (`ID=omarchy` em `/etc/os-release`). Não exige Niri nem Noctalia. O fluxo SSH roda diretamente no código Go da CLI (`cmd/dotfiles/laptop_ssh*.go`), sem scripts `.sh` ou `.py`. Somente este perfil recebe o novo fluxo; o notebook CachyOS fica em `setup/notebook/`, enquanto desktop e servidores permanecem inalterados.

## SSH por dispositivo

Cada dispositivo possui sua própria chave privada: `id_laptop`, `id_desktop`, `id_macbook`, `id_iphone` ou `id_android`. No laptop, **SSH > Configurar SSH** gera somente `~/.ssh/id_laptop` Ed25519, pedindo e-mail e passphrase diretamente no terminal do `ssh-keygen`. O e-mail é o comentário da chave, não uma conta de login. A passphrase protege a chave privada; não é a senha de login do servidor nem de sudo. Não é armazenada pela CLI nem passada em argumentos de processo. Uma chave existente é preservada; se faltar a pública correspondente, a ação recusa sobrescrevê-la.

O cliente recebe um bloco em `~/.ssh/config` para usar `id_laptop`, com `IdentitiesOnly yes` e `AddKeysToAgent yes`. Configurações anteriores são preservadas e copiadas para um backup local `~/.ssh/config.backup-*`. Outros `IdentityFile` explicitamente existentes não são apagados. Este fluxo não instala nem habilita um agente SSH.

### Chaves autorizadas

Em **SSH > Chaves autorizadas**:

- **Adicionar chave pública**: escolha um dos cinco nomes de dispositivo e cole a pública em uma linha. A CLI valida com `ssh-keygen`, mostra o fingerprint e pede `ADICIONAR`. A mesma pública não é adicionada duas vezes. Pode haver mais de uma chave de um dispositivo durante uma rotação.
- **Visualizar chaves atuais**: mostra as públicas autorizadas e seus fingerprints.
- **Remover chave pública**: escolha pelo número, confira o fingerprint e confirme `REMOVER`. A última chave não pode ser removida: adicione e teste outra primeiro.

As chaves são gravadas diretamente em `~/.ssh/authorized_keys`, atomicamente, com permissão `0600`, sob lock entre execuções da CLI. Não é necessário criar arquivos `id_desktop.pub`, `id_iphone.pub` etc. no laptop. Não cole chaves privadas. Arquivos com chaves inválidas ou opções de `authorized_keys` preexistentes são recusados sem sobrescrever: revise manualmente antes de usar o gerenciador. Os nomes de dispositivo são rótulos, não uma comprovação de propriedade; confira o fingerprint com o dono do dispositivo.

### Configuração inicial recomendada

1. Adicione a chave pública de outro dispositivo em **Chaves autorizadas > Adicionar chave pública**. As ações instalam somente `openssh` via Pacman se faltar. Não dependem de Python.
2. Execute **Configurar SSH**. A ação gera a identidade do laptop, se ausente, configura o cliente e mostra sua pública para cadastrar nos outros dispositivos.
3. Havendo ao menos uma chave autorizada, confirme `INICIAR`: gera host keys ausentes, valida a configuração atual e inicia `sshd.service` com a política que já existe. Essa etapa não altera o firewall nem endurece a política anterior. Se cancelar depois, o serviço pode permanecer ativo com essa política.
4. Mantenha a sessão atual aberta. De outro dispositivo, teste o acesso exclusivamente por chave e `sudo -v`. Tenha console local ou recuperação disponível. Se o firewall atual bloquear o teste, revise o acesso localmente; a CLI não abre portas nesta etapa.
5. Somente após `CONFIRMO`, o fluxo substitui `/etc/ssh/sshd_config` por uma configuração dedicada ao usuário atual, preservando a porta existente e mantendo o original em `/etc/ssh/laptop-backup.*` (root, `0700`). A nova política permite **somente publickey**, sem senha, autenticação interativa ou root, usa somente `.ssh/authorized_keys`, restringe `AllowUsers` ao usuário atual e oferece SFTP interno. Configurações antigas, Includes e Match deixam de ser usados; revise antes de confirmar se havia customizações.
6. Valida com `sshd -t` e `sshd -T`, habilita o serviço no boot e reinicia o servidor. Em erro na aplicação, restaura a configuração anterior e tenta reiniciar o serviço. `sshd.socket` ativo/habilitado é recusado, não alterado silenciosamente. Teste uma **nova** conexão antes de fechar a atual.

A confirmação é manual: a CLI não comprova o teste remoto. A geração local de `id_laptop` não autoriza outros dispositivos a entrar; para isso é necessário adicionar as públicas deles.

## Firewall

A opção permanece escurecida e sem execução, com **Requer SSH configurado**, até detectar `id_laptop`, `id_laptop.pub`, chaves autorizadas, serviço SSH ativo/habilitado e política restrita a chaves. Não exige as identidades antigas do notebook, chaves de plataformas Git ou flags em `config.txt`.

Antes de alterar o firewall, solicita sudo se necessário e revalida as públicas com `ssh-keygen`, permissões e proprietário dos arquivos, política efetiva com `sshd -T`, serviço, porta e LAN privada detectada. Uma conexão administrativa fora dessa LAN é recusada. A validação não substitui um teste remoto nem uma auditoria completa do sistema.

Após isso, instala UFW e iptables-nft se ausentes, bloqueia entrada por padrão, permite saída, substitui a liberação SSH global pela regra SSH somente na LAN e habilita o serviço no boot. Outras regras UFW existentes são preservadas; revise-as antes de ativar. Execute preferencialmente localmente.

Nada é aplicado automaticamente ao abrir a CLI. Não prepara ou move pastas, não cria links simbólicos e não herda `setup/notebook/` nem `setup/config/`. Apenas cria/atualiza os arquivos SSH descritos acima e seus backups, quando uma ação é solicitada.

## Testes

```bash
go test ./...
go vet ./...
```
