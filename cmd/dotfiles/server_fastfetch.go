package main

import (
	"fmt"
	"os"
)

// Official Debian packages; hashes are pinned from the upstream release assets:
// https://github.com/fastfetch-cli/fastfetch/releases/tag/2.70.0
const serverFastfetchInstallScript = `set -Eeuo pipefail
[[ $EUID == 0 ]] || { echo 'Execute como root ou via sudo.' >&2; exit 1; }
. /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo 'Esta instalação requer Ubuntu Server.' >&2; exit 1; }
version=2.70.0
case $(dpkg --print-architecture) in
  amd64)
    arch=amd64
    sha256=06e9a1be9a863dc815f8a31a6b1abec7b29967be30084ffcc29c1bca072d801b ;;
  arm64)
    arch=aarch64
    sha256=25053651486d97b8731dd6bc59e725944e39adcdcd903741334c1c3282a7281d ;;
  *) echo 'Arquitetura não suportada: use Ubuntu amd64 ou arm64.' >&2; exit 1 ;;
esac
apt-get update
apt-get install --yes curl ca-certificates
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
package="fastfetch-linux-${arch}.deb"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "https://github.com/fastfetch-cli/fastfetch/releases/download/${version}/${package}" \
  --output "$work/$package"
(cd "$work" && printf '%s  %s\n' "$sha256" "$package" | sha256sum --check --status)
apt-get install --yes "$work/$package"
fastfetch --version
`

func serverFastfetchJobFor(dotfiles, setupDir, home string, install []step) job {
	steps := []step{nativeStep("Validar configuração Fastfetch e imagem de perfil", func() error {
		for _, path := range []string{configPath(setupDir, "fastfetch", "config.json"), assetPath(dotfiles, "face.jpg")} {
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("ler %s: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s não é um arquivo regular", path)
			}
		}
		return nil
	})}
	steps = append(steps, install...)
	steps = append(steps,
		fastfetchConfigStep(setupDir, home),
		nativeStep("Criar symlink ~/.face", func() error { return installFace(dotfiles, home) }),
	)
	return job{
		title: "Fastfetch e foto de perfil",
		steps: steps,
		result: func() string {
			return "Fastfetch configurado e ~/.face criado para o usuário atual. Execute fastfetch no terminal."
		},
	}
}

func serverFastfetchJob(dotfiles, setupDir string) job {
	home, _ := os.UserHomeDir()
	var install []step
	if !commandSucceeds(nil, "fastfetch", "--version") {
		if os.Geteuid() == 0 {
			install = []step{terminalStep("Instalar Fastfetch para Ubuntu Server", "bash", "-c", serverFastfetchInstallScript)}
		} else {
			install = withSudo(terminalStep("Instalar Fastfetch para Ubuntu Server", "sudo", "bash", "-c", serverFastfetchInstallScript))
		}
	}
	return serverFastfetchJobFor(dotfiles, setupDir, home, install)
}
