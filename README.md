# ReeseArch64 Dotfiles

Dotfiles pessoais para máquinas CachyOS com Niri e Noctalia Shell. O projeto inclui uma CLI interativa para Git, SSH, UFW, wallpapers, foto de perfil, scripts e diagnóstico do sistema.

## Recursos

- Instala e configura Git, gitflow-next e Lazygit.
- Instala o mise e conecta sua configuração global.
- Instala o ambiente JavaScript com Node.js, npm, Yarn, Bun, Deno e pnpm.
- Instala o Visual Studio Code e o Zed via Shelly.
- Instala Vim, Neovim, wget, curl, bat, eza e tree via Shelly.
- Gera a identidade do Git a partir de um `.env` local.
- Instala as configurações globais do Git.
- Instala e configura Docker, Compose, Buildx, Lazydocker e Kind.
- Instala GnuPG e importa chaves públicas e privadas de arquivos `.asc`.
- Copia a configuração do Niri para `~/.config/niri`.
- Verifica os plugins e configura o Noctalia em `~/.local/state/noctalia`.
- Configura o Pi Agent com tema, settings e pacotes.
- Copia os wallpapers do repositório para `~/.wallpapers`.
- Instala a foto de perfil do repositório como `~/.face`.
- Impede a execução fora da combinação CachyOS, Niri e Noctalia Shell.
- Copia a configuração do cliente SSH para `~/.ssh/config`.
- Gerencia `sshd.service` e `sshd.socket`.
- Aplica ou remove hardening de acesso SSH.
- Instala e gerencia regras do firewall UFW.
- Exibe informações da máquina e do repositório.
- Descobre e executa scripts de `scripts/`.

## Requisitos

A CLI abre somente em ambientes com CachyOS, Niri e Noctalia Shell.

Os arquivos `~/.dotfiles/minha_chave_privada.asc` e `~/.dotfiles/minha_chave_publica.asc` são requisitos exclusivos da importação GPG.

A CLI valida essas chaves somente ao executar `GPG > Importar chaves`. Ela também confere os cabeçalhos OpenPGP nesse momento.

O navegador não é requisito para abrir a CLI. A configuração Docker exige `zen-browser` e o diretório `~/.config/zen` antes do login.

A instalação e as ações também usam:

- `bash`
- `curl`, usado pelo instalador oficial do Pi
- `git`
- `gpg`, instalado pelo pacote `gnupg` quando necessário
- `make`
- Go compatível com a versão declarada em `cli/go.mod`
- `sudo`
- `pacman`
- `systemd`
- `shelly`, usado para instalar os pacotes operacionais das opções correspondentes
- `pi`, instalado automaticamente quando necessário para configurar o Pi Agent

A CLI instala os pacotes operacionais ausentes quando a ação correspondente é executada. Go e Make ainda são necessários para compilar a CLI.

## Instalação

Clone o repositório em `~/.dotfiles`:

```bash
git clone git@github.com:ReeseArch64/dotfiles.git ~/.dotfiles
cd ~/.dotfiles
```

Crie o arquivo de ambiente:

```bash
cp .env.example .env
```

Edite `.env` com a sua identidade:

```dotenv
GIT_USER_EMAIL=seu-email@example.com
GIT_USERNAME=seu-usuario
GIT_USER_NAME=Seu Nome
GPG_PUBLIC_IMPORT=~/.dotfiles/minha_chave_publica.asc
GPG_PRIVATE_IMPORT=~/.dotfiles/minha_chave_privada.asc
```

O `.env` está no `.gitignore` e não deve ser versionado.

Copie as duas chaves de um armazenamento seguro somente se quiser executar a importação GPG:

```bash
install -m 600 /origem/chave_privada.asc ~/.dotfiles/minha_chave_privada.asc
install -m 644 /origem/chave_publica.asc ~/.dotfiles/minha_chave_publica.asc
```

Os arquivos `*.asc` permanecem ignorados pelo Git. Nunca versione a chave privada.

Compile e instale o comando:

```bash
make install
```

