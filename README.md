# ReeseArch64 Dotfiles

Configuração pessoal para CachyOS com Niri e Noctalia Shell.

O repositório fornece uma CLI interativa para instalar ferramentas, configurar serviços e aplicar arquivos do ambiente.

## Visão geral

A CLI organiza as ações em quatro áreas:

| Área | Ações |
| --- | --- |
| Sistema | SSH, Firewall, Docker e Drivers |
| Desenvolvimento | Git, GPG, Mise, JavaScript, Flutter, IDEs e ferramentas de terminal |
| Desktop | Niri, Noctalia, Ghostty, Obsidian, wallpapers e foto de perfil |
| Agentes de IA | Pi Agent |

As ações de Pacman e Shelly instalam somente os pacotes ausentes.

Antes dos passos privilegiados, a CLI valida as credenciais do `sudo`. O cache pode evitar prompts, e credenciais expiradas podem exigir nova autenticação.

## Conteúdo

- [Requisitos](#requisitos)
- [Instalação rápida](#instalação-rápida)
- [Navegação](#navegação)
- [Sistema](#sistema)
- [Desenvolvimento](#desenvolvimento)
- [Desktop](#desktop)
- [Pi Agent](#pi-agent)
- [Estrutura do repositório](#estrutura-do-repositório)
- [Desenvolvimento da CLI](#desenvolvimento-da-cli)
- [Solução de problemas](#solução-de-problemas)
- [Segurança](#segurança)

## Requisitos

A CLI exige este ambiente:

- CachyOS
- Niri
- Noctalia Shell

Ela encerra a execução quando algum desses componentes está ausente.

Também são necessários:

- Go compatível com `go.mod`
- Git
- Bash
- curl
- Acesso à internet
- Pacman
- Shelly
- sudo
- systemd

Algumas ações possuem requisitos próprios. Eles estão descritos nas seções correspondentes.

Pi Agent e LunarVim executam scripts remotos baixados com `curl`. Revise as origens antes de iniciar essas ações.

## Instalação rápida

Clone o repositório no diretório padrão:

```bash
git clone https://github.com/ReeseArch64/dotfiles.git ~/.dotfiles
cd ~/.dotfiles
```

Crie o arquivo de ambiente:

```bash
cp .env.example .env
```

Preencha sua identidade e os caminhos das chaves GPG:

```dotenv
GIT_USER_EMAIL=seu-email@example.com
GIT_USERNAME=seu-usuario
GIT_USER_NAME=Seu Nome
GPG_PUBLIC_IMPORT=~/.dotfiles/minha_chave_publica.asc
GPG_PRIVATE_IMPORT=~/.dotfiles/minha_chave_privada.asc
```

Compile a CLI e crie o comando em `~/.local/bin`:

```bash
make install
```

Adicione `~/.local/bin` ao `PATH`, se necessário. Depois, execute:

```bash
dotfiles
```

Para compilar e executar sem instalar o comando:

```bash
make run
```

## Navegação

| Tecla | Ação |
| --- | --- |
| `↑`, `k` | Selecionar o item anterior |
| `↓`, `j`, `Tab` | Selecionar o próximo item |
| `Enter`, `→`, `l`, `Espaço` | Abrir ou executar o item |
| `1` a `9` | Abrir diretamente o item numerado |
| `Esc`, `←`, `h`, `Backspace` | Voltar |
| `q`, `Ctrl+C` | Sair |

Durante uma ação, aguarde todos os passos. A CLI entrega o terminal aos comandos que exigem senha, autenticação ou confirmação.

## Sistema

### Drivers

`Sistema > Drivers` instala via Pacman:

- `base-devel`
- `vulkan-tools`
- `mesa-utils`
- `linux-headers`

O pacote `mesa-utils` fornece o comando `glxinfo`.

### Docker

`Sistema > Docker > Configurar Docker` executa estas etapas:

1. Valida o executável `zen-browser` e o diretório `~/.config/zen`.
2. Instala Docker, Compose, Buildx, Lazydocker e Kind.
3. Instala `util-linux` e `xdg-utils`, necessários ao fluxo.
4. Adiciona o usuário atual ao grupo `docker`.
5. Habilita e inicia `docker.service`.
6. Define o Zen Browser como navegador padrão.
7. Executa `docker login` com o grupo atualizado.

Abra uma nova sessão após a configuração. A alteração do grupo não alcança terminais que já estavam abertos.

Membros do grupo `docker` controlam o daemon com privilégios equivalentes a acesso root.

### SSH

O menu `Sistema > SSH` mostra o serviço, o socket, a porta, o hardening e os endereços locais.

As ações de servidor instalam `openssh` via Pacman quando necessário.

As ações disponíveis são:

- Configurar o cliente SSH.
- Ativar `sshd.service` permanentemente.
- Ativar `sshd.socket` sob demanda.
- Iniciar o serviço somente na sessão atual.
- Parar e desabilitar o serviço e o socket.
- Aplicar ou remover o hardening.

A configuração do cliente exige estes arquivos regulares em `~/.ssh`:

```text
id_github_reesearch64
id_github_reesearch64.pub
id_gitlab_reesearch64
id_gitlab_reesearch64.pub
```

A ação copia `configs/ssh/config` para `~/.ssh/config`. Ela aplica `0700` ao diretório e `0600` ao arquivo.

Uma configuração diferente recebe o sufixo `.backup-AAAAMMDD-HHMMSS`.

O hardening desativa senhas, autenticação interativa e login de root. Ele exige uma chave em `~/.ssh/authorized_keys`.

A CLI valida a configuração com `sshd -t` antes de recarregar o serviço.

### Firewall

O menu `Sistema > Firewall` instala `ufw` e `iptables-nft` via Pacman quando necessário.

Ele mostra as políticas, regras, porta SSH e sub-rede local. O menu permite:

- Ativar ou desativar o UFW.
- Liberar a porta SSH para qualquer origem.
- Liberar a porta SSH somente para a rede local.
- Remover regras que liberam a porta SSH.

A ativação usa `deny incoming` e `allow outgoing`. Em sessões remotas, a CLI libera a porta SSH antes de ativar o firewall.

## Desenvolvimento

### Git

`Desenvolvimento > Git > Configurar Git`:

1. Lê a identidade do arquivo `.env`.
2. Instala `git` via Pacman.
3. Instala `lazygit` via Shelly.
4. Gera `configs/git/.gitconfig` de forma atômica.
5. Preserva o `user.signingkey` existente em `configs/git/.gitconfig`.
6. Aplica os arquivos globais.

| Destino | Origem | Formato |
| --- | --- | --- |
| `~/.gitconfig` | `configs/git/.gitconfig` | Cópia regular |
| `~/.gitattributes` | `configs/git/.gitattributes` | Link simbólico |
| `~/.gitignore` | `configs/git/.gitignore` | Link simbólico |
| `~/.config/git/config` | `configs/git/config` | Link simbólico |

A ação substitui `~/.gitconfig` e recria os três links simbólicos sem backup automático.

A configuração global ativa assinatura GPG, rebase, autosquash, `rerere` e aliases.

Ela também declara os filtros do Git LFS, mas a CLI não instala `git-lfs`. Instale esse pacote antes de trabalhar com arquivos LFS.

### GPG

`Desenvolvimento > GPG > Importar chaves` exige que a configuração do Git esteja pronta.

Copie as chaves somente quando quiser executar a importação:

```bash
install -m 600 /origem/chave_privada.asc ~/.dotfiles/minha_chave_privada.asc
install -m 644 /origem/chave_publica.asc ~/.dotfiles/minha_chave_publica.asc
```

A ação:

1. Valida os caminhos e os cabeçalhos OpenPGP.
2. Instala `gnupg` via Pacman.
3. Importa as chaves pública e privada.
4. Obtém o fingerprint da chave privada.
5. Atualiza `user.signingkey` no repositório e em `~/.gitconfig`.

A CLI não copia as chaves. O `.gitignore` exclui `.env` e arquivos `*.asc`.

### Mise

`Desenvolvimento > Mise`:

- Instala `mise` via Shelly.
- Cria o link `~/.config/mise/config.toml` para `configs/mise/mise.toml`.
- Cria `~/.config/fish/conf.d/dotfiles-mise.fish`.

Abra um novo terminal para carregar a ativação do Fish.

O arquivo `configs/mise/mise.toml` declara:

- AWS CLI
- Java Temurin
- Flutter
- Node.js
- Bun
- pnpm
- Deno
- Ruby
- Python
- Go
- uv

A ação Mise configura o gerenciador. Os ambientes JavaScript e Flutter possuem ações próprias de instalação.

### JavaScript

`Desenvolvimento > Ambiente JavaScript` configura o Mise e instala:

- Node.js
- npm, incluído com Node.js
- Bun
- Deno
- pnpm

Ao final, a CLI executa `npm login`.

### Flutter

`Desenvolvimento > Ambiente Flutter` configura o Mise e instala a versão declarada em `configs/mise/mise.toml`.

### IDEs

`Desenvolvimento > Instalar IDEs` instala:

- `visual-studio-code-bin` via Shelly/AUR
- `zed` via Shelly

### Ferramentas de terminal

`Desenvolvimento > Ferramentas de terminal` instala os seguintes grupos.

**Shelly:**

- Neovim
- wget
- curl
- bat
- eza
- scc
- viddy, compilado pelo pacote AUR

**Pacman:**

- Yazi
- Hurl
- Glow
- FFmpeg
- mpv
- yt-dlp
- scrcpy
- android-tools
- ncdu
- tealdeer
- hyperfine
- atuin
- zoxide
- starship
- btop
- yq
- jq
- fd
- ripgrep
- fzf

Quando `lvim` está ausente, a CLI instala LunarVim com a branch `release-1.4/neovim-0.9`.

A ação também copia `configs/btop/` para `~/.config/btop`. Uma configuração diferente recebe backup com timestamp.

## Desktop

### Niri

`Desktop > Niri` copia `configs/niri/` para `~/.config/niri`.

### Noctalia

`Desktop > Noctalia` exige estes plugins:

- `github-kanban`
- `mini-docker`
- `noctaproton-vpn`
- `zed-provider`

A CLI procura os plugins neste diretório:

```text
~/.local/state/noctalia/plugins/materialized/community
```

A CLI não baixa plugins. Após a validação, ela copia `settings.toml` e `state.toml` para o estado do Noctalia.

### Ghostty

`Desktop > Ghostty` instala `ghostty` via Shelly e copia `configs/ghostty/` para `~/.config/ghostty`.

### Obsidian

`Desktop > Obsidian` instala `obsidian-bin` via Shelly/AUR e copia `configs/obsidian/` para `~/.obsidian`.

### Wallpapers

`Desktop > Wallpapers` copia `assets/wallpapers/` para `~/.wallpapers`.

A pasta contém imagens para desktop, Android e iPhone.

### Foto de perfil

`Desktop > Foto de perfil` cria `~/.face` como link simbólico para `assets/face.jpg`.

A ação substitui o destino existente. Faça uma cópia manual quando quiser preservar a foto atual.

### Backups de diretórios

Niri, Ghostty, Obsidian, wallpapers e btop usam cópias regulares.

Quando o destino é diferente, a CLI o renomeia com o sufixo `.backup-AAAAMMDD-HHMMSS`. Conteúdo idêntico não gera outro backup.

## Pi Agent

`Agentes de IA > Pi Agent > Configurar Pi Agent`:

1. Valida o settings e o tema Noctalia.
2. Instala o Pi pelo script oficial quando necessário.
3. Copia os arquivos para `~/.pi/agent`.
4. Preserva arquivos diferentes com backup datado.
5. Executa `pi update --extensions`.

A configuração inclui o repositório `nothingrotf/pi-extensions` e seus pacotes `ask`, `compact`, `fast-mode`, `filetools`, `goal`, `hud`, `inline-skill`, `loop`, `session-history`, `subagent`, `tgrep`, `todo` e `pstack`.

Ela também inclui `@gotgenes/pi-anthropic-auth` e `pi-antigravity`. A atualização pode alterar pacotes que já estão instalados.

Após a configuração:

1. Abra um novo terminal.
2. Execute `pi` dentro de um projeto.
3. Use `/login` para autenticar o provedor Codex.
4. Use `/login anthropic` e `/login antigravity` quando necessário.
5. Use `/setup-pstack` para configurar os modelos do pstack.
6. Reinicie o Pi.

Nunca versione estes arquivos:

```text
~/.pi/agent/auth.json
~/.pi/agent/antigravity-accounts.json
```

Os pacotes do Pi executam código com as permissões do usuário. Revise as origens antes de atualizar extensões.

## Estrutura do repositório

```text
.
├── assets/
│   ├── face.jpg
│   └── wallpapers/
├── cmd/dotfiles/          # Código e testes da CLI
├── configs/
│   ├── btop/
│   ├── ghostty/
│   ├── git/
│   ├── mise/
│   ├── niri/
│   ├── noctalia/
│   ├── obsidian/
│   ├── pi/
│   └── ssh/
├── .env.example
├── go.mod
├── go.sum
└── Makefile
```

## Desenvolvimento da CLI

```bash
make cli       # Compila bin/dotfiles
make run       # Compila e executa a CLI
make install   # Instala o comando em ~/.local/bin
make clean     # Remove bin/
go test ./...
go vet ./...
```

Use outro checkout com `DOTFILES_DIR`:

```bash
DOTFILES_DIR=/caminho/para/dotfiles dotfiles
```

Sem essa variável, a CLI resolve o repositório pelo executável. Se isso falhar, ela usa `~/.dotfiles`.

## Solução de problemas

### Falha em pacote AUR pelo Shelly

Instale manualmente para obter a mensagem completa:

```bash
shelly install aur nome-do-pacote
```

`AUR operation failed` é uma mensagem geral. Consulte as linhas anteriores e o bloco `Technical details` para encontrar a causa.

Para pacotes oficiais, use:

```bash
shelly install standard nome-do-pacote
```

### Comando `dotfiles` não encontrado

Confirme o link e o `PATH`:

```bash
ls -l ~/.local/bin/dotfiles
printf '%s\n' "$PATH"
```

### Remoção da CLI

Remova somente o comando e o binário:

```bash
rm -f ~/.local/bin/dotfiles
make clean
```

Essa remoção não desfaz serviços, pacotes, regras de firewall ou configurações aplicadas.

## Segurança

- Nunca versione `.env`, arquivos `*.asc`, chaves SSH privadas ou credenciais.
- Revise cada ação antes de selecioná-la. A autorização do `sudo` cobre os passos privilegiados seguintes.
- Configure uma chave autorizada antes de ativar o hardening SSH.
- Confirme a porta SSH antes de ativar o firewall remotamente.
- Revise pacotes do AUR e extensões do Pi antes da instalação.

## Licença

Consulte [LICENSE](LICENSE).
