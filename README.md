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

Ao preparar um backup, a CLI garante que o usuário atual tenha acesso a `/mnt/backups` e aos subdiretórios usados pela ação. Usa `sudo` quando precisa corrigir a propriedade ou criar um diretório sem acesso de escrita no pai. A pasta específica de cada ação fica com permissão `0700`.

## Homelab

As ações são disponibilizadas pelo hostname `homelab` e executadas no Ubuntu por um administrador com `sudo`.

- **Segurança → SSH → Configurar SSH** instala OpenSSH pelo APT e reaproveita `ssh.socket` quando habilitado. Cole a chave pública Ed25519 e teste uma segunda conexão com a chave correspondente e `sudo -v`. Após a segunda confirmação, a ação mantém a chave testada, exige autenticação por chave e bloqueia login como root. Ela valida a configuração antes de recarregar o serviço e guarda os arquivos anteriores em `/mnt/backups/dotfiles/SSH` e `/mnt/backups/dotfiles/SSHD`.
- **Segurança → Firewall → Configurar Firewall** instala e ativa o UFW. Permite todas as portas e protocolos de entrada pela interface da LAN IPv4 privada detectada, preserva as regras existentes e libera o IP e a porta da sessão SSH atual. Aplica a política padrão de negar entrada e permitir saída e restaura a configuração anterior se uma etapa falhar.
- **CasaOS → Instalar CasaOS** baixa o instalador oficial por HTTPS e, após validar o script, executa-o com `sudo`. A ação adiciona o administrador atual ao grupo `docker`. Abra uma nova sessão para que a associação ao grupo tenha efeito. [Instalação oficial do CasaOS](https://wiki.casaos.io/en/get-started)
- **Sistema Operacional → Servidor → Liberar porta 53 (DNS)** desativa o `DNSStubListener` do `systemd-resolved`, mantém a resolução via servidores DNS upstream e recusa drop-ins conflitantes. Se `resolv.conf` apontar para o stub, troca o link para o arquivo upstream. Em falha, restaura os arquivos alterados. [Configuração do systemd-resolved](https://www.freedesktop.org/software/systemd/man/latest/resolved.conf.html)
- **Sistema Operacional → Servidor → Configurar swap** ativa `/swapfile-homelab` com 2 GiB e registra a configuração em `/etc/fstab`. Funciona em ext4 e XFS, mantém outras swaps e reserva pelo menos 4 GiB livres ao criar o arquivo. [Manual de swapon](https://manpages.ubuntu.com/manpages/noble/man8/swapon.8.html)
- **Sistema Operacional → Migrar pastas para /mnt/sdk** move os dados do Mise e cria o link `~/.local/share/mise` → `/mnt/sdk/Mise`. Em conflitos, os arquivos do home prevalecem e os originais recebem backup em `/mnt/backups/dotfiles/SDK`.

As duas ações são implementadas em Go e guardam os arquivos substituídos em `/mnt/backups/dotfiles/DNS` e `/mnt/backups/dotfiles/Swap`.

## Sistema Operacional

**Sistema Operacional → Mouse personalizado** instala o tema FrierenBLZ em `~/.icons` e aplica `cachyos/niri/cfg/misc.kdl`. A ação exige o Niri instalado e que seu `config.kdl` inclua `cfg/misc.kdl`. Valida a configuração candidata com `niri validate`. Arquivos e temas substituídos recebem backups em `/mnt/backups/dotfiles/Niri`.

No Desktop e Laptop, **Sistema Operacional → Migrar pastas para /mnt/sdk** mostra os 23 links e pede confirmação antes de mover dados. Em conflitos, os arquivos do home prevalecem e os originais são guardados em `/mnt/backups/dotfiles/SDK`. Feche os aplicativos que usam as pastas durante a migração. A CLI pede `sudo` ao preparar os diretórios de destino ou corrigir permissões de uma árvore inacessível.

**Sistema Operacional → Ferramentas Essenciais** instala `base-devel`, `curl`, `wget` e `rate-mirrors`.

**Sistema Operacional → Ferramentas de Terminal** instala `bat`, `eza`, `btop`, `fzf`, `ripgrep`, `fd`, `jq`, `yq`, `zoxide`, `ncdu`, `tealdeer`, `wl-clipboard`, `atuin`, `starship`, `yazi`, `sshfs`, `glow` e `viddy`. A opção **Instalar yay** garante `git` e `base-devel` antes de clonar e compilar o pacote do AUR em `~/.aur/yay`, inclusive quando `~/.aur` aponta para `/mnt/sdk/Aur`. O instalador reutiliza o yay se ele já estiver disponível.

**Sistema Operacional → Drivers** instala os headers correspondentes aos kernels instalados, `base-devel` e os utilitários de diagnóstico `vulkan-tools` e `mesa-utils`. A CLI identifica o pacote de cada kernel pelo `vmlinuz` presente em `/usr/lib/modules`.

**Sistema Operacional → Terminal → Instalar Kitty** instala o pacote `kitty` pelo Pacman após confirmação.

**Sistema Operacional → Fish → Configurar Fish** instala `fish`, `mise`, `eza`, `bat`, `util-linux` e `man-db` e aplica `~/.config/fish/config.fish`. Quando os pacotes específicos do CachyOS estão disponíveis, instala também a configuração de notificações e o utilitário de mirrors. A saudação usa o Fastfetch quando ele e sua configuração estiverem disponíveis.

**Sistema Operacional → Fastfetch → Configurar Fastfetch** instala `fastfetch` e `chafa` quando necessário. Usa a foto em `~/.face` ou copia a imagem do repositório se ainda não houver uma. Aplica `~/.config/fastfetch/config.jsonc` para que o comando `fastfetch` encontre a configuração e renderize a foto com Chafa.

As duas ações pedem confirmação antes de copiar a configuração e guardam a versão anterior em `/mnt/backups/dotfiles/Fish` ou `/mnt/backups/dotfiles/Fastfetch` quando o conteúdo muda.

## Programação

**Programação → Git → Configurar Git** instala `git`, `gnupg`, `lazygit`, `git-delta`, `git-lfs`, `neovim`, `openssh` e `git-ignore-bin`. O último pacote usa o yay, que a CLI prepara se estiver ausente. Após confirmação, a ação aplica `~/.gitconfig`, `~/.gitattributes`, `~/.gitignore` e `~/.config/git/config` (ou `$XDG_CONFIG_HOME/git/config`). Os arquivos substituídos recebem backup em `/mnt/backups/dotfiles/Git`.

O `~/.gitconfig` recebe o fingerprint local em `user.signingkey` e habilita GPG para commits e tags. A CLI reaproveita a chave configurada se ela puder assinar pelo e-mail da identidade Git. Se não houver uma chave de assinatura para esse e-mail, gera uma Ed25519 com senha e validade de dois anos e exibe a chave pública.

**Programação → Mise → Configurar Mise e instalar ferramentas** instala o Mise e as dependências de compilação, copia `cachyos/mise/mise.toml` para `~/.config/mise/config.toml` (ou `$XDG_CONFIG_HOME/mise/config.toml`), confia nesse arquivo e executa `mise install` como o usuário atual. A autenticação em cada ferramenta fica a cargo do usuário. Uma configuração anterior recebe backup em `/mnt/backups/dotfiles/Mise` quando substituída.

## Navegador Web

**Navegador Web → Backup Completo** exige o Firefox fechado. Salva as pastas de dados e os perfis registrados em `/mnt/backups/Firefox/firefox-AAAA-MM-DD-HH-MM-SS.tar`. Inclui `profiles.ini`, arquivos de login e chaves do Firefox, cookies, sessões, extensões e demais dados de cada perfil. Caminhos dentro da pasta pessoal são armazenados relativos a ela. Perfis registrados fora dela ficam no tar com seu caminho sem a barra inicial.

O tar é privado e contém dados sensíveis. A senha principal do Firefox, quando configurada, continua necessária para abrir os logins salvos. Sites ainda podem expirar sessões ou exigir nova autenticação. [Dados armazenados nos perfis do Firefox](https://support.mozilla.org/en-US/kb/profiles-where-firefox-stores-user-data)

## Instalação

O instalador baixa a última release para Linux `amd64`, verifica o SHA-256 e instala o binário em `~/.local/bin`. Prepara `~/.dotfiles` e adiciona os arquivos CachyOS que estiverem faltando. Também atualiza o modelo do Fish da série `1.7.x` quando ele ainda tem o conteúdo original. O Go é necessário para compilar, e o OpenSSH é necessário ao gerar uma chave.

```sh
curl -fsSL https://raw.githubusercontent.com/ReeseArch64/dotfiles/main/install.sh | sh
~/.local/bin/dotfiles
```

## Desenvolvimento

- [cli/](cli/) contém o módulo Go ativo.
- [cachyos/](cachyos/) e [ubuntu/](ubuntu/) reservam espaço para configurações de cada sistema.
- [legacy/](legacy/) contém a implementação anterior, suas configurações e seus arquivos de referência.

Execute `make test` para testar a CLI e `make cli` para compilar `bin/dotfiles`.