O comando cria `bin/dotfiles` e o link `~/.local/bin/dotfiles`. Adicione `~/.local/bin` ao `PATH` quando necessário.

Execute a interface:

```bash
dotfiles
```

Também é possível compilar e executar diretamente:

```bash
make run
```

A CLI não oferece opção para ignorar a validação de plataforma. Requisitos locais não impedem a abertura da interface.

## Mise

A opção `Desenvolvimento > Mise` instala o pacote `mise` via Shelly e cria `~/.config/mise/config.toml` como link simbólico para `mise.toml` deste repositório. Ela também ativa o mise no Fish. Abra um novo terminal para usar diretamente os comandos instalados, sem prefixá-los com `mise`.

## Ambiente JavaScript

A opção `Desenvolvimento > Ambiente JavaScript` configura o mise e instala Node.js, Yarn, Bun, Deno e pnpm nas versões declaradas em `mise.toml`. O npm acompanha a instalação do Node.js. A ação cria `~/.yarnrc` como link simbólico para `.yarnrc` deste repositório e executa `npm login` no terminal. Em novos terminais Fish, todos esses comandos ficam disponíveis diretamente.

## Ferramentas de terminal

A opção `Desenvolvimento > Ferramentas de terminal` instala Vim, Neovim, wget, curl, bat, eza e tree via Shelly.

## Navegação da CLI

| Tecla | Ação |
| --- | --- |
| `↑`, `k` | Item anterior |
| `↓`, `j`, `Tab` | Próximo item |
| `Enter`, `→`, `l`, `Espaço` | Selecionar |
| `1` a `9` | Abrir o item correspondente |
| `Esc`, `←`, `h`, `Backspace` | Voltar |
| `q`, `Ctrl+C` | Sair |

Durante um job, aguarde a conclusão dos passos. A CLI entrega o terminal aos comandos que exigem senha ou confirmação.

O menu principal contém seis opções e organiza as ações nestes submenus:

| Submenu | Opções |
| --- | --- |
| `Sistema` | SSH, Firewall e Docker |
| `Desenvolvimento` | Git, GPG, Mise, Ambiente JavaScript, Instalar IDEs e Ferramentas de terminal |
| `Desktop` | Niri, Noctalia, Wallpapers e Foto de perfil |
| `Utilitários` | Scripts e Info do sistema |
| `Agentes de IA` | Pi Agent |

## Pi Agent

Abra `Agentes de IA > Pi Agent > Configurar Pi Agent`. A ação executa estas etapas:

1. Valida `pi/agent/settings.json` e o tema `noctalia`.
2. Quando o Pi está ausente, executa `curl -fsSL https://pi.dev/install.sh | sh`. Ao final, escolha não iniciar o Pi para continuar o job.
3. Copia os dois arquivos como arquivos regulares para `~/.pi/agent`.
4. Preserva arquivos diferentes com o sufixo `.backup-AAAAMMDD-HHMMSS`.
5. Executa `pi update --extensions` para instalar ou atualizar os pacotes declarados.

A configuração instala o repositório `nothingrotf/pi-extensions`, seus pacotes `ask`, `compact`, `fast-mode`, `filetools`, `goal`, `hud`, `inline-skill`, `loop`, `session-history`, `subagent`, `tgrep`, `todo` e `pstack`. Ela também instala `@gotgenes/pi-anthropic-auth` e `pi-antigravity` pelo gerenciador de pacotes do Pi.

Os pacotes do Pi podem executar código com as permissões do usuário. Revise as origens antes de executar a configuração.

Abra uma nova sessão do Pi ou execute `/reload` após alterar manualmente os arquivos.

### Tutorial pós-configuração

Após concluir o job da CLI, finalize a configuração manual:

1. Abra um novo terminal para carregar o caminho instalado pelo script oficial.
2. Confirme a instalação e os pacotes:

   ```bash
   pi --version
   pi list
   ```

3. Inicie o Pi no diretório de um projeto:

   ```bash
   cd /caminho/do/projeto
   pi
   ```

