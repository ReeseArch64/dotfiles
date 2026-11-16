# Laptop — Omarchy Linux

Perfil reconhecido pelo identificador de hostname `laptop-alan`, com Omarchy Linux (`ID=omarchy` em `/etc/os-release`). Não exige Niri nem Noctalia.

Oferece somente **Firewall > Configurar Firewall**. A opção fica escurecida, sem destaque de seleção e sem permitir execução enquanto os pré-requisitos SSH não forem atendidos. O subtítulo mostra apenas “Requer SSH configurado”. Não existe configuração SSH neste perfil por enquanto: a CLI apenas verifica, sem instalar, copiar chaves ou configurar SSH.

A checagem exige OpenSSH instalado, configuração do servidor legível, porta válida, `sshd.service` ativo e habilitado no boot, os arquivos de identidades exigidos pelo notebook CachyOS, `~/.ssh/config` não vazio e `authorized_keys` contendo somente a chave de `id_trabalho_desktop.pub`. A existência dos arquivos não comprova que as chaves privadas funcionam ou que o acesso remoto foi testado. O estado é relido ao abrir a CLI, entrar/voltar nas telas e antes de executar o job; não depende de flags `ssh=true` ou `firewall=true` em `config.txt`.

Após desbloqueado, o firewall instala UFW e iptables-nft se ausentes, bloqueia entrada por padrão, permite saída, substitui a liberação SSH global pela regra SSH somente na LAN detectada e habilita o serviço no boot. Outras regras UFW existentes são preservadas; revise-as antes de ativar. Execute localmente para evitar perder uma sessão remota.

Nada é aplicado automaticamente ao iniciar a CLI. Não prepara ou move pastas, não cria links simbólicos e não herda as configurações do notebook CachyOS (`setup/laptop/`) nem de `setup/config/`.
