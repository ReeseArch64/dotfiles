# ReeseArch64 Dotfiles

Dotfiles pessoais para máquinas CachyOS com Niri e Noctalia Shell. O projeto inclui uma CLI interativa para Git, SSH, UFW, foto de perfil, scripts e diagnóstico do sistema.

## Recursos

- Instala e configura Git, gitflow-next e Lazygit.
- Gera a identidade do Git a partir de um `.env` local.
- Cria links simbólicos para configurações globais do Git.
- Instala GnuPG e importa chaves públicas e privadas de arquivos `.asc`.
- Configura o Niri em `~/.config/niri` com um link para o repositório.
- Verifica os plugins e configura o Noctalia em `~/.local/state/noctalia`.
- Instala a foto de perfil do repositório como `~/.face`.
- Impede a execução fora da combinação CachyOS, Niri e Noctalia Shell.
- Gerencia `sshd.service` e `sshd.socket`.
- Aplica ou remove hardening de acesso SSH.
- Instala e gerencia regras do firewall UFW.
- Exibe informações da máquina e do repositório.
- Descobre e executa scripts de `scripts/`.

## Requisitos

Este projeto suporta somente esta combinação:

1. CachyOS
2. Niri
3. Noctalia Shell

Antes de abrir a interface, a CLI exige `ID=cachyos` em `/etc/os-release`. Ela também exige os executáveis `niri` e `noctalia` no `PATH`. A execução termina com uma mensagem de erro quando qualquer requisito está ausente.

A instalação e as ações também usam:

- `bash`
- `git`
- `gpg`, instalado pelo pacote `gnupg` quando necessário
- `make`
- Go compatível com a versão declarada em `cli/go.mod`
- `sudo`
- `pacman`
- `systemd`
- `shelly`, usado para instalar `gitflow-next-bin` e `lazygit`

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
GPG_PUBLIC_IMPORT=/run/media/seu-usuario/pendrive/chave_publica.asc
GPG_PRIVATE_IMPORT=/run/media/seu-usuario/pendrive/chave_privada.asc
```

O `.env` está no `.gitignore` e não deve ser versionado.

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

A CLI não oferece opção para ignorar a validação de plataforma.

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

## Configuração do Git

Abra `Git > Configurar Git` na CLI. Essa ação executa as seguintes etapas:

1. Lê `GIT_USER_EMAIL`, `GIT_USERNAME` e `GIT_USER_NAME` do `.env`.
2. Gera `git/.gitconfig` de forma atômica.
3. Mantém `user.signingkey` vazio.
4. Instala `git`, `gitflow-next-bin` e `lazygit` quando necessário.
5. Cria o diretório `~/.config/git`.
6. Cria os quatro links simbólicos globais.

Os destinos são:

| Destino | Origem no repositório | Finalidade |
| --- | --- | --- |
| `~/.gitconfig` | `git/.gitconfig` | Identidade do usuário |
| `~/.gitattributes` | `git/.gitattributes` | Tratamento global de arquivos e diffs |
| `~/.gitignore` | `git/.gitignore` | Exclusões globais |
| `~/.config/git/config` | `git/config` | Comportamento global do Git |

A configuração substitui qualquer arquivo ou link existente nesses destinos. Faça backup de configurações locais antes de executar essa ação.

### Assinatura de commits

`user.signingkey` permanece vazio por enquanto. `git/config` mantém `commit.gpgsign = true`, portanto o Git tentará usar uma chave padrão disponível. Configure uma chave depois com:

```bash
git config --global user.signingkey ID_DA_CHAVE
```

A execução posterior de `Git > Configurar Git` volta a deixar esse campo vazio.

## Importação de chaves GPG

Configure o Git antes de abrir `GPG > Importar chaves`. A ação valida os quatro links do Git, a identidade e os pacotes instalados pelo fluxo de Git.

Defina `GPG_PUBLIC_IMPORT` e `GPG_PRIVATE_IMPORT` no `.env`. Cada variável deve apontar para um arquivo regular com extensão `.asc`. Caminhos absolutos permitem importar diretamente de pendrives montados em `/run/media`, discos externos ou qualquer outro diretório acessível.

A ação executa as seguintes etapas:

1. Valida que o Git está configurado e que os dois arquivos existem.
2. Instala o pacote `gnupg` com `pacman` quando necessário.
3. Importa a chave pública com `gpg --import`.
4. Importa a chave privada com `gpg --import`.

Os arquivos podem ficar fora do repositório e não são copiados pela CLI. O padrão `*.asc` está no `.gitignore` para impedir o versionamento acidental de chaves exportadas.

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

A opção `Niri` cria `~/.config/niri` como link simbólico para `niri/` no repositório. Quando o destino já existe, a CLI preserva a configuração anterior em `~/.config/niri.backup-AAAAMMDD-HHMMSS` antes de criar o link.

A ação mantém o link existente quando ele já aponta para a configuração deste repositório.

## Noctalia

A tela `Noctalia` lista os plugins necessários e indica quais ainda precisam ser baixados pela interface do Noctalia:

- `github-kanban`
- `llamanager`
- `mini-docker`
- `noctaproton-vpn`
- `pomodoro`
- `ssh-launcher`
- `vpn-manager`
- `warp`
- `zed-provider`

A opção `Configurar Noctalia` exige todos esses diretórios em `~/.local/state/noctalia/plugins/materialized/community`. A CLI não baixa os plugins.

Após a validação, a CLI cria links para `noctalia/settings.toml` e `noctalia/state.toml`. Os demais dados do Noctalia, inclusive os plugins, permanecem no diretório de estado.

Arquivos existentes são preservados com o sufixo `.backup-AAAAMMDD-HHMMSS` antes da ativação.

## Foto de perfil

A opção `Foto de perfil` cria `~/.face` como link simbólico para `.face` no repositório. A imagem versionada é um JPEG quadrado de 300 por 300 pixels.

A ação substitui um arquivo ou link existente em `~/.face`. Faça backup da foto atual antes de executar a opção.

## SSH

O menu `SSH` mostra o estado do servidor, o modo de inicialização, a porta, o hardening, as chaves autorizadas e os endereços locais.

As ações disponíveis são:

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
├── niri/                # Configuração vinculada em ~/.config/niri
├── noctalia/            # Arquivos vinculados no estado do Noctalia
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
- Adicione uma chave autorizada antes de habilitar o hardening SSH.
- Libere a porta correta antes de ativar o firewall em uma máquina remota.
- Faça backup dos quatro destinos Git e de `~/.face` antes de criar os links.
- Nunca versione `.env`, chaves privadas ou credenciais.
- Desmonte o pendrive após importar as chaves quando ele não estiver em uso.

Para remover somente o comando instalado:

```bash
rm -f ~/.local/bin/dotfiles
make clean
```

A remoção do comando não desfaz serviços, regras de firewall ou links simbólicos criados anteriormente. Remova `~/.face` manualmente quando quiser desfazer a foto de perfil.
