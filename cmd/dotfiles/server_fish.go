package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const serverFishInstallScript = `set -Eeuo pipefail
[[ $EUID == 0 ]] || { echo 'Execute como root ou via sudo.' >&2; exit 1; }
. /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo 'Esta instalação requer Ubuntu Server.' >&2; exit 1; }
apt-get update
apt-get install --yes fish chafa bat bsdextrautils
`

func serverFishSource(dotfiles string) string {
	return filepath.Join(dotfiles, "setup", "server", "fish", "config.fish")
}

func installServerFish(dotfiles, home string) error {
	// Do not use installFishConfig: servers must retain Bash and recovery files.
	return copyFileAtomic(serverFishSource(dotfiles), filepath.Join(home, ".config", "fish", "config.fish"))
}

func serverFishJob(dotfiles, setupDir string) job {
	home, err := os.UserHomeDir()
	if err != nil {
		return job{title: "Configurar Fish", steps: []step{nativeStep("Localizar diretório pessoal", func() error { return err })}}
	}
	username, err := currentUsername()
	if err != nil {
		return job{title: "Configurar Fish", steps: []step{nativeStep("Identificar usuário atual", func() error { return err })}}
	}
	privileged := func(label string, args ...string) []step {
		if os.Geteuid() == 0 {
			return []step{terminalStep(label, args...)}
		}
		return withSudo(terminalStep(label, append([]string{"sudo"}, args...)...))
	}
	configured := serverFastfetchJob(dotfiles, setupDir)
	configured.title = "Configurar Fish"
	configured.steps = append([]step{nativeStep("Validar configuração Fish para servidores", func() error {
		if filepath.Base(setupDir) != "homelab" && filepath.Base(setupDir) != "homeprod" {
			return fmt.Errorf("configuração Fish disponível somente para Homelab e HomeProd")
		}
		info, err := os.Stat(serverFishSource(dotfiles))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuração Fish não é arquivo regular")
		}
		return nil
	})}, configured.steps...)
	configured.steps = append(configured.steps, privileged("Instalar Fish, Chafa e ferramentas de man no Ubuntu", "bash", "-c", serverFishInstallScript)...)
	configured.steps = append(configured.steps, nativeStep("Copiar configuração Fish para servidores", func() error {
		return installServerFish(dotfiles, home)
	}))
	configured.steps = append(configured.steps, privileged("Definir Fish como shell de login de "+username, "chsh", "-s", "/usr/bin/fish", username)...)
	configured.result = func() string {
		return "Fish, Fastfetch e foto configurados para " + username + ". Abra uma nova sessão SSH para usar o Fish."
	}
	return configured
}
