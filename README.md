# ReeseArch64 Dotfiles

Configuração pessoal para CachyOS com Niri e Noctalia Shell, e mini-servidor Ubuntu.

O repositório fornece uma CLI interativa para instalar ferramentas, configurar serviços e aplicar arquivos do ambiente. As ações são selecionadas automaticamente para Desktop, Notebook, Homelab ou HomeProd pelo hostname; o MacBook ainda não possui suporte.

## Visão geral

A CLI organiza as ações em cinco áreas:

| Área | Ações |
| --- | --- |
| Sistema | SSH, Firewall, Docker, Drivers e remoção de aplicativos pré-instalados |
| Desenvolvimento | Git, GPG, ambientes de desenvolvimento e sandbox, IDEs e ferramentas de terminal |
| Desktop | Niri, Noctalia, Ghostty, Zen Browser, Thunderbird, Obsidian, Turing Smart Screen, wallpapers e foto de perfil |
| Agentes de IA | Pi Agent e Ollama com GPU AMD |
| Backup | Ponto de restauração Btrfs, navegador, projetos locais e chaves SSH/GPG |

As ações de Pacman e yay instalam somente os pacotes ausentes. Antes de cada instalação, a CLI atualiza as bases com `sudo pacman -Syy`.

A CLI também ignora instaladores de ferramentas já disponíveis e autenticações detectadas. Ela confere o estado imediatamente antes de cada etapa, inclusive após instalar ferramentas na mesma execução. Se uma verificação não confirmar a sessão, o login será solicitado novamente. Credenciais locais do Docker e do Better Stack não comprovam que os tokens ainda são válidos.

Quando o yay falha, a CLI cria `~/.aur`, clona o pacote do AUR e executa `makepkg -si`. Depois, ela confirma a instalação com `pacman -Q`.

Antes dos passos privilegiados, a CLI valida as credenciais do `sudo`. O cache pode evitar prompts, e credenciais expiradas podem exigir nova autenticação.

## Conteúdo

