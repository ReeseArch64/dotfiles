# ReeseArch64 Dotfiles

Esta pasta arquiva a versão anterior da CLI. Consulte o [README atual](../README.md) para instalar ou desenvolver a versão em uso.

Configuração pessoal para CachyOS com Niri e Noctalia Shell.

O executável legado abre apenas a opção **Sair**. As ações e os requisitos abaixo documentam o código arquivado para a migração.

## Migração do Desktop

O [Android CLI do Google](#android-cli) e o [Ollama com ROCm para GPU AMD](#ollama) já estão disponíveis na [CLI atual](../README.md).

O Proton VPN CLI ficou fora da migração. O código legado instala o pacote `proton-vpn-cli` pelo yay.

Também ficaram fora da migração a lixeira `~/.trash`, o login automático, o Thunderbird, o VS Code, o Better Stack e as antigas ações de backup de snapshots Btrfs, projetos, chaves SSH/GPG e perfis de navegador.

## Android CLI

`Desenvolvimento > Android CLI` executa o instalador:

```bash
curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash
```

O comando é executado com `bash -o pipefail` para detectar também falhas no download.

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

## Licença

Consulte [LICENSE](../LICENSE).
