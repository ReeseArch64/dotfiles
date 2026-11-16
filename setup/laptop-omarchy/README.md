# Laptop — Omarchy Linux

Perfil reconhecido pelo identificador de hostname `laptop-alan`, com Omarchy Linux (`ID=omarchy` em `/etc/os-release`). Não exige Niri nem Noctalia.

Oferece somente **Firewall > Configurar Firewall**, usando UFW: bloqueia entrada por padrão, permite saída, remove as liberações SSH global e LAN gerenciadas pelo fluxo existente e habilita o serviço no boot. Não adiciona liberação SSH. Outras regras UFW existentes são preservadas; revise-as antes de ativar. Execute localmente para evitar perder uma sessão remota.

Nada é aplicado automaticamente ao iniciar a CLI. Não prepara ou move pastas, não cria links simbólicos e não herda as configurações do notebook CachyOS (`setup/laptop/`) nem de `setup/config/`.