4. Execute `/login` e autentique o provedor Codex usado pelo modelo padrão.
5. Execute `/login anthropic` se quiser usar uma assinatura Claude Pro ou Max.
6. Execute `/login antigravity` para conectar uma conta Google ao provedor Antigravity.
7. Execute `/model` e confirme que o modelo desejado está disponível.
8. Execute `/setup-pstack` e escolha os modelos usados por cada função do pstack.
9. Reinicie o Pi após configurar o pstack.

Use estes comandos para conferir as extensões opcionais:

```text
/anthropic-auth:status
/antigravity.models
/antigravity.doctor
```

Em uma máquina remota, copie a URL final do OAuth Antigravity para o prompt do Pi. Como alternativa, encaminhe a porta `51121` por SSH.

Não versione `~/.pi/agent/auth.json` nem `~/.pi/agent/antigravity-accounts.json`. Esses arquivos contêm credenciais de acesso.

O comando `/setup-pstack` grava a política em `~/.agents/rules/pstack-models.md`. Revise as escolhas antes de iniciar tarefas delegadas.

## Docker

Abra `Docker > Configurar Docker` na CLI. Essa ação:

1. Exige o executável `zen-browser` e o diretório `~/.config/zen`.
2. Instala `docker`, `docker-compose`, `lazydocker`, `docker-buildx`, `kind`, `util-linux` e `xdg-utils` com o Pacman.
3. Executa `sudo usermod -aG docker USUÁRIO`.
4. Executa `sudo systemctl enable --now docker.service`.
5. Executa `xdg-settings set default-web-browser zen.desktop`.
6. Executa `newgrp docker -c "docker login"` para autenticar com o grupo atualizado.

A CLI bloqueia toda a configuração Docker quando o Zen Browser ou seu perfil está ausente. Assim, o login nunca inicia sem esses requisitos.

A CLI entrega o terminal ao login interativo. Abra uma nova sessão após a configuração para aplicar o grupo `docker` aos outros terminais.

Membros do grupo `docker` controlam o daemon e possuem privilégios equivalentes a acesso root. Adicione somente usuários confiáveis.

## Configuração do Git

Abra `Git > Configurar Git` na CLI. Essa ação executa as seguintes etapas:

1. Lê `GIT_USER_EMAIL`, `GIT_USERNAME` e `GIT_USER_NAME` do `.env`.
2. Gera `git/.gitconfig` de forma atômica.
3. Preserva `user.signingkey` quando ele já está configurado.
4. Instala `git`, `gitflow-next-bin` e `lazygit` quando necessário.
5. Cria o diretório `~/.config/git`.
6. Copia `~/.gitconfig` e cria links para os outros três arquivos globais.

Os destinos são:

| Destino | Origem no repositório | Finalidade |
| --- | --- | --- |
| `~/.gitconfig` | `git/.gitconfig` | Identidade do usuário |
| `~/.gitattributes` | `git/.gitattributes` | Tratamento global de arquivos e diffs |
| `~/.gitignore` | `git/.gitignore` | Exclusões globais |
| `~/.config/git/config` | `git/config` | Comportamento global do Git |

`~/.gitconfig` é um arquivo regular e independente. Os outros três destinos são links simbólicos. A configuração substitui arquivos ou links existentes nesses destinos.

### Assinatura de commits

`git/config` mantém `commit.gpgsign = true`. A importação GPG identifica o fingerprint da chave privada e grava esse valor em `user.signingkey`.

A execução posterior de `Git > Configurar Git` preserva a chave configurada.

## Importação de chaves GPG

Configure o Git antes de abrir `GPG > Importar chaves`. A ação valida os quatro destinos do Git, a identidade e os pacotes instalados pelo fluxo de Git.

Defina `GPG_PUBLIC_IMPORT` e `GPG_PRIVATE_IMPORT` no `.env`. Use as cópias obrigatórias em `~/.dotfiles/minha_chave_publica.asc` e `~/.dotfiles/minha_chave_privada.asc`.

A ação executa as seguintes etapas:

1. Valida que o Git está configurado e que os dois arquivos existem.
2. Instala o pacote `gnupg` com `pacman` quando necessário.
3. Importa a chave pública com `gpg --import`.
4. Importa a chave privada com `gpg --import`.
5. Identifica o fingerprint da chave privada.
6. Grava o fingerprint em `user.signingkey` no repositório e em `~/.gitconfig`.

A CLI não copia as chaves. O padrão `*.asc` está no `.gitignore` para impedir o versionamento acidental.

### Comportamentos globais do Git

`git/config` define, entre outros ajustes:

- branch inicial `main`;
- `pull` com rebase e somente fast-forward;
- configuração automática do upstream no primeiro push;
- remoção de referências obsoletas no fetch;
- algoritmo de diff `histogram` e detecção de cópias;
- rebase com autostash e autosquash;
- `rerere` habilitado;
- editor e ferramenta de merge `vimdiff`;
- cache de credenciais por uma hora;
- Git LFS quando o executável estiver instalado;
- aliases `ci`, `co`, `cm`, `cb`, `st`, `sf` e `lg`.

## Niri

A opção `Niri` copia `niri/` para `~/.config/niri`. O destino é um diretório regular, sem links simbólicos.

Quando o destino possui conteúdo diferente, a CLI preserva a configuração anterior em `~/.config/niri.backup-AAAAMMDD-HHMMSS`. Cópias idênticas não criam backups.

## Noctalia

A tela `Noctalia` lista os plugins necessários e indica quais ainda precisam ser baixados pela interface do Noctalia:

- `github-kanban`
- `llamanager`
- `mini-docker`
- `noctaproton-vpn`
- `pomodoro`
- `ssh-launcher`
- `warp`
- `zed-provider`

A opção `Configurar Noctalia` exige todos os plugins em `~/.local/state/noctalia/plugins/materialized/community`. A CLI não baixa os plugins.

Após a validação, a CLI copia `noctalia/settings.toml` e `noctalia/state.toml` como arquivos regulares. Os demais dados do Noctalia, inclusive os plugins, permanecem no diretório de estado.

Arquivos existentes são preservados com o sufixo `.backup-AAAAMMDD-HHMMSS` antes da ativação.

## Wallpapers

A opção `Desktop > Wallpapers` copia `wallpapers/` para `~/.wallpapers`. O destino é um diretório regular, sem links simbólicos.

A pasta inclui imagens para desktop, Android e iPhone. A CLI preserva um destino diferente como `~/.wallpapers.backup-AAAAMMDD-HHMMSS`.

Uma cópia idêntica não cria outro backup. Execute a ação novamente para aplicar alterações feitas nas imagens do repositório.

## Foto de perfil

A opção `Foto de perfil` cria `~/.face` como link simbólico para `.face` no repositório. A imagem versionada é um JPEG quadrado de 300 por 300 pixels.

A ação substitui um arquivo ou link existente em `~/.face`. Faça backup da foto atual antes de executar a opção.

## SSH

O menu `SSH` mostra o estado do servidor, o modo de inicialização, a porta, o hardening, as chaves autorizadas e os endereços locais.

A ação `Configurar cliente` exige estes arquivos regulares em `~/.ssh`:

- `id_github_reesearch64`
- `id_github_reesearch64.pub`
- `id_gitlab_reesearch64`
- `id_gitlab_reesearch64.pub`

Após validar os quatro arquivos, a ação copia `ssh/config` para `~/.ssh/config`. Ela aplica permissão `0700` ao diretório e `0600` ao arquivo.

Uma configuração diferente é preservada como `~/.ssh/config.backup-AAAAMMDD-HHMMSS`. Uma cópia idêntica não cria outro backup.

As ações disponíveis são:

- **Configurar cliente:** valida as identidades e copia a configuração do cliente.
- **Ativar permanente:** habilita `sshd.service` no boot.
- **Ativar via socket:** habilita `sshd.socket` e inicia o daemon sob demanda.
- **Ativar temporário:** inicia o serviço somente na sessão atual.
- **Parar SSH:** desabilita e para o serviço e o socket.
- **Aplicar hardening:** desativa senha, autenticação interativa e login de root.
- **Remover hardening:** remove `/etc/ssh/sshd_config.d/10-hardening.conf`.