- [Requisitos](#requisitos)
- [Instalação rápida](#instalação-rápida)
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

A CLI identifica o perfil sem perguntar ao usuário. O hostname deve conter exatamente um destes termos, sem diferenciar maiúsculas de minúsculas:

- `desktop`: disponibiliza todas as ações;
- `notebook`: disponibiliza somente o conjunto específico para o portátil;
- `homelab`: disponibiliza CasaOS, DNS, SSH e firewall para Ubuntu;
- `homeprod`: disponibiliza preparação da VPS, CasaOS, SSH, swap e HomeProd Tunnel (etapa 1);
- `macbook`: informa que o dispositivo ainda não possui suporte e encerra.

Hostnames sem um desses termos, ou contendo mais de um deles, são rejeitados.

Nos perfis Desktop e Notebook, a CLI exige este ambiente:

- CachyOS
- Niri
- Noctalia Shell

Ela encerra a execução quando algum desses componentes está ausente.

O perfil `homelab` exige Ubuntu (`ID=ubuntu` em `/etc/os-release`), sem exigir Niri ou Noctalia. Seus pacotes são instalados via APT, não Pacman/yay.

Também são necessários:

- Go compatível com `go.mod`
- Git
- Bash
- curl
- Acesso à internet
- Pacman (Desktop/Notebook) ou APT (Homelab Ubuntu)
- sudo
- systemd

O `yay` é instalado automaticamente, quando necessário, compilando o pacote `yay` do AUR (não `yay-bin`) em `~/.aur/yay` com `makepkg -si`. A CLI instala `git` e `base-devel` via Pacman antes da compilação.

Algumas ações possuem requisitos próprios. Eles estão descritos nas seções correspondentes.

CasaOS, Pi Agent, Ollama, LunarVim e Better Stack CLI executam scripts remotos baixados com `curl`. Revise as origens antes de iniciar essas ações.

## Instalação rápida

Clone o repositório no diretório padrão:

```bash
git clone https://github.com/ReeseArch64/dotfiles.git ~/.dotfiles
cd ~/.dotfiles
```

As configurações são cópias independentes em `setup/desktop/` e `setup/laptop/`, selecionadas pelo hostname (`desktop` e `notebook`, respectivamente). Não há base compartilhada nem sobreposição. Discord, Obsidian, Ollama e Turing Smart Screen são exclusivos de `setup/desktop/` e não estão presentes no laptop. Nos caminhos abaixo, `setup/<perfil>/` representa a pasta selecionada.

Os diretórios `setup/homelab/` (servidor Ubuntu), `setup/homeprod/` (Ubuntu) e `setup/macbook/` (macOS) ficam separados e não herdam configurações dos perfis CachyOS. O hostname `homelab` usa `setup/homelab/`; um hostname contendo `homeprod` usa `setup/homeprod/`. O MacBook ainda não tem ações suportadas. Hostnames desconhecidos ou com mais de um identificador são recusados antes de abrir o menu; nenhuma opção fica disponível.

A identidade do Git fica definida diretamente em `setup/<perfil>/git/.gitconfig`.

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

Somente nos perfis Desktop e Notebook, antes de abrir a interface, após validar o perfil e o sistema operacional, a CLI garante que existam `/mnt/workspaces`, `/mnt/sdk`, `/mnt/storage` e `/mnt/backups`, com proprietário e grupo do usuário atual. Usa `0700` para workspaces/backups e `0755` para sdk/storage (permitindo acesso de serviços aos seus próprios subdiretórios). Solicita `sudo` somente se houver pastas ausentes ou com permissões/proprietário/grupo diferentes. Links simbólicos nesses caminhos são rejeitados. Não altera permissões ou donos recursivamente, nem formata ou monta discos.

HomeLab e HomeProd não executam essa preparação automática, as migrações de diretórios nem a criação dos links simbólicos de SDK, pastas pessoais e lixeira descritos abaixo. Essa exclusão não desfaz migrações realizadas anteriormente.

Na mesma preparação inicial, a CLI migra os dados e cria estes links no home:

| Link no home | Destino |
| --- | --- |
| `~/.aur` | `/mnt/sdk/Aur` |
| `~/.local` | `/mnt/sdk/Local` |
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

Também são migradas as pastas pessoais para `/mnt/storage`. Os caminhos no home são links simbólicos apontando para os destinos, assim como todos os diretórios SDK acima:

| Link no home | Destino |
| --- | --- |
| `~/Documents` | `/mnt/storage/Documents` |
| `~/Downloads` | `/mnt/storage/Downloads` |
| `~/Pictures` | `/mnt/storage/Pictures` |
| `~/Videos` | `/mnt/storage/Videos` |
| `~/Music` | `/mnt/storage/Music` |

Se já houver dados no destino, eles são mesclados: os arquivos do home prevalecem, mas conflitos são preservados em backup. Os diretórios originais do home e os destinos anteriores são mantidos integralmente em backups dentro de `/mnt/backups`, antes da criação dos links. Arquivos exclusivos do destino são mantidos na mesclagem. Links que já apontem para o destino correto não são migrados novamente; links para outros destinos são rejeitados, sem redirecionamento silencioso.

Se uma pasta a migrar não puder ser lida por falta de permissão, a CLI solicita `sudo` para corrigir seu proprietário/grupo para o usuário atual (`chown -R -P --no-dereference`) e adicionar acesso ao dono (`chmod -R u+rwX`), antes de tentar novamente. Isso se limita à árvore afetada, no home ou no destino; não segue links simbólicos externos, não concede acesso a outros usuários e não apaga dados. As permissões corrigidas também ficam nos backups, mesmo se a migração falhar depois.

**Feche os aplicativos antes da primeira migração**, especialmente os que usam `~/.local` e `~/.cache`. A cópia preserva permissões, datas e links simbólicos, inclusive entre discos diferentes. Reserve espaço para os dados copiados e os backups. Arquivos especiais (por exemplo, sockets ativos) e caminhos de origem/destino inseguros interrompem a preparação sem ativar os links do grupo afetado (SDK ou storage). Os backups são preservados para conferência; a CLI não os remove automaticamente.

Após preparar os links SDK, a CLI prepara a lixeira principal: `$XDG_DATA_HOME/Trash` passa a ser um link absoluto para `~/.trash`, um diretório real no mesmo sistema de arquivos do home. Quando `XDG_DATA_HOME` estiver ausente, vazio ou relativo, usa `~/.local/share/Trash`. Isso mantém a lixeira do home funcionando mesmo com `~/.local` em outro disco. Não modifica `XDG_DATA_HOME` nem as lixeiras próprias de outros volumes, como `/mnt/storage/.Trash-1000`.

Na primeira execução, feche também os gerenciadores de arquivos e demais aplicativos que usam a lixeira. Os itens existentes e seus metadados de restauração são copiados para `~/.trash`, preservando os originais em `/mnt/backups` pelo mesmo mecanismo das outras migrações. `~/.trash`, `files` e `info` recebem permissão `0700`. Se as duas lixeiras já tiverem conteúdo, os itens sem conflitos são reunidos; nomes conflitantes, inclusive entre arquivos e seus metadados, interrompem a preparação antes de substituir os originais. Resolva esses conflitos antes de executar novamente. Links inesperados, diretórios de outro proprietário ou `~/.trash` em outro sistema de arquivos são rejeitados. Falhas na ativação acionam a recuperação dos originais; execuções seguintes reutilizam o link correto.

Para compilar e executar sem instalar o comando:

```bash
make run
```

### Ações do Notebook

O Notebook recebe:

- Sistema: SSH, Firewall, Docker e remoção de aplicativos pré-instalados;
- Desenvolvimento: Git, GPG, Ambiente de Desenvolvimento, Linear CLI, Android CLI e ferramentas de terminal;
- Desktop: Niri, Noctalia, Ghostty, Zen Browser, wallpapers e foto de perfil;
- Agentes de IA: somente Pi Agent;
- Backup: todas as ações disponíveis.

As demais ações ficam exclusivas do Desktop.

### Ações do Homelab (Ubuntu)

O perfil oferece instalação do CasaOS e configuração de DNS, SSH, firewall e swap, sem ações de desktop.

#### Ferramentas de administração (sem GUI)

A ação `Ferramentas de Administração` instala LazyDocker, jq, ripgrep (`rg`), fd, fzf, bat, Git e ncdu no Ubuntu Server. Usa APT com os nomes Ubuntu (`fd-find` e `bat`) e disponibiliza `fd` e `bat` por links em `/usr/local/bin` para `fdfind` e `batcat` quando esses comandos ainda não existem. Não substitui comandos existentes.

O LazyDocker 0.25.2 é baixado da release oficial, com SHA-256 fixado para amd64 ou arm64. Outras arquiteturas e sistemas diferentes de Ubuntu são recusados antes da instalação. Não instala GUI, não reinstala Docker e não altera o daemon ou o socket do CasaOS. Para usá-lo sem sudo, o usuário precisa ter acesso ao Docker (grupo `docker`, equivalente a privilégios administrativos); após alterar grupos, abra uma nova sessão.

Também é possível executar `sudo bash setup/homelab/tools/install.sh`. A instalação requer internet e repositórios APT habilitados para os pacotes; não habilita repositórios automaticamente. Reexecutar reaplica a instalação e baixa novamente a versão fixada do LazyDocker. Esse conjunto não altera o perfil Desktop nem o HomeProd.

#### Swap de 2 GiB

A ação `Configurar Swap` cria `/swapfile-homelab` com 2 GiB, permissão `0600`, ativa a swap e registra sua ativação no `/etc/fstab` com backup prévio. Usa sudo no menu; para execução direta, use `sudo bash setup/homelab/swap/configure.sh`. Requer ext4 ou XFS e mantém pelo menos 4 GiB livres após a alocação. Preserva outras swaps, reutiliza o arquivo compatível sem duplicar a entrada no fstab e recusa arquivos incompatíveis em vez de sobrescrevê-los. Não altera `swappiness` nem configura hibernação. Arquivos temporários são preservados e informados em caso de falha na criação. O HomeProd continua com sua configuração independente de 8 GiB.

#### SSH por chave no HomeLab

Execute o menu como seu usuário administrador (por exemplo, `alsgy`). A ação instala e ativa OpenSSH e solicita que você cole o conteúdo de `id_pessoal_desktop.pub` em uma única linha no terminal; não exige arquivo de chave no HomeLab nem no repositório. Valida a pública Ed25519, mostra seu fingerprint, cria `authorized_keys` se necessário e adiciona a chave preservando as existentes durante o teste. Teste uma segunda conexão usando essa chave e `sudo -v`. Somente depois de digitar `CONFIRMO`, mantém apenas a chave colada e aplica uma política permitindo SSH apenas para esse usuário com chave pública, bloqueando senha, autenticação interativa e root. Valida a política com `sshd -t` e `sshd -T`, guarda backup em `/etc/ssh/homelab-backup.*` e recarrega o serviço. Em caso de falha na aplicação, restaura a configuração e as chaves disponíveis durante o teste. Mantenha a sessão atual aberta até testar novamente. A chave privada permanece no desktop.

O instalador do CasaOS executa o comando solicitado, com detecção de falhas no download:

```bash
curl -fsSL https://get.casaos.io | sudo bash
```

A configuração DNS altera `DNSStubListener=no` na seção `[Resolve]` de `/etc/systemd/resolved.conf` e reinicia `systemd-resolved` para liberar a porta 53. Se `/etc/resolv.conf` apontar para o stub do systemd, o link é ajustado para `/run/systemd/resolve/resolv.conf`, mantendo a resolução de nomes sem o listener local. Drop-ins que habilitem o stub são rejeitados antes de qualquer alteração; ajuste esses arquivos antes de repetir a ação.

SSH usa os pacotes e unidades do Ubuntu. No Homelab, antes de ser ativado, o firewall UFW libera todas as portas e protocolos de entrada para a sub-rede LAN IPv4 privada detectada. A regra é adicionada no início para prevalecer sobre bloqueios anteriores. Todos os dispositivos dessa sub-rede passam a poder acessar serviços escutando na LAN: use apenas em uma rede confiável. A política padrão continua sendo negar entrada e permitir saída; a conexão SSH administrativa atual recebe uma exceção para preservar o acesso. Regras existentes não são apagadas, inclusive eventuais liberações externas anteriores. A ação não abre portas no roteador, não configura encaminhamento para containers e não expõe serviços vinculados somente a `127.0.0.1`. Esta política é exclusiva do Homelab, não do HomeProd.


### Ações do HomeProd (Ubuntu)

#### HomeProd Tunnel — etapa 1 (sem fechamento de portas)

A ação `HomeProd Tunnel (etapa 1)` substitui o antigo túnel exclusivo do Vaultwarden. Ela usa `setup/homeprod/tunnel/configure.sh` com sudo, instala o pacote oficial `cloudflared` no Ubuntu quando ausente e configura uma unidade systemd independente do CasaOS. Não instale os apps Cloudflared/Cloudflared Web do marketplace para este fluxo. A CLI `cloudflare`/`cf` não substitui o conector `cloudflared` e não é necessária aqui.

**A VPS ainda permanece exposta pelo IP nesta etapa.** O script não altera firewall, SSH, Samba, Docker ou Vaultwarden. O hostname `homeprod.reesearch64.com` encaminha apenas o painel CasaOS em `http://127.0.0.1:80`, não todas as portas da VPS. Outros aplicativos precisarão de rotas próprias. Não remova o acesso direto antes de testar Tunnel e Access.

Antes de executar, configure no painel Cloudflare Zero Trust, nesta ordem:

1. Crie uma aplicação Access self-hosted cobrindo todo o hostname `homeprod.reesearch64.com` (não apenas uma URL de login). Permita somente sua identidade, usando provedor de identidade com MFA. Não use políticas `Bypass` nem `Allow Everyone`. MFA da conta administrativa Cloudflare não protege automaticamente quem acessa a aplicação.
2. Crie um Tunnel gerenciado remotamente, dedicado ao HomeProd. Não execute o comando de instalação com token oferecido pelo painel; o script gerencia seu próprio serviço e pede apenas o token.
3. Configure a rota publicada `homeprod.reesearch64.com` para HTTP `127.0.0.1:80`, com regra final sem hostname `http_status:404`. O DNS desse subdomínio deve apontar para o Tunnel; não reutilize os hostnames `vault` ou `homelab`. Não adicione rotas SSH/Samba.
4. Com o CasaOS respondendo na porta 80 local, execute a opção do menu ou diretamente:

   ```bash
   sudo bash setup/homeprod/tunnel/configure.sh
   ```

5. Confirme a preparação de Access/DNS e cole **somente o token**, em entrada oculta. Não grave tokens no Git, vault, linha de comando ou histórico. O script não cria nem audita políticas na sua conta Cloudflare; a confirmação manual não é uma auditoria de segurança.

Requer systemd com `LoadCredential` (247+, disponível no Ubuntu suportado), `curl`, `flock` e acesso aos repositórios APT. A instalação usa o [repositório oficial Cloudflare](https://pkg.cloudflare.com/), com chave dedicada e `signed-by`. Um binário existente em `/usr/bin/cloudflared` precisa suportar `--token-file` (2025.4.0+); versões antigas exigem atualização antes de repetir. Uma unidade `cloudflared.service` existente ou um conector em execução no Docker interrompem a configuração para evitar duplicação.

A unidade `cloudflared-homeprod.service` inicia no boot, usa usuário dinâmico sem privilégios e recebe a credencial por `LoadCredential`. O token fica em `/etc/homeprod-cloudflared/tunnel-token` (`0600`, diretório `0700`, root), nunca no `ExecStart`. Backups privados ficam em `/etc/homeprod-cloudflared/backup.*`; trate-os também como segredos. O endpoint de saúde fica apenas em `127.0.0.1:49312`. O script aguarda `/ready` responder com sucesso; isso comprova conexão do conector, **não** a configuração correta de DNS/Access ou a segurança da origem.

Em falha após alterar unidade/token, o script tenta restaurar os arquivos e o estado anterior do serviço. Pacotes e repositório APT instalados permanecem. A repetição atualiza a unidade/credencial com novo backup. Para parar somente este conector: `sudo systemctl disable --now cloudflared-homeprod.service`; isso não remove a aplicação Access, o Tunnel ou o DNS na Cloudflare. Recursos legados de Vaultwarden/Caddy/Tunnel já existentes na VPS não são migrados nem apagados automaticamente.

Validação manual obrigatória, usando outra rede e navegador privado:

- Sem sessão Access, `https://homeprod.reesearch64.com` deve exigir autenticação antes de exibir qualquer conteúdo do CasaOS; uma identidade não autorizada deve ser recusada.
- Após autenticar, testar login, navegação e terminal/WebSocket do CasaOS.
- Repetir após `sudo systemctl restart cloudflared-homeprod.service`; posteriormente testar persistência após reboot, com console de recuperação disponível.
- Diagnóstico: `sudo systemctl status cloudflared-homeprod.service` e `sudo journalctl -u cloudflared-homeprod.service -n 50 --no-pager`. Revise logs antes de compartilhar; nunca compartilhe token ou backups.

#### Etapa 2 pendente — fechar exposição direta, preservando SSH

Somente depois de validar a etapa 1, implementar/aplicar o bloqueio das entradas públicas. O inventário inicial mostrou SSH TCP 22 (IPv4/IPv6), CasaOS TCP 80, Samba TCP 139/445 e UDP 137/138, UFW inativo e nenhum container em execução. Refazer o inventário antes da etapa 2.

O bloqueio deverá preservar SSH sem alterar porta/autenticação, tráfego de resposta e protocolos essenciais de rede, cobrir IPv4/IPv6 e encaminhamento/portas Docker (UFW sozinho não basta). Manter sessão SSH e console do provedor disponíveis, com rollback programado antes de aplicar. Não restringir saídas necessárias ao Tunnel. Testar uma nova conexão SSH, tentativas externas ao IP/hostname antigo e uma porta Docker publicada; repetir após reboot. Um firewall do provedor pode acrescentar uma barreira independente.

**O menu ainda não aplica essa etapa.** Até sua implementação e validação externa, HTTP e Samba podem continuar acessíveis por `209.50.240.34` e `vps12431.integrator.host`. Depois dela, somente o SSH deverá continuar público (além dos protocolos essenciais de rede); o IP conhecido não se torna secreto.

#### Preparação da VPS, swap e SSH

`Configurar Swap` executa como root a criação de `/swapfile-homeprod` com 8 GiB, permissão `0600`, ativa a swap e registra sua ativação no `/etc/fstab` com backup prévio. Requer ext4 ou XFS e espaço para manter pelo menos 4 GiB livres após a alocação. A ação preserva outras swaps e reutiliza seu arquivo se ele já tiver o tamanho e formato esperados; não redimensiona nem sobrescreve arquivos incompatíveis. Para optar por 12 GiB na primeira criação, execute `bash setup/homeprod/swap/configure.sh 12`. Não altera `swappiness` nem configura hibernação. Se houver falha durante a criação, o arquivo temporário é preservado e seu caminho é informado.

O hostname da VPS deve conter `homeprod`. A preparação inicial deve ser executada na sessão temporária de `root`; esse perfil não prepara `/mnt`, SDKs, storage nem a lixeira do root.

A primeira ação executa, nesta ordem, `apt-get update`, `apt-get upgrade --yes` e a instalação do pacote `sudo`. Em seguida cria interativamente o usuário `alsgy` com shell Bash e o adiciona ao grupo `sudo`. A senha é lida diretamente pelo `adduser`, não aparece em argumentos e não é armazenada pelo dotfiles. Se `alsgy` já existir, a execução preserva sua senha e apenas garante sua associação ao grupo administrativo.

Após preparar o administrador, o bootstrap instala `curl` e `ca-certificates` e chama `setup/homeprod/casaos/install.sh`. A ação independente `Instalar CasaOS` permite repetir somente essa instalação; como root, também pode executar `bash setup/homeprod/casaos/install.sh` diretamente.

O wrapper baixa o instalador oficial de `https://get.casaos.io` antes de executá-lo e verifica sua sintaxe. Para contornar o HTTP 500 de `/uninstall/v0.4.16`, baixa antecipadamente o desinstalador e, se o endpoint falhar, usa `uninstall.sh` do repositório oficial `IceWhaleTech/get`, fixado na revisão `15d5563aef5e3628f2b2ac65d619d81228ab2f3e` e validado por SHA-256. Esse arquivo é apenas armazenado, nunca executado pelo wrapper. A declaração exata de URL do instalador é substituída por uma referência ao arquivo local validado; se a declaração oficial mudar, o wrapper interrompe para revisão. Não se presume equivalência de versões entre o endpoint e a alternativa do repositório.

O instalador continua executando suas demais etapas, incluindo Rclone e inicialização/verificação dos serviços. Seus arquivos e o log de execução ficam no diretório `/tmp/dotfiles-casaos.*` informado na tela (podem desaparecer após reiniciar). Falhas indicam a etapa e preservam o código de saída. Quando o terminfo do terminal atual não estiver disponível, a execução usa `TERM=xterm`. A instalação pode reiniciar Docker e outros serviços, assim como o instalador oficial; não é apenas uma atualização de pacotes ou criação de usuário.

Mantenha a sessão root aberta até instalar a chave pública do desktop e confirmar, em outro terminal, o acesso SSH e o funcionamento de `sudo` como `alsgy`.

Depois do bootstrap, a ação independente `Configurar SSH` do HomeProd deve ser executada como root. Cole o conteúdo de `id_pessoal_desktop.pub` diretamente no prompt do terminal, em uma única linha; nenhum arquivo de chave precisa ser colocado no repositório. Também é possível executar `bash setup/homeprod/ssh/configure.sh`. A ação aceita uma única pública Ed25519, valida seu formato, mostra seu fingerprint e adiciona a chave ao usuário `alsgy` para teste. A chave privada permanece no desktop. Em outro terminal, teste o acesso com a chave privada correspondente, `PasswordAuthentication=no` e `IdentityAgent=none`, seguido de `sudo -v`.

Somente após digitar `CONFIRMO`, atestando o teste e a disponibilidade de recuperação, a ação restringe o SSH a `alsgy`, exige chave pública e proíbe login SSH de root e autenticação por senha. A única fonte de chaves passa a ser `/etc/ssh/homeprod_authorized_keys/alsgy`, gerenciada por root. A política é validada com `sshd -t` e `sshd -T` antes de recarregar o serviço. O backup fica em `/etc/ssh/homeprod-backup.*`; falhas na aplicação restauram a configuração anterior. Teste uma nova conexão antes de encerrar a sessão administrativa. Console/rescue do provedor e backup da chave precisam ser providenciados separadamente. Essa ação não configura o firewall.

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
- `shelly`

A ação exige que o pacote `ghostty` esteja instalado antes de remover o Alacritty. Configure o Ghostty no menu `Desktop` primeiro.

A CLI atualiza as bases e executa `pacman -Rns` somente com os pacotes da lista que ainda estão instalados. Confirme a lista apresentada pelo Pacman antes de continuar.

Após a desinstalação, a ação remove estes diretórios do usuário atual:

```text
~/.mozilla
~/.cache/mozilla
~/.config/alacritty
~/.cache/alacritty
~/.config/meld
~/.local/share/meld
~/.cache/meld
```

Esse conteúdo não recebe backup. Preserve manualmente qualquer dado necessário antes de executar a ação.

### Docker

`Sistema > Docker > Configurar Docker` executa estas etapas:

1. Valida o executável `zen-browser` e o diretório `~/.config/zen`.
2. Instala Docker, Compose, Buildx e Lazydocker.
3. Instala `util-linux` e `xdg-utils`, necessários ao fluxo.
4. Cria `/etc/docker` com `sudo` e grava `/etc/docker/daemon.json` com o conteúdo abaixo, substituindo qualquer configuração existente.
5. Adiciona o usuário atual ao grupo `docker`.
6. Habilita e inicia `docker.service`.
7. Define o Zen Browser como navegador padrão.
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

As identidades são separadas pelo dispositivo de origem:

| Perfil configurado | Chaves próprias | Chaves públicas recebidas |
| --- | --- | --- |
| Desktop | `id_trabalho_desktop`, `id_pessoal_desktop` e respectivos `.pub` | nenhuma |
| Notebook | `id_trabalho_notebook` e seu `.pub` | `id_trabalho_desktop.pub` |
| Homelab | nenhuma chave privada | `id_pessoal_desktop.pub` |

Desktop e notebook também exigem os pares compartilhados `id_github` e `id_gitlab`, sem sufixo de dispositivo. Não existe uma identidade `id_pessoal_notebook`.

A ação copia `setup/<perfil>/ssh/config` para `~/.ssh/config`, cria `~/.ssh/known_hosts` quando necessário e aplica permissões `0700` ao diretório e `0600` aos arquivos. No desktop, notebook e homelab usam `StrictHostKeyChecking accept-new`: a primeira conexão registra automaticamente a chave do host e qualquer mudança posterior continua sendo recusada. Os demais destinos mantêm `StrictHostKeyChecking yes`.

No desktop e no notebook, a ação `Configurar SSH` executa `systemctl --user enable --now ssh-agent.socket`, sem sudo: inicia o socket imediatamente e habilita sua ativação nas próximas sessões do usuário. A ativação não depende do autostart do Niri. Nos dois perfis, `IdentityAgent /run/user/%i/ssh-agent.socket` aponta explicitamente para esse agente, compartilhado entre os terminais. Cada chave privada criptografada solicita sua passphrase no primeiro uso; `AddKeysToAgent yes` mantém a chave desbloqueada somente em memória enquanto o agente estiver ativo. Reiniciar o agente ou o computador exige desbloquear novamente. O dotfiles não remove nem armazena a passphrase. Isso não memoriza senhas de login dos servidores nem senhas de `sudo`.

O fluxo entre dispositivos é estritamente unidirecional: desktop → notebook e desktop → homelab. O desktop não aceita nenhuma chave, mantém `authorized_keys` vazio, bloqueia a porta SSH no UFW e desabilita `sshd.service` e `sshd.socket`. O notebook aceita somente `id_trabalho_desktop.pub`; o homelab aceita somente `id_pessoal_desktop.pub`. Assim, a identidade pessoal não concede acesso ao equipamento de trabalho. A configuração do notebook não contém destinos LAN para retornar ao desktop ou acessar o homelab.

A ação substitui `authorized_keys` pelo conjunto exato do perfil, aplica permissão `0600` e preserva a versão anterior em `/mnt/backups`. A autenticação por senha permanece disponível como recuperação no notebook e no homelab; clientes com a chave privada correspondente entram diretamente por chave pública.

Uma configuração diferente é preservada em `/mnt/backups`, identificada pela origem e pela data, sem criar arquivos de backup dentro de `~/.ssh`.

### Firewall

O menu `Sistema > Firewall` oferece somente a ação `Configurar Firewall`.

A ação instala `ufw` e `iptables-nft`, bloqueia entradas e permite saídas por padrão. Ela remove a liberação global do SSH, libera a porta somente para a rede local e ativa o UFW permanentemente.

## Desenvolvimento

### Git

`Desenvolvimento > Git > Configurar Git`:

1. Instala `git` e `github-cli` via Pacman.
2. Instala `lazygit` e `glab` via yay.
3. Aplica os arquivos globais.
4. Exige que as chaves e o arquivo `~/.ssh/config` já estejam configurados.
5. Executa `gh auth login --git-protocol ssh` quando o GitHub não está autenticado.
6. Executa `glab auth login --git-protocol ssh` quando o GitLab não está autenticado.

| Destino | Origem | Formato |
| --- | --- | --- |
| `~/.gitconfig` | `setup/<perfil>/git/.gitconfig` | Cópia regular |
| `~/.gitattributes` | `setup/<perfil>/git/.gitattributes` | Link simbólico |
| `~/.gitignore` | `setup/<perfil>/git/.gitignore` | Link simbólico |
| `~/.config/git/config` | `setup/<perfil>/git/config` | Link simbólico |

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

1. Instala o `mise` via yay.
2. Cria o link `~/.config/mise/config.toml` para `setup/<perfil>/mise/mise.toml`.
3. Marca a configuração do Mise como confiável.
4. Instala todas as ferramentas declaradas no `mise.toml`.
5. Autentica AWS, Google Cloud, Railway, Firebase e Cloudflare. Tenta autenticar no Azure, mas continua se o login falhar.
6. Instala Fish, `rustup` e o toolkit `tk` para Python via Pacman.
7. Copia `setup/<perfil>/fish/config.fish` para `~/.config/fish/config.fish`.
8. Instala e seleciona a toolchain Rust estável.
9. Valida `rustc --version` e `cargo --version`.
10. Instala Better Stack CLI em `~/.local/bin/bs` pelo script oficial.
11. Executa `bs auth init`.
12. Executa `npm login`.

O arquivo `setup/<perfil>/mise/mise.toml` instala:

- AWS CLI
- Google Cloud CLI
- Railway CLI
- Firebase CLI
- Azure CLI
- Cloudflare CLI (`cf`, pacote npm `cf`)
- Cloudflare Wrangler
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

A cópia substitui o `config.fish` atual. Ela remove o antigo arquivo `conf.d/dotfiles-mise.fish` para evitar ativação duplicada. Também remove `~/.bashrc`, `~/.bash_logout`, `~/.bash_profile` e `~/.zshrc`.

Abra um novo terminal para carregar a configuração do Fish e a ativação do Mise.

### Ambiente Sandbox

`Desenvolvimento > Ambiente Sandbox` executa estas etapas:

1. Instala `podman`, `podman-compose` e `distrobox` via Pacman.
2. Instala `podman-tui-bin` via yay/AUR.
3. Define `container_manager="podman"` em `~/.config/distrobox/distrobox.conf`.
4. Preserva as outras opções existentes no arquivo do Distrobox.
5. Executa `distrobox create --name sandbox --image archlinux`.

Entre no ambiente depois da criação:

```bash
distrobox enter sandbox
```

Os pacotes instalados dentro do contêiner não alteram a base do CachyOS. O Distrobox integra o diretório pessoal por padrão e não funciona como limite de segurança.

### IDEs

`Desenvolvimento > Instalar IDEs` instala:

- `visual-studio-code-bin` via yay/AUR
- `android-studio` via yay/AUR
- `zed` via yay

Após instalar o Android Studio, use o SDK Manager para instalar o Android SDK. A instalação da IDE não configura automaticamente `ANDROID_HOME`.

### Android CLI

`Desenvolvimento > Android CLI` executa o instalador:

```bash
curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash
```

O comando é executado com `bash -o pipefail` para detectar também falhas no download.

### Ferramentas de terminal

`Desenvolvimento > Ferramentas de terminal` instala os seguintes grupos.

**yay:**

- Neovim
- scc
- viddy, compilado pelo pacote AUR
- mprocs, fornecido pelo modo de compatibilidade do Dekit e instalado pelo script oficial
- Posting, instalado pelo `uv` com Python 3.13
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
- git-delta
- Ventoy
- eza
- bat
- wget
- curl
- uv

Quando `lvim` está ausente, a CLI instala LunarVim com a branch `release-1.4/neovim-0.9`.

A ação copia `setup/<perfil>/fastfetch/config.json` para `~/.config/fastfetch/config.json` sem alterar outros arquivos do diretório.

Ela também copia `setup/<perfil>/btop/` para `~/.config/btop`. Uma configuração diferente recebe backup com timestamp.

## Desktop

### Login automático

Em desktop e notebook, execute `Desktop > Login automático` para entrar direto no Niri ao ligar, sem pedir senha no SDDM. A ação usa o usuário que executou a CLI (não execute com `sudo`), preserva as outras configurações e não reinicia a sessão atual. O efeito começa na próxima inicialização.

A configuração fica em `/etc/sddm.conf`; o arquivo anterior, quando existente, é salvo uma única vez em `/etc/sddm.conf.before-autologin`. Para desfazer, restaure esse backup ou remova a seção `[Autologin]` criada pela ação. A senha da conta, o sudo, o bloqueio de tela e uma eventual senha de criptografia do disco não são alterados. Qualquer pessoa com acesso físico poderá entrar na sessão ao ligar o computador.

### Niri

`Desktop > Niri` copia integralmente `setup/<perfil>/niri/` para `~/.config/niri`. O cursor de `setup/<perfil>/cursor/FrierenBLZ/` é copiado para `~/.icons/FrierenBLZ`.

Cada equipamento tem sua configuração completa e independente. O `cfg/display.kdl` do laptop contém somente `eDP-1`; os demais monitores ficam na cópia do desktop.

A inicialização da Turing Smart Screen fica somente em `setup/desktop/niri/cfg/autostart.kdl`. O laptop não inicia esse serviço.

Quando o cursor de destino é diferente, a CLI preserva a versão anterior em `/mnt/backups`.

### Noctalia

`Desktop > Noctalia` exige estes plugins:

- `containers`
- `github-kanban`
- `noctaproton-vpn`
- `zed-provider`

A CLI procura os plugins neste diretório:

```text
~/.local/state/noctalia/plugins/materialized/community
```

A CLI não baixa plugins. Após a validação, ela copia `settings.toml` e `state.toml` para o estado do Noctalia.

### Ghostty

`Desktop > Ghostty` instala `ghostty` via yay e copia `setup/<perfil>/ghostty/` para `~/.config/ghostty`.

### Zen Browser

Na máquina original, crie o backup com o Zen Browser fechado:

```bash
tar -cvf zen-backup.tar -C ~ .config/zen .cache/zen .local/share/keyrings
mv zen-backup.tar ~/.dotfiles/
```

`Desktop > Zen Browser` executa estas etapas:

1. Exige o arquivo regular `~/.dotfiles/zen-backup.tar`.
2. Instala `zen-browser-bin` via yay/AUR.
3. Aceita somente `.config/zen`, `.cache/zen` e `.local/share/keyrings` no arquivo.
4. Descompacta o conteúdo em `~/.dotfiles/zen-backup`.
5. Substitui os três diretórios correspondentes no diretório pessoal.
6. Remove de `zen-backup` os diretórios movidos para seus destinos.

A restauração substitui os dados atuais sem manter backup. Feche o Zen Browser e preserve manualmente qualquer perfil ou chave necessária.

O `.gitignore` exclui `zen-backup.tar` e `zen-backup/`. O arquivo contém dados pessoais e credenciais que nunca devem ser versionados.

### Thunderbird

`Desktop > Thunderbird` instala o pacote `thunderbird` via Pacman quando ele ainda não está instalado.

### Obsidian

`Desktop > Obsidian` instala `obsidian-bin` via yay/AUR e copia `setup/desktop/obsidian/` para `~/.obsidian`.

### Turing Smart Screen (somente Desktop)

`Desktop > Turing Smart Screen` instala `turing-smart-screen-python` com Python 3.13, suporte à GPU AMD (Radeon RX 7600) e tema Frieren. Requer o `mise` instalado; configure primeiro `Desenvolvimento > Ambiente de Desenvolvimento`.

A ação executa etapas separadas:

1. Instala o pacote AUR com `env -u PYTHONHOME -u PYTHONPATH PATH=/usr/bin:/bin /usr/bin/yay -S turing-smart-screen-python`, evitando interferência do Python do mise. Instala o `yay` a partir do código-fonte se necessário.
2. Executa `mise install python@3.13`, sem alterar o Python global ou do sistema.
3. Cria `~/.local/share/turing-screen-venv` com `mise exec python@3.13 -- python -m venv`.
4. Em outro comando, instala `/opt/turing-smart-screen-python/requirements.txt` usando o Python desse venv.
5. Adiciona o usuário atual ao grupo `turing-smart-screen` criado pelo pacote e recarrega as regras com `sudo udevadm control --reload-rules`.
6. Instala `base-devel` e `libdrm` via Pacman; instala `setuptools`, `wheel` e `Cython` no venv.
7. Compila `pyamdgpuinfo` no venv com `pip install --no-build-isolation --no-binary=pyamdgpuinfo pyamdgpuinfo`. Os requisitos dessa versão não instalam essa biblioteca automaticamente no Python 3.13.
8. Copia `setup/desktop/turing-smart-screen-python/themes/frieren/` para `/opt/turing-smart-screen-python/res/themes/frieren/` e substitui `/opt/turing-smart-screen-python/config.yaml` pelo arquivo do repositório, usando `sudo`.

Após concluir, desconecte e reconecte a tela USB, saia da sessão gráfica e entre novamente. Confira o grupo com `id` e, para a tela em `/dev/ttyACM0`, com `/usr/bin/ls -l /dev/ttyACM0`: o dispositivo deve pertencer ao grupo `turing-smart-screen`.

Para iniciar após renovar a sessão:

```bash
env PATH="$HOME/.local/share/turing-screen-venv/bin:$PATH" /usr/bin/turing-smart-screen main
```

Para executar temporariamente com o grupo atualizado, sem sair da sessão, use `sg`:

```bash
sg turing-smart-screen -c 'env PATH="$HOME/.local/share/turing-screen-venv/bin:$PATH" /usr/bin/turing-smart-screen main'
```

A instalação não inicia o programa automaticamente. A configuração do Niri já contém uma entrada de autostart para esse venv.

### Wallpapers

`Desktop > Wallpapers` copia `assets/wallpapers/` para `~/.wallpapers`.

A pasta contém dez imagens destinadas a diferentes telas:

- `android.jpg`
- `browser.jpg`
- `desktop.jpg`
- `desktop-ultrawide.jpg`
- `dual-monitor.jpg`
- `ipad.jpg`
- `iphone.jpg`
- `macbook.jpg`
- `notebook.jpg`
- `tablet.jpg`

### Foto de perfil

`Desktop > Foto de perfil` cria `~/.face` como link simbólico para `assets/face.jpg`.

A ação substitui o destino existente. Faça uma cópia manual quando quiser preservar a foto atual.

### Backups de diretórios

Niri, o cursor FrierenBLZ, Ghostty, Obsidian, wallpapers e btop usam cópias regulares.

Quando o destino é diferente, a CLI preserva a versão anterior em `/mnt/backups`, com identificação da origem e da data e sem sobrescrever backups anteriores. Conteúdo idêntico não gera outro backup. O backup é concluído antes da substituição, inclusive quando o destino fica em outro disco.

Essa política também se aplica às configurações de SSH, Noctalia e Pi Agent, às migrações SDK e storage e aos arquivos DNS do mini-servidor. Não são criados backups permanentes nos diretórios dos programas. Backups antigos já existentes nesses locais não são movidos nem apagados automaticamente.

## Pi Agent

`Agentes de IA > Pi Agent > Configurar Pi Agent`:

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

Todos os backups locais gerados pela CLI ficam em `/mnt/backups`, em registros únicos por categoria, origem e data UTC:

```text
/mnt/backups/<categoria>/<hash-SHA256-da-origem>/<data-UTC>-<identificador>/
├── origin.json
└── original
```

As categorias são `files` (configurações e migração SDK), `browser`, `ssh`, `gpg`, `dns` e `snapshots`. O conteúdo original fica em `original`; o arquivo `origin.json` informa o caminho de origem e a data (no DNS, `origin.path` registra a origem). A estrutura privada usa `0700`; configurações DNS e snapshots, executados via sudo, têm categorias próprias pertencentes ao root, sem alterar o dono do diretório principal.

Cada execução cria uma nova geração e preserva as anteriores. A CLI prepara `/mnt/backups` com proprietário/grupo do usuário e permissão `0700`. Para execução isolada ou testes, `DOTFILES_BACKUP_DIR=/caminho/absoluto` substitui explicitamente a raiz; erros nunca causam fallback para o diretório do programa.

### Ponto de restauração

`Backup > Ponto de Restauração` cria um snapshot Btrfs gravável do subvolume raiz dentro de `/mnt/backups`. O registro fica em `snapshots/<hash-da-origem>/<data-UTC>-<identificador>/original`.

A ação prepara o destino centralizado e executa `btrfs subvolume snapshot / <destino>` com privilégios de administrador. O destino precisa pertencer ao mesmo sistema de arquivos Btrfs da raiz; se `/mnt/backups` estiver em outro disco ou sistema de arquivos, a criação do snapshot é recusada, sem gravá-lo em outro local.

### Backup do navegador

`Backup > Backup do Navegador` arquiva `.config/zen`, `.cache/zen` e `.local/share/keyrings` em `/mnt/backups/browser/<hash>/<geração>/original/zen-backup.tar`.

Feche o Zen Browser antes de iniciar. A CLI verifica os processos antes e depois de criar o arquivo.

A ação cria o arquivo temporário na nova geração. Em caso de falha, as gerações anteriores permanecem intactas. O arquivo recebe permissão `0600` porque contém dados do navegador e do chaveiro.

### Backup de projetos locais

`Backup > Backup de Projetos Locais` percorre toda a árvore de `/mnt/workspaces/reesearch64` e identifica diretórios que contêm `.git`.

Para cada projeto, a CLI executa nesta ordem:

```text
git push origin
git push backup
```

O remoto `origin` deve apontar para o GitHub. O remoto `backup` deve apontar para o GitLab. Se o envio ao GitHub falhar, a CLI não envia esse projeto ao GitLab. Ela continua processando os outros projetos e apresenta todas as falhas ao final.

Quando `/mnt/workspaces` não está pronto, a CLI usa `sudo install` para criá-lo, atribuí-lo ao usuário atual e aplicar a permissão `0700`. Ela também cria `/mnt/workspaces/reesearch64` e o link simbólico `~/Projects`, que aponta para `/mnt/workspaces`.

A ação envia somente commits e referências alcançados pelo `git push` configurado. Ela não cria commits para alterações locais pendentes.

### Backup das chaves SSH

`Backup > Backup das Chaves SSH` copia para `/mnt/backups/ssh/<hash>/<geração>/original/` somente as identidades exigidas pelo perfil atual. No desktop isso inclui os pares `id_trabalho_desktop`, `id_pessoal_desktop`, `id_github` e `id_gitlab`. No notebook inclui os pares `id_trabalho_notebook`, `id_github`, `id_gitlab` e a pública `id_trabalho_desktop.pub`.

A CLI valida todos os arquivos do perfil antes de criar o backup. Se algum estiver ausente ou não for um arquivo regular, a ação informa os nomes faltantes e não inicia a cópia. Chaves privadas de outro dispositivo nunca são exigidas nem copiadas.

Os diretórios de backup SSH recebem permissão `0700`. Cada geração mantém a permissão dos arquivos da origem, sem substituir gerações anteriores. Proteja as chaves privadas em `~/.ssh` com permissão `0600`.

### Backup das chaves GPG

`Backup > Backup das Chaves GPG` exporta em formato ASCII Armor todas as chaves disponíveis no keyring GPG:

| Conteúdo | Destino | Permissão |
| --- | --- | --- |
| Chaves públicas | `/mnt/backups/gpg/<hash>/<geração>/original/minha_chave_publica.asc` | `0644` |
| Chaves privadas | `/mnt/backups/gpg/<hash>/<geração>/original/minha_chave_privada.asc` | `0600` |

A ação exige o comando `gpg`, normalmente fornecido pelo pacote `gnupg` instalado em `Desenvolvimento > GPG`. O diretório `/mnt/backups/gpg` recebe permissão `0700`.

As duas exportações são concluídas antes da gravação dos arquivos. A CLI rejeita resultados sem os cabeçalhos OpenPGP esperados e usa arquivos temporários dentro de uma nova geração, preservando todos os backups existentes.

## Estrutura do repositório

```text
.
├── assets/
│   ├── face.jpg
│   └── wallpapers/
├── cmd/dotfiles/          # Código e testes da CLI
├── setup/
│   ├── desktop/          # Configurações completas CachyOS
│   ├── laptop/           # Cópia independente CachyOS
│   ├── homelab/          # Homelab Ubuntu
│   ├── homeprod/         # Bootstrap da VPS Ubuntu
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

Use outro checkout com `DOTFILES_DIR`:

```bash
DOTFILES_DIR=/caminho/para/dotfiles dotfiles
```

Sem essa variável, a CLI resolve o repositório pelo executável. Se isso falhar, ela usa `~/.dotfiles`.

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
