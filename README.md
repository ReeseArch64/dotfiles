# ReeseArch64 Dotfiles

Configuração pessoal para CachyOS com Niri e Noctalia Shell.

O repositório fornece uma CLI interativa para instalar ferramentas, configurar serviços e aplicar arquivos do ambiente.

## Visão geral

A CLI organiza as ações em quatro áreas:

| Área | Ações |
| --- | --- |
| Sistema | SSH, Firewall, Docker, Drivers e remoção de aplicativos pré-instalados |
| Desenvolvimento | Git, GPG, ambiente de desenvolvimento, IDEs e ferramentas de terminal |
| Desktop | Niri, Noctalia, Ghostty, Zen Browser, Obsidian, wallpapers e foto de perfil |
| Agentes de IA | Pi Agent |

As ações de Pacman e Shelly instalam somente os pacotes ausentes. Antes de cada instalação, a CLI atualiza as bases com `sudo pacman -Syy`.

Quando o Shelly falha, a CLI cria `~/.aur`, clona o pacote do AUR e executa `makepkg -si`. Depois, ela confirma a instalação com `pacman -Q`.

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

Pi Agent, LunarVim e Better Stack CLI executam scripts remotos baixados com `curl`. Revise as origens antes de iniciar essas ações.

## Instalação rápida

Clone o repositório no diretório padrão:

```bash
git clone https://github.com/ReeseArch64/dotfiles.git ~/.dotfiles
cd ~/.dotfiles
```

A identidade do Git fica definida diretamente em `configs/git/.gitconfig`.

Para importar chaves GPG, coloque estes arquivos na raiz do repositório:

