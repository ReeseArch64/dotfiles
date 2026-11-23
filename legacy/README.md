# ReeseArch64 Dotfiles

Esta pasta arquiva a versão anterior da CLI. Consulte o [README atual](../README.md) para instalar ou desenvolver a versão em uso.

Configuração pessoal para CachyOS com Niri e Noctalia Shell e para Laptop com Omarchy.

O executável legado abre apenas a opção **Sair**. As ações e os requisitos abaixo documentam o código arquivado para a migração.

## Visão geral

As ações de configuração implementadas no repositório se dividem em cinco áreas.

| Área | Ações |
| --- | --- |
| Sistema | SSH, firewall, Docker e remoção de aplicativos pré-instalados no Notebook |
| Desenvolvimento | Git, GPG, Linear CLI e ferramentas de terminal no Notebook, Android CLI nos dois perfis e Proton VPN CLI no Desktop |
| Desktop | Foto de perfil, Niri, Noctalia, Firefox e papéis de parede no Notebook |
| Agentes de IA | Pi Agent no Notebook e Ollama com GPU AMD no Desktop |
| Backup | Ponto de restauração Btrfs, projetos locais e chaves SSH/GPG no Notebook |

### Migração do Desktop

A [CLI atual](../README.md) reúne os instaladores e as configurações migradas do Desktop, incluindo Noctalia, as ferramentas de terminal, as pastas pessoais em `/mnt/storage`, o link `~/Projects` para `/mnt/workspaces` e os arquivos gerenciados do Niri. Desde a v1.31.1, `config.kdl`, `cfg/input.kdl` e `cfg/display.kdl` são arquivos locais do Desktop. A configuração arquivada do Notebook inclui seus próprios arquivos do Niri, Noctalia e Pi Agent.

