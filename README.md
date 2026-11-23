# ReeseArch64 Dotfiles

A CLI identifica a máquina pelo hostname e mostra o ambiente e o autor no menu.

| Identificador | Ambiente | Chave SSH |
| --- | --- | --- |
| `desktop` | CachyOS · Niri · Noctalia Shell | `~/.ssh/id_desktop` |
| `laptop` | CachyOS · Niri · Noctalia Shell | `~/.ssh/id_laptop` |
| `homelab` | Ubuntu | |
| `homeprod` | Ubuntu | |

O identificador pode aparecer como parte do hostname separada por hífen, como em `desktop-alan`. A CLI ignora maiúsculas e minúsculas. Nomes sem identificador ou com mais de um perfil aparecem como **Sem suporte**.

A estrutura e os atalhos dos menus são os mesmos em todos os perfis. Opções indisponíveis aparecem em cinza e ficam desabilitadas. Quando o Homelab oferece uma ação compartilhada de SSH ou firewall, ela usa o procedimento do Ubuntu.

## Segurança

Em Desktop e Laptop, **Segurança → SSH** oferece estas ações.

- **Configurar SSH** cria ou reaproveita a identidade Ed25519 do dispositivo. Instala um `~/.ssh/config` com a identidade única para qualquer destino. Guarda a configuração anterior em `/mnt/backups/dotfiles/SSH/config.backup-*`, sem transportar aliases por IP ou chaves diferentes por destino. Valida e habilita o servidor OpenSSH para o usuário atual, somente por chave pública e sem login como root. O servidor mantém a porta já configurada e guarda o `sshd_config` anterior em `/mnt/backups/dotfiles/SSHD/sshd-*/sshd_config`.
- **Criar chave SSH** gera somente o par local. A passphrase é informada ao OpenSSH no terminal. Arquivos existentes são preservados.
- **Chaves autorizadas** reúne as ações de adicionar, visualizar e remover chaves públicas em `~/.ssh/authorized_keys`. Para autorizar um dispositivo, cole sua chave pública e confira o fingerprint. O IP de origem não é vinculado à chave.

**Configurar SSH** também habilita o socket do `ssh-agent` para o usuário. Depois de informar a passphrase no primeiro uso da chave, o OpenSSH pode reutilizá-la por 8 horas em outros terminais e destinos. Reiniciar ou encerrar o agente limpa o cache.

**Segurança → Firewall** valida o SSH antes de ativar o UFW. Bloqueia entrada por padrão, permite saída e limita novas conexões SSH à interface e à sub-rede IPv4 privada detectadas. Mantém as regras de outros serviços e recusa aplicar a configuração quando a sessão SSH atual está fora da LAN. Use o console local para a primeira configuração e teste uma nova conexão antes de encerrar a sessão atual.

**Segurança → Proton Pass** exige que o Firefox já esteja instalado e que exista o diretório `~/.config/mozilla`. Instala `proton-pass-cli-bin` pelo yay, prepara o yay se precisar e executa `pass-cli login` no terminal. Se o pacote já estiver instalado, a ação inicia o login novamente.

No perfil HomeProd, apenas **Sair** fica habilitado. Desktop e Laptop podem acessar os servidores quando suas chaves públicas estiverem autorizadas.

Ao preparar um backup, a CLI garante que o usuário atual tenha acesso a `/mnt/backups` e aos subdiretórios usados pela ação. Usa `sudo` quando precisa corrigir a propriedade ou criar um diretório sem acesso de escrita no pai. Os backups da CLI são gravados em subpastas de `/mnt/backups/dotfiles`, com permissão `0700` para cada categoria.

## Homelab

As ações são disponibilizadas pelo hostname `homelab` e executadas no Ubuntu por um administrador com `sudo`.