```text
~/.dotfiles/minha_chave_publica.asc
~/.dotfiles/minha_chave_privada.asc
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

### Aplicativos pré-instalados

`Sistema > Aplicativos pré-instalados > Remover aplicativos` remove estes pacotes do CachyOS:

- `firefox`
- `alacritty`
- `meld`
- `micro`

A ação exige que o pacote `ghostty` esteja instalado antes de remover o Alacritty. Configure o Ghostty no menu `Desktop` primeiro.

A CLI atualiza as bases e executa `pacman -Rns` somente com os quatro pacotes que ainda estão instalados. Confirme a lista apresentada pelo Pacman antes de continuar.

Após a desinstalação, a ação remove estes diretórios do usuário atual:

```text
~/.mozilla
~/.cache/mozilla
~/.config/alacritty
~/.cache/alacritty
~/.config/meld
~/.local/share/meld
~/.cache/meld
~/.config/micro
~/.cache/micro
```

Esse conteúdo não recebe backup. Preserve manualmente qualquer dado necessário antes de executar a ação.

### Docker

`Sistema > Docker > Configurar Docker` executa estas etapas:

1. Valida o executável `zen-browser` e o diretório `~/.config/zen`.
2. Instala Docker, Compose, Buildx e Lazydocker.
3. Instala `util-linux` e `xdg-utils`, necessários ao fluxo.
4. Adiciona o usuário atual ao grupo `docker`.
5. Habilita e inicia `docker.service`.
6. Define o Zen Browser como navegador padrão.
7. Executa `docker login` com o grupo atualizado.

Abra uma nova sessão após a configuração. A alteração do grupo não alcança terminais que já estavam abertos.

Membros do grupo `docker` controlam o daemon com privilégios equivalentes a acesso root.

### SSH

O menu `Sistema > SSH` oferece somente a ação `Configurar SSH`.

A ação executa a configuração completa:

1. Valida as identidades e copia `configs/ssh/config` para `~/.ssh/config`.
2. Instala `openssh`, `ufw` e `iptables-nft` quando necessário.
3. Define as políticas padrão do UFW.
4. Libera a porta do SSH somente para a rede local.
5. Ativa o UFW permanentemente.
6. Desabilita `sshd.socket`.
7. Habilita e inicia `sshd.service` permanentemente.

A configuração do cliente exige estes arquivos regulares em `~/.ssh`:

```text
id_github_reesearch64
id_github_reesearch64.pub
id_gitlab_reesearch64
id_gitlab_reesearch64.pub
```

A ação copia `configs/ssh/config` para `~/.ssh/config`. Ela aplica `0700` ao diretório e `0600` ao arquivo.

Uma configuração diferente recebe o sufixo `.backup-AAAAMMDD-HHMMSS`.

### Firewall

O menu `Sistema > Firewall` oferece somente a ação `Configurar Firewall`.

A ação instala `ufw` e `iptables-nft`, bloqueia entradas e permite saídas por padrão. Ela remove a liberação global do SSH, libera a porta somente para a rede local e ativa o UFW permanentemente.

## Desenvolvimento

### Git

`Desenvolvimento > Git > Configurar Git`:

1. Instala `git` e `github-cli` via Pacman.
2. Instala `lazygit` e `glab` via Shelly.
3. Aplica os arquivos globais.
4. Exige que as chaves e o arquivo `~/.ssh/config` já estejam configurados.
5. Executa `gh auth login --git-protocol ssh`.
6. Executa `glab auth login --git-protocol ssh`.

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

A CLI não copia as chaves. O `.gitignore` exclui arquivos `*.asc`.

### Ambiente de Desenvolvimento

`Desenvolvimento > Ambiente de Desenvolvimento` executa toda a configuração em uma única ação:

1. Instala o `mise` via Shelly.
2. Cria o link `~/.config/mise/config.toml` para `configs/mise/mise.toml`.
3. Marca a configuração do Mise como confiável.
4. Instala todas as ferramentas declaradas no `mise.toml`.
5. Autentica AWS, Google Cloud, Railway, Firebase e Azure.
6. Instala Fish, `rustup` e o toolkit `tk` para Python via Pacman.
7. Copia `configs/fish/config.fish` para `~/.config/fish/config.fish`.
8. Instala e seleciona a toolchain Rust estável.
9. Valida `rustc --version` e `cargo --version`.
10. Instala Better Stack CLI em `~/.local/bin/bs` pelo script oficial.
11. Executa `bs auth init`.
12. Executa `npm login`.

O arquivo `configs/mise/mise.toml` instala:

- AWS CLI
- Google Cloud CLI
- Railway CLI
- Firebase CLI
- Azure CLI
- k9s
- kind
- kubectl
- Java Temurin 8, 11, 17 e 21, com Java 21 como padrão
- Gradle
- Maven
- Flutter
- Node.js e npm
- Bun
- pnpm
- Deno
- Ruby
- Python
- Go
- uv

Após a instalação pelo Mise, a ação executa estes fluxos interativos:

```text
aws login
gcloud auth login
railway login
firebase login
az login
```

A cópia substitui o `config.fish` atual. Ela também remove o antigo arquivo `conf.d/dotfiles-mise.fish` para evitar ativação duplicada.

Abra um novo terminal para carregar a configuração do Fish e a ativação do Mise.

### IDEs

`Desenvolvimento > Instalar IDEs` instala:

- `visual-studio-code-bin` via Shelly/AUR
- `zed` via Shelly

### Ferramentas de terminal

`Desenvolvimento > Ferramentas de terminal` instala os seguintes grupos.

**Shelly:**

- Neovim
- scc
- viddy, compilado pelo pacote AUR
- mprocs, compilado pelo pacote AUR
- Posting, compilado pelo pacote AUR
- usql-bin, instalado pelo pacote AUR
- proton-pass-cli-bin, instalado pelo pacote AUR

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
- wl-clipboard
- just
- rate-mirrors
- CMake
- git-delta
- Ventoy
- eza
- bat
- wget
- curl

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

### Zen Browser

Na máquina original, crie o backup com o Zen Browser fechado:

```bash
tar -cvf zen-backup.tar -C ~ .config/zen .cache/zen .local/share/keyrings
mv zen-backup.tar ~/.dotfiles/
```

`Desktop > Zen Browser` executa estas etapas:

1. Exige o arquivo regular `~/.dotfiles/zen-backup.tar`.
2. Instala `zen-browser-bin` via Shelly/AUR.
3. Aceita somente `.config/zen`, `.cache/zen` e `.local/share/keyrings` no arquivo.
4. Descompacta o conteúdo em `~/.dotfiles/zen-backup`.
5. Substitui os três diretórios correspondentes no diretório pessoal.
6. Remove de `zen-backup` os diretórios movidos para seus destinos.

A restauração substitui os dados atuais sem manter backup. Feche o Zen Browser e preserve manualmente qualquer perfil ou chave necessária.

O `.gitignore` exclui `zen-backup.tar` e `zen-backup/`. O arquivo contém dados pessoais e credenciais que nunca devem ser versionados.

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

### Falha na instalação pelo Shelly e AUR

Quando o Shelly falha, a CLI tenta instalar o mesmo pacote a partir de `~/.aur/<nome-do-pacote>`.

Se o fallback também falhar, consulte a saída do `git clone`, do `makepkg -si` e da verificação `pacman -Q` exibida no terminal.

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

- Nunca versione arquivos `*.asc`, chaves SSH privadas ou credenciais.
- Revise cada ação antes de selecioná-la. A autorização do `sudo` cobre os passos privilegiados seguintes.
- Confirme a porta SSH antes de ativar o firewall remotamente.
- Revise os arquivos PKGBUILD em `~/.aur` e as extensões do Pi antes da instalação.

## Licença

Consulte [LICENSE](LICENSE).