O hardening exige pelo menos uma entrada válida em `~/.ssh/authorized_keys`. A CLI valida a configuração com `sshd -t` antes de recarregar o serviço.

## Firewall

O menu `Firewall` usa UFW e mostra o serviço, as políticas, as regras de entrada, a porta SSH e a sub-rede local detectada.

As ações disponíveis são:

- ativar ou desativar o firewall;
- liberar a porta SSH para qualquer origem;
- liberar a porta SSH apenas para a LAN;
- remover regras que liberam a porta SSH.

A ativação usa `deny incoming` e `allow outgoing`. Em uma sessão SSH remota, a CLI libera a porta atual antes de ativar o UFW para reduzir o risco de perder acesso.

## Scripts

A tela `Scripts` lista arquivos `*.sh` em `scripts/`. A primeira linha de comentário após o shebang aparece como descrição.

O script `scripts/setup-ssh.sh` oferece um fluxo independente e legado para instalar OpenSSH e UFW. Consulte as opções com:

```bash
./scripts/setup-ssh.sh --help
```

Opções principais:

- `--socket`
- `--temp`
- `--stop`
- `--lan-only`
- `--subnet CIDR`
- `--harden`

## Informações do sistema

A tela `Info do sistema` apresenta:

- usuário e hostname;
- distribuição e kernel;
- uptime;
- shell e terminal;
- caminho dos dotfiles;
- branch e último commit do repositório.

## Estrutura do repositório

```text
.
├── .env.example         # Modelo da identidade Git
├── .face                # Foto de perfil instalada em ~/.face
├── Makefile             # Build, instalação, execução e limpeza
├── cli/                 # Aplicação Go com Bubble Tea
├── git/
│   ├── .gitattributes   # Atributos globais
│   ├── .gitconfig       # Identidade gerada pelo .env
│   ├── .gitignore       # Exclusões globais
│   └── config           # Preferências globais do Git
├── mise.toml            # Configuração global vinculada em ~/.config/mise
├── niri/                # Origem copiada para ~/.config/niri
├── noctalia/            # Origem copiada para o estado do Noctalia
├── pi/                   # Settings e tema do Pi Agent
├── ssh/                  # Configuração copiada para ~/.ssh/config
├── wallpapers/           # Imagens copiadas para ~/.wallpapers
└── scripts/
    └── setup-ssh.sh     # Setup alternativo de SSH e UFW
```

## Comandos de desenvolvimento

```bash
make cli       # compila bin/dotfiles
make run       # compila e executa
make install   # compila e cria o link em ~/.local/bin
make clean     # remove bin/
cd cli && go test ./...
```

Use `DOTFILES_DIR` para apontar a CLI para outro checkout:

```bash
DOTFILES_DIR=/caminho/para/dotfiles dotfiles
```

Sem essa variável, a CLI procura o repositório a partir do caminho real do executável. Se a descoberta falhar, usa `~/.dotfiles`.

## Segurança e recuperação

- Revise ações de `sudo` antes de confirmar instalações ou mudanças de serviço.
- Proteja as chaves privadas SSH e nunca as versione.
- Adicione uma chave autorizada antes de habilitar o hardening SSH.
- Libere a porta correta antes de ativar o firewall em uma máquina remota.
- Faça backup dos quatro destinos Git, de `~/.wallpapers` e de `~/.face` antes de configurá-los.
- Nunca versione `.env`, chaves privadas ou credenciais.
- Desmonte o pendrive após importar as chaves quando ele não estiver em uso.

Para remover somente o comando instalado:

```bash
rm -f ~/.local/bin/dotfiles
make clean
```

A remoção do comando não desfaz serviços, regras de firewall ou configurações instaladas. Remova `~/.face` manualmente quando quiser desfazer a foto de perfil.
