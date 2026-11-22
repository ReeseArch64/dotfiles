# ReeseArch64 Dotfiles

A CLI identifica a máquina pelo hostname e mostra o ambiente e o autor no menu.

| Identificador | Ambiente | Chave SSH |
| --- | --- | --- |
| `desktop` | CachyOS · Niri · Noctalia Shell | `~/.ssh/id_desktop` |
| `laptop` | CachyOS · Niri · Noctalia Shell | `~/.ssh/id_laptop` |
| `homeprod`, `homelab` | Ubuntu | |

O identificador pode aparecer como parte do hostname separada por hífen, como em `desktop-alan`. A CLI ignora maiúsculas e minúsculas. Nomes sem identificador ou com mais de um perfil aparecem como **Sem suporte**.

## SSH e firewall

Em Desktop e Laptop, o menu **Sistema Operacional → SSH** oferece estas ações.

- **Configurar SSH** cria ou reaproveita a identidade Ed25519 do dispositivo. Instala um `~/.ssh/config` com a identidade única para qualquer destino. Guarda a configuração anterior em `~/.ssh/config.backup-*`, sem transportar aliases por IP ou chaves diferentes por destino. Valida e habilita o servidor OpenSSH para o usuário atual, somente por chave pública e sem login como root. O servidor mantém a porta já configurada e guarda o `sshd_config` anterior em `/etc/ssh/dotfiles-ssh-backup.*`.
- **Criar chave SSH** gera somente o par local. A passphrase é informada ao OpenSSH no terminal. Arquivos existentes são preservados.
- **Chaves autorizadas** reúne as ações de adicionar, visualizar e remover chaves públicas em `~/.ssh/authorized_keys`. Para autorizar um dispositivo, cole sua chave pública e confira o fingerprint. O IP de origem não é vinculado à chave.

**Configurar SSH** também habilita o socket do `ssh-agent` para o usuário. Depois de informar a passphrase no primeiro uso da chave, o OpenSSH pode reutilizá-la por 8 horas em outros terminais e destinos. Reiniciar ou encerrar o agente limpa o cache.

**Firewall** valida o SSH antes de ativar o UFW. Bloqueia entrada por padrão, permite saída e limita novas conexões SSH à interface e à sub-rede IPv4 privada detectadas. Mantém as regras de outros serviços e recusa aplicar a configuração quando a sessão SSH atual está fora da LAN. Use o console local para a primeira configuração e teste uma nova conexão antes de encerrar a sessão atual.

HomeProd e Homelab continuam apenas identificados no menu. Desktop e Laptop podem usar suas próprias chaves para acessá-los quando as públicas estiverem autorizadas nesses servidores.

## Mouse personalizado

**Sistema Operacional → Mouse personalizado** instala o tema FrierenBLZ em `~/.icons` e aplica o `cachyos/niri/cfg/misc.kdl` do legado. A ação espera que o `config.kdl` do Niri inclua `cfg/misc.kdl` e valida a configuração candidata com `niri validate`. Arquivos e temas substituídos recebem backups.

## Navegador Web

**Navegador Web → Backup Completo** exige o Firefox fechado. Salva as pastas de dados e os perfis registrados em `/mnt/backups/Firefox/firefox-AAAA-MM-DD-HH-MM-SS.tar`. Inclui `profiles.ini`, arquivos de login e chaves do Firefox, cookies, sessões, extensões e demais dados de cada perfil. Caminhos dentro da pasta pessoal são armazenados relativos a ela. Perfis registrados fora dela ficam no tar com seu caminho sem a barra inicial.

A ação usa `sudo` para criar o destino quando necessário, com acesso exclusivo ao usuário. O tar é privado e contém dados sensíveis. A senha principal do Firefox, quando configurada, continua necessária para abrir os logins salvos. Sites ainda podem expirar sessões ou exigir nova autenticação. [Dados armazenados nos perfis do Firefox](https://support.mozilla.org/en-US/kb/profiles-where-firefox-stores-user-data)

## Instalação

O instalador baixa a última release para Linux `amd64`, verifica o SHA-256 e instala o binário em `~/.local/bin`. Prepara `~/.dotfiles` e adiciona os arquivos CachyOS que estiverem faltando. O Go é necessário para compilar, e o OpenSSH é necessário ao gerar uma chave.

```sh
curl -fsSL https://raw.githubusercontent.com/ReeseArch64/dotfiles/main/install.sh | sh
~/.local/bin/dotfiles
```

## Desenvolvimento

- [cli/](cli/) contém o módulo Go ativo.
- [cachyos/](cachyos/) e [ubuntu/](ubuntu/) reservam espaço para configurações de cada sistema.
- [legacy/](legacy/) contém a implementação anterior, suas configurações e seus arquivos de referência.

Execute `make test` para testar a CLI e `make cli` para compilar `bin/dotfiles`.