- **Segurança → SSH → Configurar SSH** instala OpenSSH pelo APT e reaproveita `ssh.socket` quando habilitado. Cole a chave pública Ed25519 e teste uma segunda conexão com a chave correspondente e `sudo -v`. Após a segunda confirmação, a ação mantém a chave testada, exige autenticação por chave e bloqueia login como root. Ela valida a configuração antes de recarregar o serviço e guarda os arquivos anteriores em `/mnt/backups/dotfiles/SSH` e `/mnt/backups/dotfiles/SSHD`.
- **Segurança → Firewall → Configurar Firewall** instala e ativa o UFW. Permite todas as portas e protocolos de entrada pela interface da LAN IPv4 privada detectada, preserva as regras existentes e libera o IP e a porta da sessão SSH atual. Aplica a política padrão de negar entrada e permitir saída e restaura a configuração anterior se uma etapa falhar.
- **CasaOS → Instalar CasaOS** baixa o instalador oficial por HTTPS e, após validar o script, executa-o com `sudo`. A ação adiciona o administrador atual ao grupo `docker`. Abra uma nova sessão para que a associação ao grupo tenha efeito. [Instalação oficial do CasaOS](https://wiki.casaos.io/en/get-started)
- **Sistema Operacional → Servidor → Liberar porta 53 (DNS)** desativa o `DNSStubListener` do `systemd-resolved`, mantém a resolução via servidores DNS upstream e recusa drop-ins conflitantes. Se `resolv.conf` apontar para o stub, troca o link para o arquivo upstream. Em falha, restaura os arquivos alterados. [Configuração do systemd-resolved](https://www.freedesktop.org/software/systemd/man/latest/resolved.conf.html)
- **Sistema Operacional → Servidor → Configurar swap** ativa `/swapfile-homelab` com 2 GiB e registra a configuração em `/etc/fstab`. Funciona em ext4 e XFS, mantém outras swaps e reserva pelo menos 4 GiB livres ao criar o arquivo. [Manual de swapon](https://manpages.ubuntu.com/manpages/noble/man8/swapon.8.html)
- **Sistema Operacional → Fastfetch** aplica a configuração para SSH em `~/.config/fastfetch/config.jsonc` ou no diretório definido por `XDG_CONFIG_HOME`. Instala Chafa pelo APT e, se necessário, instala o pacote oficial do Fastfetch 2.70.0 com SHA-256 verificado. Usa a foto em `~/.face` ou copia a do repositório se estiver ausente. [Fastfetch 2.70.0](https://github.com/fastfetch-cli/fastfetch/releases/tag/2.70.0)
- **Sistema Operacional → Fish** instala o Fish pelo APT, aplica a configuração do servidor e confirma que `/usr/bin/fish` é o shell de login do usuário atual. Em seguida, prepara a saudação do Fastfetch, instala Bat e `bsdextrautils` e cria `~/.hushlogin`. Abra uma nova sessão SSH para entrar no Fish.
- **Programas → Ferramentas de Terminal** instala LazyDocker, `jq`, `ripgrep`, `fd-find`, `fzf`, `bat`, `git` e `ncdu`. Cria os links simbólicos `fd` e `bat` quando necessário, reaproveita o Docker existente e verifica o SHA-256 do LazyDocker 0.25.2 quando precisa baixá-lo. [LazyDocker 0.25.2](https://github.com/jesseduffield/lazydocker/releases/tag/v0.25.2)

As duas ações são implementadas em Go e guardam os arquivos substituídos em `/mnt/backups/dotfiles/DNS` e `/mnt/backups/dotfiles/Swap`.

## Sistema Operacional

**Sistema Operacional → Mouse personalizado** instala o tema FrierenBLZ em `~/.icons` e aplica `cachyos/niri/cfg/misc.kdl`. A ação exige o Niri instalado e que seu `config.kdl` inclua `cfg/misc.kdl`. Valida a configuração candidata com `niri validate`. Arquivos e temas substituídos recebem backups em `/mnt/backups/dotfiles/Niri`.

**Sistema Operacional → Papéis de parede** copia as imagens do legado para `~/.wallpapers`. No Desktop, instala `desktop-4k.jpg`, `desktop-ultrawide.jpg` e `dual-monitor.jpg`. No Laptop, instala as 11 imagens. Quando a pasta existente é diferente, a ação guarda uma cópia em `/mnt/backups/dotfiles/Wallpapers` antes de substituí-la. A escolha da imagem de fundo é feita pelo ambiente gráfico.

No Desktop e Laptop com CachyOS, **Sistema Operacional → Migrar pastas para /mnt/sdk** mostra os 27 links e pede confirmação antes de mover dados. A lista inclui `~/.local/share/mise` → `/mnt/sdk/Mise`, `~/go` → `/mnt/sdk/Go`, `~/.render` → `/mnt/sdk/Render` e `~/.maple` → `/mnt/sdk/Maple`. Em conflitos, os arquivos do home prevalecem e os originais são guardados em `/mnt/backups/dotfiles/SDK`. Feche os aplicativos que usam as pastas durante a migração. A CLI pede `sudo` ao preparar os diretórios de destino ou corrigir permissões de uma árvore inacessível.

No Desktop e Laptop com CachyOS, **Sistema Operacional → Armazenamento e projetos → Migrar pastas para /mnt/storage** cria links para `Documents`, `Downloads`, `Pictures`, `Videos` e `Music`. A ação mescla os dados, dá prioridade aos arquivos da pasta pessoal em conflitos e preserva os diretórios anteriores em `/mnt/backups/dotfiles/Storage`.

**Sistema Operacional → Armazenamento e projetos → Configurar ~/Projects** prepara `/mnt/workspaces` com permissão `0700` e cria o link `~/Projects` apontando para ele. Se o caminho já contiver um arquivo, um diretório ou um link para outro destino, a ação pede que o caminho seja liberado antes de continuar.

No Desktop, **Sistema Operacional → Niri** aplica regras, atalhos, animações, layout, autostart e demais arquivos do Dotfiles em `~/.config/niri` ou em `$XDG_CONFIG_HOME/niri`. A ação preserva `config.kdl`, `cfg/input.kdl`, `cfg/display.kdl` e os demais arquivos locais. Valida uma cópia da configuração local com os novos arquivos usando `niri validate` e guarda os arquivos substituídos em `/mnt/backups/dotfiles/Niri`. Para que o Niri carregue os arquivos, mantenha os includes correspondentes no seu `config.kdl`. A ação cria `~/Pictures/Screenshots`. O autostart da Turing Smart Screen é executado quando o programa e seu ambiente Python estão disponíveis. [Configuração e validação do Niri](https://github.com/niri-wm/niri/wiki/Configuration%3A-Overview)

**Sistema Operacional → Fish → Configurar Fish** instala as dependências ausentes e aplica `~/.config/fish/config.fish`. A ação reaproveita o Mise disponível no `PATH` ou em `~/.local/bin`. Quando os pacotes específicos do CachyOS estão disponíveis, instala também a configuração de notificações e o utilitário de mirrors. A saudação usa o Fastfetch quando ele e sua configuração estiverem disponíveis.

**Sistema Operacional → Fastfetch → Configurar Fastfetch** instala `fastfetch` e `chafa` quando necessário. Usa a foto em `~/.face` ou copia a imagem do repositório se ainda não houver uma. Aplica `~/.config/fastfetch/config.jsonc` para que o comando `fastfetch` encontre a configuração e renderize a foto com Chafa.

As duas ações pedem confirmação antes de copiar a configuração e guardam a versão anterior em `/mnt/backups/dotfiles/Fish` ou `/mnt/backups/dotfiles/Fastfetch` quando o conteúdo muda.

**Sistema Operacional → Noctalia** aplica `settings.toml` e `state.toml` do legado em `~/.local/state/noctalia`. Respeita as variáveis `NOCTALIA_STATE_HOME` e `XDG_STATE_HOME` e guarda os arquivos substituídos em `/mnt/backups/dotfiles/Noctalia`. Antes de aplicar, instale no Noctalia os plugins comunitários `androidconnect`, `github-activity`, `godot-provider`, `kuma-status`, `kumacat`, `pomodoro-heatmap`, `process-manager`, `proton-pass`, `shipyard`, `ufw` e `zed-provider`, além do plugin oficial `world_clock`. O papel de parede usa `~/.wallpapers/desktop-4k.jpg`, copiado pela ação **Papéis de parede**. [Configuração do Noctalia](https://docs.noctalia.dev/noctalia/configuration/)

## Programas

**Programas → Ferramentas Essenciais** instala `base-devel`, `curl`, `wget` e `rate-mirrors`.

**Programas → Ferramentas de Terminal** instala `bat`, `eza`, `btop`, `fzf`, `ripgrep`, `fd`, `jq`, `yq`, `zoxide`, `ncdu`, `tealdeer`, `wl-clipboard`, `atuin`, `starship`, `yazi`, `sshfs`, `glow` e `viddy`. A opção **Instalar yay** garante `git` e `base-devel` antes de clonar e compilar o pacote do AUR em `~/.aur/yay`, inclusive quando `~/.aur` aponta para `/mnt/sdk/Aur`. O instalador reutiliza o yay se ele já estiver disponível.

Em **Programas → Ferramentas de Terminal**, a instalação em lote também inclui `ffmpeg`, `mpv`, `yt-dlp`, `scrcpy`, `android-tools`, `hurl`, `hyperfine`, `just`, `cmake` e `ninja` pelo Pacman, e `ventoy`, `scc`, `usql-bin` e `mprocs` pelo yay.

**Instalar Posting** instala a versão 2 com `uv` e Python 3.13. A ação procura primeiro o `uv` instalado pelo Mise, inclusive sem ativação no shell, e depois um executável já disponível no sistema ou em `~/.local/bin`. Recorre ao Pacman quando o `uv` está ausente. **Instalar LunarVim** usa a versão 1.4 do editor com Neovim 0.9.5 pelo Mise e reaproveita essa versão quando já está instalada. Por padrão, os executáveis são disponibilizados em `~/.local/bin`. [Posting 2 e 3](https://posting.sh/guide/migrating/) · [Instalação do LunarVim](https://www.lunarvim.org/docs/installation)

**Programas → Drivers** instala os headers correspondentes aos kernels instalados, `base-devel` e os utilitários de diagnóstico `vulkan-tools` e `mesa-utils`. A CLI identifica o pacote de cada kernel pelo `vmlinuz` presente em `/usr/lib/modules`.

**Programas → Terminal → Instalar Kitty** instala o pacote `kitty` pelo Pacman após confirmação.

No Desktop e Laptop com CachyOS, **Programas → Aplicativos pré-instalados → Remover aplicativos** exige o Kitty instalado. Após confirmação, desinstala os pacotes encontrados entre `alacritty`, `meld` e `shelly` com `pacman -Rns`. A ação também apaga `~/.config/alacritty`, `~/.cache/alacritty`, `~/.config/meld`, `~/.local/share/meld` e `~/.cache/meld` quando existirem. [Manual do Pacman](https://man.archlinux.org/man/pacman.8.en#REMOVE_OPTIONS_(APPLY_TO_-R))

**Programas → Instalar ArtCraft Launcher** instala `artcraft-launcher-bin` pelo yay após confirmação no Desktop e no Laptop. A CLI prepara o yay se precisar. [Pacote no AUR](https://aur.archlinux.org/packages/artcraft-launcher-bin)

## Programação

**Programação → Instalar Rust** reaproveita o Rustup disponível no `PATH` ou em `~/.cargo/bin` e usa o Pacman quando o executável está ausente. Seleciona a toolchain estável como padrão e verifica `rustc` e `cargo`. A ação roda como usuário normal após confirmação.

**Programação → Git → Configurar Git** instala `git`, `gnupg`, `lazygit`, `git-delta`, `git-lfs`, `neovim`, `openssh` e `git-ignore-bin`. O último pacote usa o yay, que a CLI prepara se estiver ausente. Após confirmação, a ação aplica `~/.gitconfig`, `~/.gitattributes`, `~/.gitignore` e `~/.config/git/config` (ou `$XDG_CONFIG_HOME/git/config`). Os arquivos substituídos recebem backup em `/mnt/backups/dotfiles/Git`.

O `~/.gitconfig` recebe a chave de assinatura em `user.signingkey` e habilita GPG para commits e tags. A CLI preserva a chave configurada se ela puder assinar, inclusive quando foi selecionada uma subchave ou a identidade Git mudou. Sem uma chave configurada utilizável, a CLI prioriza as chaves existentes com o e-mail da identidade Git e oferece as demais chaves utilizáveis se nenhuma corresponder. Se o chaveiro não tiver uma chave de assinatura utilizável, gera uma RSA de 4096 bits com senha e validade de dois anos e exibe a chave pública.

**Programação → Mise → Configurar Mise e instalar ferramentas** reaproveita o executável do Mise quando disponível e instala as dependências de compilação ausentes. Copia `cachyos/mise/mise.toml` para `~/.config/mise/config.toml` (ou `$XDG_CONFIG_HOME/mise/config.toml`), confia nesse arquivo e executa `mise install` como o usuário atual. A lista inclui as CLIs do GitHub (`gh`), do GitLab (`glab`), do Heroku (`heroku`), do Render (`render`), do Sentry (`sentry`) e do Linear (`linear`). Uma configuração anterior recebe backup em `/mnt/backups/dotfiles/Mise` quando substituída.

No Desktop e Laptop, o menu **Programação → Login das CLIs** inclui AWS, Google Cloud, Azure, Railway, GitHub, GitLab, Heroku, Render, Sentry, Linear, Firebase, Cloudflare CLI, Cloudflare Wrangler, Vercel, Neon e npm. Cada ação abre o login da própria ferramenta e usa a versão ativa do Mise, a última versão instalada ou o executável disponível no `PATH`. O Linear usa a CLI `schpet/linear-cli`, já usada no legado, e pede uma chave pessoal para armazenar no chaveiro do sistema.

O Mise também instala Claude Code, Codex, Pi, GitHub Copilot CLI, Gemini CLI e Antigravity CLI. O menu de login tem opções para Claude Code, Codex, Copilot, Gemini e Antigravity. No Gemini, use `/auth` para escolher o método de autenticação e `/quit` para voltar ao menu.

O Mise instala o Maple (`maple`) e o Axiom (`axiom`). Em **Login das CLIs**, o Maple abre a autorização no navegador. O Axiom pede um token de acesso pessoal e o ID da organização no terminal.

O Mise instala a EAS CLI (`eas`) para os serviços do Expo. Em **Login das CLIs → Expo (EAS)**, a ação `eas login` abre a autenticação da conta Expo. [Documentação da EAS CLI](https://docs.expo.dev/eas/cli/)

No perfil Desktop, **Programação → Docker → Configurar Docker** instala `docker`, `docker-compose`, `docker-buildx`, `docker-machine`, `lazydocker`, `util-linux` e `xdg-utils` via Pacman. A ação valida e aplica `/etc/docker/daemon.json` com `data-root` em `/var/lib/docker` e `storage-driver` igual a `overlay2`. Quando substitui uma configuração, guarda uma cópia em `/mnt/backups/dotfiles/Docker/daemon.json.backup-*`. Habilita `docker.service` no boot e inicia o serviço se estiver parado. Um daemon já ativo usa a nova configuração no próximo reinício.

A ação adiciona o usuário ao grupo `docker`, define o Firefox como navegador padrão e inicia o login no Docker Hub quando ainda não há credenciais locais. Abra uma nova sessão para atualizar o grupo nos demais terminais.

**Programação → Docker → Ambiente Sandbox** instala Docker e Distrobox quando necessário e cria o contêiner `sandbox` com a imagem oficial do Arch Linux. A ação reaproveita um Docker acessível e um Distrobox com esse nome. Define Docker na configuração do Distrobox, preserva as outras opções e guarda uma cópia da configuração anterior em `/mnt/backups/dotfiles/Distrobox`. Se `~/.distroboxrc` existir, a ação usa esse arquivo devido à sua precedência. Para entrar, execute `distrobox enter sandbox`. O Distrobox compartilha o diretório pessoal com o contêiner. [Configuração do Distrobox](https://distrobox.it/#configure)

## Agentes de IA

No Desktop e Laptop, **Agentes de IA → Pi Agent → Configurar Pi Agent** prepara o Pi, Bun e Node.js pelo Mise. A ação aplica o `settings.json` derivado do legado em `~/.pi/agent`, ou em `PI_CODING_AGENT_DIR` se estiver definido, e instala ou atualiza os pacotes declarados com `pi update --extensions`. A escolha de tema fica a cargo do usuário. O arquivo substituído recebe backup em `/mnt/backups/dotfiles/Pi`. [Pacotes do Pi](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/packages.md)

## Navegador Web

**Navegador Web → Instalar Firefox** instala `firefox` e `xdg-utils` pelo Pacman e define o Firefox como navegador padrão após confirmação.

## Instalação

O instalador baixa a última release para Linux `amd64`, verifica o SHA-256 e instala o binário em `~/.local/bin`. Prepara `~/.dotfiles` e adiciona os arquivos CachyOS e Ubuntu que estiverem faltando. Também atualiza o modelo do Fish da série `1.7.x` e o modelo do Mise das versões `1.10` a `1.22` quando ainda têm o conteúdo original. O Go é necessário para compilar, e o OpenSSH é necessário ao gerar uma chave.

```sh
curl -fsSL https://raw.githubusercontent.com/ReeseArch64/dotfiles/main/install.sh | sh
~/.local/bin/dotfiles
```

## Desenvolvimento

- [cli/](cli/) contém o módulo Go ativo.
- [cachyos/](cachyos/) e [ubuntu/](ubuntu/) reservam espaço para configurações de cada sistema.
- [legacy/](legacy/) contém a implementação anterior, suas configurações e seus arquivos de referência.

Execute `make test` para testar a CLI e `make cli` para compilar `bin/dotfiles`.