| Área | O que falta migrar |
| --- | --- |
| Desenvolvimento e IA | [Android CLI do Google](#android-cli) e [Ollama com ROCm para GPU AMD](#ollama). O pacote `android-tools` já está na CLI atual. |

No Desktop, o Fastfetch da CLI atual cria `~/.face` quando a foto está ausente. O ambiente Sandbox usa Docker em **Programação → Docker → Ambiente Sandbox**. Zed, Obsidian, Turing Smart Screen e Discord estão em **Programas** na CLI atual. Os papéis de parede ficam em `../cachyos/wallpapers/`, que também é a fonte dos fluxos arquivados.

Ficaram fora da migração do Desktop o Proton VPN CLI, a lixeira `~/.trash`, o login automático, o Thunderbird, o VS Code, o Android Studio, o Better Stack e as ações de backup de Btrfs, projetos, chaves SSH/GPG e perfis de navegador. O mapeamento histórico de `~/.vscode` e as ações de backup do Notebook estão descritos nas seções do código arquivado.

As ações de Pacman e yay instalam somente os pacotes ausentes. Antes de cada instalação, a CLI atualiza as bases com `sudo pacman -Syy`.

A CLI também ignora instaladores de ferramentas já disponíveis e autenticações detectadas. Ela confere o estado imediatamente antes de cada etapa, inclusive após instalar ferramentas na mesma execução. Se uma verificação não confirmar a sessão, o login será solicitado novamente. As credenciais locais do Docker podem ter expirado.

Quando o yay falha, a CLI cria `~/.aur`, clona o pacote do AUR e executa `makepkg -si`. Depois, ela confirma a instalação com `pacman -Q`.

Antes dos passos privilegiados, a CLI valida as credenciais do `sudo`. O cache pode evitar prompts, e credenciais expiradas podem exigir nova autenticação.

## Conteúdo

- [Requisitos](#requisitos)
- [Configurações dos perfis](#configurações-dos-perfis)
- [Navegação](#navegação)
- [Sistema](#sistema)
- [Desenvolvimento](#desenvolvimento)
- [Desktop](#desktop)
- [Pi Agent](#pi-agent)
- [Ollama](#ollama)
- [Backup](#backup)
- [Estrutura do repositório](#estrutura-do-repositório)
- [Desenvolvimento da CLI](#desenvolvimento-da-cli)
- [Solução de problemas](#solução-de-problemas)
- [Segurança](#segurança)

## Requisitos

Os requisitos abaixo se referem às ações de configuração em desenvolvimento.

Ao executar essas ações, a CLI identifica o perfil sem perguntar ao usuário. O hostname deve conter exatamente um destes termos, sem diferenciar maiúsculas de minúsculas:

- `desktop`: disponibiliza as ações legadas ainda associadas ao Desktop;
- `notebook`: disponibiliza somente o conjunto específico para o portátil;
- `macbook`: informa que o dispositivo ainda não possui suporte e encerra;
- `laptop-alan`: perfil Laptop (Omarchy Linux, `ID=omarchy`), com SSH, Firewall UFW, Navegador Web > Firefox (exige `~/.config/mozilla` existente e prepara `/mnt/backups/dotfiles/Restore Firefox` com sudo para o usuário) e a ação manual Aparência (`~/.face` e wallpaper em `~/.config/omarchy/backgrounds/dracula/laptop.jpg`). Gera `id_laptop` Ed25519 pedindo e-mail e passphrase, e permite adicionar chaves públicas por colagem, visualizar e remover em **SSH > Chaves autorizadas**. O servidor permite apenas autenticação por chave e proíbe root. A configuração aceita zero chaves autorizadas e não exige teste de login remoto: verifica o serviço ativo/habilitado e a porta SSH respondendo localmente. O Firewall fica escurecido até verificar essa configuração; quando aplicado, permite SSH somente na LAN. Não usa flags em `config.txt`. É independente do notebook CachyOS (`setup/notebook/`), em `setup/laptop/`, sem herdar `setup/config/`. Não move pastas nem cria links simbólicos; cria apenas os diretórios necessários às ações solicitadas. O fluxo SSH é implementado em Go, sem scripts Python ou shell. Veja [o fluxo e as precauções do laptop](setup/laptop/README.md).

Hostnames sem um desses termos, ou contendo mais de um deles, são rejeitados.

Nos perfis Desktop e Notebook, a CLI exige este ambiente:

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
- sudo
- systemd

O `yay` é instalado automaticamente, quando necessário, compilando o pacote `yay` do AUR (não `yay-bin`) em `~/.aur/yay` com `makepkg -si`. A CLI instala `git` e `base-devel` via Pacman antes da compilação.

Algumas ações possuem requisitos próprios. Eles estão descritos nas seções correspondentes.

CasaOS, Pi Agent, Ollama e LunarVim executam scripts remotos baixados com `curl`. Revise as origens antes de iniciar essas ações.

## Configurações dos perfis

Os perfis arquivados usam `setup/desktop/` e `setup/notebook/`. A resolução procura primeiro o arquivo do perfil e, quando existir uma base em `setup/config/`, permite complementá-lo com os arquivos dessa base.

O Notebook mantém seu `ssh/config` e tem seus próprios arquivos do Niri, Fish, Noctalia e Pi Agent. Reutiliza o `misc.kdl`, o cursor, o Fastfetch e o Mise de `../cachyos/` e usa os arquivos estáticos de Git dessa mesma árvore junto com `setup/notebook/git/.gitconfig`. A CLI atual tem os arquivos gerenciados do Niri para Desktop e preserva o config, o input e o display locais.

Ollama continua exclusivo de `setup/desktop/`. Nos caminhos abaixo, `setup/<perfil>/` representa `setup/desktop/` ou `setup/notebook/`. No Notebook, a ação de Git substitui os links antigos por cópias regulares. A ação do Mise substitui o link antigo por uma cópia regular da configuração compartilhada.

O Laptop e o MacBook usam diretórios separados dos perfis CachyOS. O MacBook ainda não tem ações suportadas.

A identidade do Git no Notebook fica definida em `setup/notebook/git/.gitconfig`.

No Notebook, a ação `Desenvolvimento > GPG` gera as chaves GPG localmente.

A CLI atual prepara os destinos quando cada ação é selecionada. As rotinas internas de migração do legado registram a preparação histórica de workspaces, SDK, storage e backups.

A tabela registra os mapeamentos históricos da migração SDK dos dois perfis:

| Link no home | Destino |
| --- | --- |
| `~/.aur` | `/mnt/sdk/Aur` |
| `~/.cache` | `/mnt/sdk/Cache` |
| `~/Android` | `/mnt/sdk/Android` |
| `~/.gradle` | `/mnt/sdk/Gradle` |
| `~/.bun` | `/mnt/sdk/Bun` |
| `~/.pi` | `/mnt/sdk/Pi` |
| `~/.codex` | `/mnt/sdk/Codex` |
| `~/.npm` | `/mnt/sdk/Npm` |
| `~/.rustup` | `/mnt/sdk/Rustup` |
| `~/.agent-browser` | `/mnt/sdk/AgentBrowser` |
| `~/.cargo` | `/mnt/sdk/Cargo` |
| `~/.expo` | `/mnt/sdk/Expo` |
| `~/.vscode` | `/mnt/sdk/Vscode` |
| `~/.vite-plus` | `/mnt/sdk/VitePlus` |
| `~/go` | `/mnt/sdk/Go` |
| `~/.docker` | `/mnt/sdk/Docker` |
| `~/.dotnet` | `/mnt/sdk/Dotnet` |
| `~/.java` | `/mnt/sdk/Java` |
| `~/.copilot` | `/mnt/sdk/Copilot` |
| `~/.azure` | `/mnt/sdk/Azure` |
| `~/.aws` | `/mnt/sdk/Aws` |
| `~/.gemini` | `/mnt/sdk/Gemini` |
| `~/.railway` | `/mnt/sdk/Railway` |
| `~/.tauri` | `/mnt/sdk/Tauri` |

A CLI atual usa os mesmos mapeamentos de pastas pessoais da implementação arquivada:

| Link no home | Destino |
| --- | --- |
| `~/Documents` | `/mnt/storage/Documents` |
| `~/Downloads` | `/mnt/storage/Downloads` |
| `~/Pictures` | `/mnt/storage/Pictures` |
| `~/Videos` | `/mnt/storage/Videos` |
| `~/Music` | `/mnt/storage/Music` |

Se já houver dados no destino, eles são mesclados: os arquivos do home prevalecem, mas conflitos são preservados em backup. Os diretórios originais do home e os destinos anteriores são mantidos integralmente em backups dentro de `/mnt/backups/dotfiles`, antes da criação dos links. Arquivos exclusivos do destino são mantidos na mesclagem. Links que já apontem para o destino correto não são migrados novamente; links para outros destinos são rejeitados, sem redirecionamento silencioso.

Se uma pasta a migrar não puder ser lida por falta de permissão, a CLI solicita `sudo` para corrigir seu proprietário/grupo para o usuário atual (`chown -R -P --no-dereference`) e adicionar acesso ao dono (`chmod -R u+rwX`), antes de tentar novamente. Isso se limita à árvore afetada, no home ou no destino; não segue links simbólicos externos, não concede acesso a outros usuários e não apaga dados. As permissões corrigidas também ficam nos backups, mesmo se a migração falhar depois.

**Feche os aplicativos antes da primeira migração**, especialmente os que usam `~/.cache`. A cópia preserva permissões, datas e links simbólicos, inclusive entre discos diferentes. Reserve espaço para os dados copiados e os backups. Arquivos especiais (por exemplo, sockets ativos) e caminhos de origem/destino inseguros interrompem a preparação sem ativar os links do grupo afetado (SDK ou storage). Os backups são preservados para conferência; a CLI não os remove automaticamente.

Para compilar e executar sem instalar o comando:

```bash
make run
```

### Ações do Notebook

O código do Notebook conserva estas ações:

- Sistema: SSH, Firewall, Docker e remoção de aplicativos pré-instalados;
- Desenvolvimento: Git, GPG, Ambiente de Desenvolvimento, Linear CLI, Android CLI e ferramentas de terminal;
- Desktop: Niri, login automático, Noctalia, Mozilla Firefox, papéis de parede e foto de perfil;
- Agentes de IA: somente Pi Agent;
- Backup: todas as ações disponíveis.

As demais ações ficam exclusivas do Desktop.

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

### Aplicativos pré-instalados

No Notebook, `Sistema > Aplicativos pré-instalados > Remover aplicativos` remove estes pacotes do CachyOS:

- `alacritty`
- `meld`
- `shelly`

A ação exige que o pacote `kitty` esteja instalado antes de remover o Alacritty. O submenu **Sistema Operacional → Terminal → Instalar Kitty** da CLI atual instala esse pacote.

A CLI atualiza as bases e executa `pacman -Rns` somente com os pacotes da lista que ainda estão instalados. Confirme a lista apresentada pelo Pacman antes de continuar.

Após a desinstalação, a ação remove estes diretórios do usuário atual:

```text
~/.config/alacritty
~/.cache/alacritty
~/.config/meld
~/.local/share/meld
~/.cache/meld
```

Esse conteúdo não recebe backup. Preserve manualmente qualquer dado necessário antes de executar a ação.

### Docker

No Notebook, `Sistema > Docker > Configurar Docker` executa estas etapas:

1. Valida o executável `firefox`, sem exigir um perfil pré-existente.
2. Instala Docker, Compose, Buildx e Lazydocker.
3. Instala `util-linux` e `xdg-utils`, necessários ao fluxo.
4. Cria `/etc/docker` com `sudo` e grava `/etc/docker/daemon.json` com o conteúdo abaixo, substituindo qualquer configuração existente.
5. Adiciona o usuário atual ao grupo `docker`.
6. Habilita e inicia `docker.service`.
7. Define o Mozilla Firefox como navegador padrão.
8. Executa `docker login` com o grupo atualizado.

```json
{
  "data-root": "/var/lib/docker",
  "storage-driver": "overlay2"
}
```

Abra uma nova sessão após a configuração. A alteração do grupo não alcança terminais que já estavam abertos.

Membros do grupo `docker` controlam o daemon com privilégios equivalentes a acesso root.

### SSH

O menu `Sistema > SSH` oferece somente a ação `Configurar SSH`.

A ação executa a configuração completa:

1. Valida as identidades e copia `setup/<perfil>/ssh/config` para `~/.ssh/config`.
2. Substitui `~/.ssh/authorized_keys` pelo conjunto permitido para o dispositivo.
3. Instala `openssh`, `ufw` e `iptables-nft` quando necessário.
4. Define as políticas padrão do UFW.
5. Libera a porta do SSH somente para a rede local.
6. Ativa o UFW permanentemente.
7. Desabilita `sshd.socket`.
8. Habilita e inicia `sshd.service` permanentemente.

O Notebook exige o par `id_trabalho_notebook`, a chave pública `id_trabalho_desktop.pub` e os pares compartilhados `id_github`, `id_gitlab` e `id_gitea`. A ação mantém `id_trabalho_desktop.pub` em `authorized_keys`. O alias `gitea` usa a porta 2222 e a identidade `~/.ssh/id_gitea`.

A ação habilita `ssh-agent.socket` para o usuário, cria `~/.ssh/known_hosts` e preserva configurações substituídas em `/mnt/backups/dotfiles`.

### Firewall

O menu `Sistema > Firewall` no Notebook instala `ufw` e `iptables-nft`, bloqueia entradas, permite saídas por padrão e libera o SSH somente na rede local.

## Desenvolvimento

### Git

No Notebook, `Desenvolvimento > Git > Configurar Git`:

1. Instala `git` e `github-cli` via Pacman.
2. Instala `lazygit` e `glab` via yay.
3. Aplica os arquivos globais.
4. Exige que as chaves e o arquivo `~/.ssh/config` já estejam configurados.
5. Executa `gh auth login --git-protocol ssh` quando o GitHub não está autenticado.
6. Executa `glab auth login --git-protocol ssh` quando o GitLab não está autenticado.

| Destino | Origem | Formato |
| --- | --- | --- |
| `~/.gitconfig` | `setup/<perfil>/git/.gitconfig` | Cópia regular |
| `~/.gitattributes` | `../cachyos/git/.gitattributes` | Cópia regular |
| `~/.gitignore` | `../cachyos/git/.gitignore` | Cópia regular |
| `~/.config/git/config` | `../cachyos/git/config` | Cópia regular |

A ação substitui `~/.gitconfig` e recria os três links simbólicos sem backup automático.

Todos os arquivos da tabela são copiados atomicamente, substituindo eventuais links simbólicos antigos sem alterar seus alvos. O status exige cópias regulares com conteúdo igual à origem; links, mesmo com conteúdo igual, aparecem como não configurados. Mudanças futuras no repositório exigem executar `Configurar Git` novamente para atualizar as cópias.

A configuração global ativa assinatura GPG, rebase, autosquash, `rerere` e aliases.

Ela também declara os filtros do Git LFS, mas a CLI não instala `git-lfs`. Instale esse pacote antes de trabalhar com arquivos LFS.

### GPG

No Notebook, `Desenvolvimento > GPG > Gerar chave por dispositivo` exige que a configuração do Git esteja pronta.

A ação:

1. Lê `user.name` e `user.email` de `setup/<perfil>/git/.gitconfig` e inclui o hostname na identidade da chave.
2. Instala `gnupg` via Pacman, quando necessário.
3. Solicita uma senha não vazia e sua confirmação no terminal, sem exibi-la. A senha é entregue ao GnuPG por descritor, nunca por argumentos, ambiente ou arquivo.
4. Gera uma chave Ed25519 de assinatura, com validade de dois anos, no keyring local (`~/.gnupg`, ou `GNUPGHOME` quando definido). Não importa chaves externas nem exporta a chave privada.
5. Identifica o fingerprint exato informado pelo GnuPG para a nova chave.
6. Exibe o bloco ASCII completo da chave pública no terminal para copiar e cadastrar manualmente no GitHub, GitLab e Gitea. Aguarda Enter antes de continuar.
7. Preenche `user.signingkey` com esse fingerprint no arquivo do perfil selecionado e em `~/.gitconfig`.

Use um e-mail verificado nas plataformas para associar as assinaturas à sua conta. A geração não envia chaves automaticamente e não remove chaves existentes. O GnuPG pode recusar uma nova geração com a mesma identidade já presente. Para guardar uma cópia protegida da chave privada, use `Backup > Backup das Chaves GPG`.

### Ambiente de Desenvolvimento

No Desktop, use `Programação > Instalar Rust`, `Programação > Mise`, `Programação > Login das CLIs` e `Sistema Operacional > Fish` na CLI atual.

No Notebook, `Desenvolvimento > Ambiente de Desenvolvimento` executa toda a configuração em uma única ação:

1. Instala o `mise` via yay.
2. Copia `../cachyos/mise/mise.toml` para `~/.config/mise/config.toml`, com prioridade para o arquivo específico do perfil quando existir.
3. Marca a configuração do Mise como confiável.
4. Instala Fish, `rustup`, o toolkit `tk` para Python e as dependências de compilação do PHP via Pacman (`sudo pacman -S --needed`): `base-devel`, `autoconf`, `bison`, `re2c`, `pkgconf`, `libxml2`, `openssl`, `curl`, `libzip`, `sqlite`, `oniguruma`, `libpng`, `libjpeg-turbo`, `freetype2`, `icu`, `readline`, `gettext`, `libsodium` e `libxslt`.
5. Instala todas as ferramentas declaradas no `mise.toml`, incluindo PHP na versão `latest`.
6. Autentica AWS, Google Cloud, Railway, Firebase e Cloudflare. Tenta autenticar no Azure, mas continua se o login falhar.
7. Copia a configuração do Fish para `~/.config/fish/config.fish`. A fonte fica em `setup/notebook/fish/config.fish`.
8. Instala e seleciona a toolchain Rust estável.
9. Valida `rustc --version` e `cargo --version`.
10. Executa `npm login`.

O arquivo compartilhado `../cachyos/mise/mise.toml` instala:

- AWS CLI
- Google Cloud CLI
- Railway CLI
- Firebase CLI
- Azure CLI
- Cloudflare CLI (`cf`, pacote npm `cf`)
- Cloudflare Wrangler
- Vercel CLI (`vercel`, pacote npm `vercel`), no Desktop e Notebook
- Neon CLI (`neon`, pacote npm `neon`), no Desktop e Notebook
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
- Ruby
- PHP
- Python
- Go
- uv

Após a instalação pelo Mise, a ação executa estes fluxos interativos quando não há autenticação ativa:

```text
aws login
gcloud auth login
railway login
firebase login
cf auth login
az login
```

O Wrangler fica disponível como `wrangler`. Ele não exige login separado nesta ação.

A Vercel CLI é instalada pelo Mise e fica disponível como `vercel`. Para autenticar, execute `vercel login` manualmente após a instalação. No Notebook, use `Desenvolvimento > Ambiente de Desenvolvimento`. No Desktop, use `Programação > Mise` na CLI atual.

O Mise também instala a Neon CLI. No Notebook, execute `neon login` após instalar. No Desktop, use `Programação > Login das CLIs > Neon` na CLI atual.

As configurações Fish dos perfis Desktop e Notebook incluem o alias `gpa` (`git push origin && git push ogle && git push backup`): envia para GitHub, GitLab e Gitea, interrompendo se algum push falhar. Os três remotos devem estar configurados no repositório. O alias `piu` (`pi update --all`) substitui `pu` e atualiza tanto o Pi Agent quanto suas extensões.

A cópia substitui o `config.fish` atual. Ela remove o antigo arquivo `conf.d/dotfiles-mise.fish` para evitar ativação duplicada. Também remove `~/.bashrc`, `~/.bash_logout`, `~/.bash_profile` e `~/.zshrc`.

Abra um novo terminal para carregar a configuração do Fish e a ativação do Mise.

### Android CLI

`Desenvolvimento > Android CLI` executa o instalador:

```bash
curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash
```

O comando é executado com `bash -o pipefail` para detectar também falhas no download.

### Ferramentas de terminal

No Desktop, use **Programas → Ferramentas de Terminal** na CLI atual. A opção **Desenvolvimento → Proton VPN CLI** continua no código legado.

No Notebook, a ação instala os seguintes grupos.

**yay:**

- Neovim
- scc
- viddy, compilado pelo pacote AUR
- mprocs, fornecido pelo modo de compatibilidade do Dekit e instalado pelo script oficial
- Posting, instalado pelo `uv` com Python 3.13
- usql-bin, instalado pelo pacote AUR
- proton-vpn-cli, instalado pelo pacote AUR (Desktop e Notebook)
- proton-pass-cli-bin, instalado pelo pacote AUR

**Pacman:**

- Yazi
- SSHFS, para acessar pastas remotas pelo Yazi
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
- Chafa, usado pelo Fastfetch para renderizar o logo PNG
- yq
- jq
- fd
- ripgrep
- fzf
- wl-clipboard
- just
- rate-mirrors
- CMake
- Ninja
- git-delta
- Ventoy
- eza
- bat
- wget
- curl
- uv

Quando `lvim` está ausente, a CLI instala LunarVim com a branch `release-1.4/neovim-0.9`.

A ação copia `../cachyos/fastfetch/config.jsonc` para `~/.config/fastfetch/config.json` sem alterar outros arquivos do diretório, priorizando um arquivo específico do perfil caso exista.

## Desktop

### Login automático

No Notebook, execute `Desktop > Login automático` para entrar direto no Niri ao ligar, sem pedir senha no SDDM. A ação usa o usuário que executou a CLI (não execute com `sudo`), preserva as outras configurações e não reinicia a sessão atual. O efeito começa na próxima inicialização.

A configuração fica em `/etc/sddm.conf`; o arquivo anterior, quando existente, é salvo uma única vez em `/etc/sddm.conf.before-autologin`. Para desfazer, restaure esse backup ou remova a seção `[Autologin]` criada pela ação. A senha da conta, o sudo, o bloqueio de tela e uma eventual senha de criptografia do disco não são alterados. Qualquer pessoa com acesso físico poderá entrar na sessão ao ligar o computador.

### Niri

No Desktop, use **Sistema Operacional → Niri** na CLI atual. O autostart, as regras, os atalhos e os demais arquivos gerenciados estão em `../cachyos/niri/`. Os arquivos `config.kdl`, `cfg/input.kdl` e `cfg/display.kdl` são mantidos localmente.

No Notebook, a ação arquivada aplica `setup/notebook/niri/` e o cursor FrierenBLZ. O arquivo `cfg/display.kdl` do Notebook contém somente `eDP-1`.

### Noctalia

No Desktop, use **Sistema Operacional → Noctalia** na CLI atual. No Notebook, `Desktop > Noctalia` exige estes plugins:

- `containers`
- `github-kanban`
- `noctaproton-vpn`
- `zed-provider`

A CLI procura os plugins neste diretório:

```text
~/.local/state/noctalia/plugins/materialized/community
```

A CLI não baixa plugins. Após a validação, ela copia `settings.toml` e `state.toml` para o estado do Noctalia.

### Mozilla Firefox

No Notebook, `Desktop > Mozilla Firefox` instala `firefox` e `xdg-utils` via Pacman e define `firefox.desktop` como navegador padrão. No Desktop, use `Navegador Web > Instalar Firefox` na CLI atual. Os atalhos do Niri e o Noctalia usam Firefox.

O backup e a restauração de navegador e chaveiro foram removidos da CLI. Arquivos de backup criados anteriormente permanecem no disco.

### Wallpapers

No perfil Notebook do legado, `Desktop > Wallpapers` copia `../cachyos/wallpapers/` para `~/.wallpapers`. No Desktop, use `Sistema Operacional > Papéis de parede` na CLI atual.

A pasta contém onze imagens destinadas a diferentes telas:

- `browser.jpg`
- `desktop-4k.jpg`
- `desktop-ultrawide.jpg` — 3440×1440
- `dual-monitor.jpg` — 7680×2160, com laterais desfocadas para preservar a arte
- `ipad.jpg` — 2048×2732
- `ipad-mini.jpg` — 1488×2266
- `iphone.jpg`
- `laptop.jpg`
- `macbook.jpg`
- `smartphone-android.jpg`
- `tablet.jpg` — 1600×2560

### Foto de perfil

No Notebook, `Desktop > Foto de perfil` copia `../cachyos/fastfetch/face.jpg` para `~/.face`.

A ação substitui o destino existente. Faça uma cópia manual quando quiser preservar a foto atual.

### Backups de diretórios

Niri, o cursor FrierenBLZ, Obsidian e os wallpapers usam cópias regulares.

Quando o destino é diferente, a CLI preserva a versão anterior em `/mnt/backups/dotfiles`, com identificação da origem e da data e sem sobrescrever backups anteriores. Conteúdo idêntico não gera outro backup. O backup é concluído antes da substituição, inclusive quando o destino fica em outro disco.

Essa política também se aplica às configurações de SSH, Noctalia e Pi Agent, às migrações SDK e storage. Não são criados backups permanentes nos diretórios dos programas. Backups antigos já existentes nesses locais não são movidos nem apagados automaticamente.

## Pi Agent

No Notebook, `Agentes de IA > Pi Agent > Configurar Pi Agent`:

1. Valida o settings e o tema Noctalia.
2. Instala o Pi pelo script oficial quando necessário.
3. Copia os arquivos para `~/.pi/agent`.
4. Preserva arquivos diferentes com backup datado.
5. Executa `pi update --extensions`.

A configuração inclui o repositório `nothingrotf/pi-extensions` e seus pacotes `ask`, `compact`, `fast-mode`, `filetools`, `goal`, `hud`, `inline-skill`, `loop`, `session-history`, `subagent`, `tgrep`, `todo` e `pstack`.

Ela também inclui `@gotgenes/pi-anthropic-auth`, `pi-antigravity` e `pi-multi-account`. A atualização pode alterar pacotes que já estão instalados.

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

## Ollama

`Agentes de IA > Ollama` instala e configura o serviço para GPU AMD:

1. Valida o driver `amdgpu` e o dispositivo `/dev/kfd`.
2. Instala `curl`, `pciutils`, `tar`, `zstd` e `util-linux` quando necessário.
3. Executa `curl -fsSL https://ollama.com/install.sh | sh` com `pipefail` para detectar falhas no download.
4. Verifica o runtime ROCm baixado pelo instalador oficial.
5. Verifica se `/mnt/storage` é um ponto de montagem ativo.
6. Cria `/mnt/storage/ollama` e `/mnt/storage/ollama/models` com `sudo install`, proprietário `ollama:ollama` e permissão `0750`.
7. Adiciona o usuário do serviço `ollama` aos grupos `render` e `video`.
8. Aplica `setup/desktop/ollama/amd.conf` em `/etc/systemd/system/ollama.service.d/amd.conf`.
9. Habilita o serviço no boot, reinicia o Ollama e verifica a API local.

A ação ignora o instalador quando encontra o binário, o runtime ROCm e a unidade `ollama.service`.
O serviço usa `127.0.0.1:11434` e não abre portas no firewall.
`OLLAMA_MODELS` direciona os novos downloads para `/mnt/storage/ollama/models`.
A ação reaplica o proprietário e as permissões das duas pastas sem alterar recursivamente os arquivos existentes.
Modelos no diretório padrão não são migrados automaticamente.

Configure a montagem permanente de `/mnt/storage` antes de executar a ação.
O serviço exige essa montagem no boot e não inicia quando ela está ausente.
A configuração não força `HSA_OVERRIDE_GFX_VERSION` nem seleciona uma GPU por índice.

A aceleração depende de uma GPU compatível com [ROCm no Ollama](https://docs.ollama.com/gpu).
O módulo não baixa modelos automaticamente.

Para verificar a aceleração:

1. Execute `ollama run llama3.2:1b` para baixar e carregar um modelo.
2. Em outro terminal, execute `ollama ps`.
3. Confira a coluna `PROCESSOR` para identificar o uso de GPU ou CPU.
4. Se o modelo usar somente CPU, consulte `journalctl -u ollama.service -b`.

## Backup

Todos os backups locais gerados pela CLI ficam em `/mnt/backups/dotfiles`, em registros únicos por categoria, origem e data UTC:

```text
/mnt/backups/dotfiles/<categoria>/<hash-SHA256-da-origem>/<data-UTC>-<identificador>/
├── origin.json
└── original
```

As categorias são `files` (configurações e migração SDK), `ssh`, `gpg`, `dns` e `snapshots`. O conteúdo original fica em `original`; o arquivo `origin.json` informa o caminho de origem e a data (no DNS, `origin.path` registra a origem). A estrutura privada usa `0700`; configurações DNS e snapshots, executados via sudo, têm categorias próprias pertencentes ao root, sem alterar o dono do diretório principal.

Cada execução cria uma nova geração e preserva as anteriores. A CLI prepara `/mnt/backups/dotfiles` com proprietário/grupo do usuário e permissão `0700`. Para execução isolada ou testes, `DOTFILES_BACKUP_DIR=/caminho/absoluto` substitui explicitamente a raiz; erros nunca causam fallback para o diretório do programa.

### Ponto de restauração

`Backup > Ponto de Restauração` cria um snapshot Btrfs gravável do subvolume raiz dentro de `/mnt/backups/dotfiles`. O registro fica em `snapshots/<hash-da-origem>/<data-UTC>-<identificador>/original`.

A ação prepara o destino centralizado e executa `btrfs subvolume snapshot / <destino>` com privilégios de administrador. O destino precisa pertencer ao mesmo sistema de arquivos Btrfs da raiz; se `/mnt/backups/dotfiles` estiver em outro disco ou sistema de arquivos, a criação do snapshot é recusada, sem gravá-lo em outro local.

### Backup de projetos locais

`Backup > Backup de Projetos Locais` percorre toda a árvore de `/mnt/workspaces`, independentemente do proprietário ou agrupamento dos projetos, e identifica repositórios com `.git` (diretório ou arquivo, incluindo worktrees).

Para cada projeto, a CLI executa nesta ordem:

```text
git push origin
git push ogle
git push backup
```

Os remotos devem estar previamente configurados: `origin` para o GitHub, `ogle` para o GitLab e `backup` para o Gitea. A CLI não cria nem altera URLs de remotos. Projetos com qualquer um desses remotos ausente são reportados como falha, sem envio parcial. Se o envio ao GitHub falhar, a CLI não envia esse projeto aos demais serviços. Após sucesso no GitHub, tenta GitLab e Gitea independentemente. Ela continua processando os outros projetos e apresenta todas as falhas ao final.

No Desktop, **Sistema Operacional → Armazenamento e projetos** da CLI atual prepara `/mnt/workspaces` e o link `~/Projects`. A ação arquivada do Notebook prepara o destino e o link antes do envio.

A ação envia somente commits e referências alcançados pelo `git push` configurado. Ela não cria commits para alterações locais pendentes.

### Backup das chaves SSH

No Notebook arquivado, `Backup > Backup das Chaves SSH` copia os pares `id_trabalho_notebook`, `id_github`, `id_gitlab`, `id_gitea` e a chave pública `id_trabalho_desktop.pub` para `/mnt/backups/dotfiles/ssh/<hash>/<geração>/original/`.

A CLI valida todos os arquivos do perfil antes de criar o backup. Se algum estiver ausente ou não for um arquivo regular, a ação informa os nomes faltantes e não inicia a cópia. Chaves privadas de outro dispositivo nunca são exigidas nem copiadas.

Os diretórios de backup SSH recebem permissão `0700`. Cada geração mantém a permissão dos arquivos da origem, sem substituir gerações anteriores. Proteja as chaves privadas em `~/.ssh` com permissão `0600`.

### Backup das chaves GPG

`Backup > Backup das Chaves GPG` exporta em formato ASCII Armor todas as chaves disponíveis no keyring GPG:

| Conteúdo | Destino | Permissão |
| --- | --- | --- |
| Chaves públicas | `/mnt/backups/dotfiles/gpg/<hash>/<geração>/original/minha_chave_publica.asc` | `0644` |
| Chaves privadas | `/mnt/backups/dotfiles/gpg/<hash>/<geração>/original/minha_chave_privada.asc` | `0600` |

A ação exige o comando `gpg`, normalmente fornecido pelo pacote `gnupg` instalado em `Desenvolvimento > GPG`. O diretório `/mnt/backups/dotfiles/gpg` recebe permissão `0700`.

As duas exportações são concluídas antes da gravação dos arquivos. A CLI rejeita resultados sem os cabeçalhos OpenPGP esperados e usa arquivos temporários dentro de uma nova geração, preservando todos os backups existentes.

## Estrutura do repositório

```text
.
├── cmd/dotfiles/          # Código e testes da CLI
├── setup/
│   ├── desktop/          # Configurações ainda arquivadas do Desktop
│   ├── notebook/         # Configurações específicas CachyOS
│   ├── laptop/           # Laptop Omarchy (SSH implementado em Go)
│   └── macbook/          # Reservado para macOS
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

## Solução de problemas

### Falha na instalação pelo yay e AUR

Quando o yay falha, a CLI tenta instalar o mesmo pacote a partir de `~/.aur/<nome-do-pacote>`.

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
