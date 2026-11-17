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

1. Execute **Configurar SSH**, preferencialmente pelo console local. A ação instala somente `openssh` se faltar, gera a identidade do laptop se ausente e configura o cliente. Não depende de Python.
2. `authorized_keys` é criado vazio se ainda não existir; chaves existentes são preservadas. Não é obrigatório cadastrar públicas nem testar login remoto. Sem públicas autorizadas, o servidor escuta, mas ninguém pode autenticar.
3. Confirme `CONFIRMO` após revisar o aviso e garantir console/recuperação disponível. Sem chaves autorizadas, executar de uma sessão SSH é recusado para evitar perda de acesso. O firewall não é alterado nesta etapa.
4. Gera host keys ausentes e substitui `/etc/ssh/sshd_config` por uma configuração dedicada ao usuário atual, preservando a porta existente e mantendo o original em `/etc/ssh/laptop-backup.*` (root, `0700`). A política permite **somente publickey**, sem senha, autenticação interativa ou root, usa somente `.ssh/authorized_keys`, restringe `AllowUsers` ao usuário atual e oferece SFTP interno. Configurações antigas, Includes e Match deixam de ser usados; revise antes de confirmar se havia customizações.
5. Valida com `sshd -t` e `sshd -T`, habilita o serviço no boot, inicia/reinicia o servidor e verifica a resposta SSH na porta local, sem autenticação. Não exige que o serviço já estivesse ativo. Em erro na aplicação, restaura a configuração e tenta restaurar os estados anteriores de atividade/habilitação do serviço. `sshd.socket` ativo/habilitado é recusado, não alterado silenciosamente.
6. Quando quiser permitir login de outro dispositivo, adicione a pública dele em **Chaves autorizadas > Adicionar chave pública**. Isso é independente da configuração inicial. Se estiver conectado remotamente, mantenha a sessão atual aberta e teste uma nova conexão antes de encerrá-la.

A geração local de `id_laptop` não autoriza outros dispositivos a entrar. A verificação da porta é local, não comprova acesso de outra rede.

A leitura aceita `Port 22`, `port=22`, aspas/comentários e portas repetidas. Se `sshd -T` não imprimir configuração reconhecível, tenta `sshd -G`. Na descoberta inicial, se ainda faltar a porta, lê `sshd_config` e seus Includes; usa o padrão 22 somente quando não existe `Port` explícito e todos os arquivos necessários foram lidos. Portas distintas, Includes cíclicos e arquivos ilegíveis são recusados. A política candidata continua exigindo `sshd -t` e uma saída efetiva verificável: nunca considera configuração vazia como segura. Erros mostram as tentativas `-T`/`-G` diretamente no terminal da CLI.

## Firewall

A opção permanece escurecida e sem execução, com **Requer SSH configurado**, até detectar `id_laptop`, `id_laptop.pub`, `authorized_keys` válido (pode estar vazio), serviço SSH ativo/habilitado, porta respondendo ao protocolo SSH localmente e política restrita a chaves. Não exige as identidades antigas do notebook, chaves de plataformas Git ou flags em `config.txt`.

Antes de alterar o firewall, solicita sudo se necessário e revalida as públicas com `ssh-keygen`, permissões e proprietário dos arquivos, política efetiva com `sshd -T`, serviço, porta e LAN privada detectada. Uma conexão administrativa fora dessa LAN é recusada. A validação não substitui um teste remoto nem uma auditoria completa do sistema.

Após isso, instala UFW e iptables-nft se ausentes, bloqueia entrada por padrão, permite saída, substitui a liberação SSH global pela regra SSH somente na LAN e habilita o serviço no boot. Outras regras UFW existentes são preservadas; revise-as antes de ativar. Execute preferencialmente localmente.

## Aparência

A ação manual **Aparência** copia `assets/face.jpg` para `~/.face` como arquivo regular e `assets/wallpapers/laptop.jpg` para `~/.config/omarchy/backgrounds/dracula/laptop.jpg`. Cria os diretórios ausentes com `os.MkdirAll` (equivalente a `mkdir -p`) e preserva os demais backgrounds. Reexecutar substitui somente esses dois arquivos, sem backup; um symlink existente em `~/.face` é substituído pela cópia, sem modificar seu alvo. Não ativa o wallpaper automaticamente.

Nada é aplicado automaticamente ao abrir a CLI. Não move pastas, não cria links simbólicos e não herda `setup/notebook/` nem `setup/config/`. Além do SSH, cria somente os diretórios necessários às cópias de aparência quando essa ação é solicitada.

## Testes

```bash
go test ./...
go vet ./...
```
