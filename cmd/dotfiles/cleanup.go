package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var preinstalledPackages = []string{"firefox", "alacritty", "meld"}

const removePreinstalledPackagesScript = `packages=()
for package in "$@"; do
    if pacman -Q "$package" >/dev/null 2>&1; then
        packages+=("$package")
    fi
done
if [ "${#packages[@]}" -eq 0 ]; then
    printf 'Nenhum aplicativo pré-instalado está instalado.\n'
    exit 0
fi
sudo pacman -Rns "${packages[@]}"`

func validateGhosttyInstalled(missing []string) error {
	if len(missing) != 0 {
		return errors.New("instale e configure o Ghostty antes de remover o Alacritty")
	}
	return nil
}

func preinstalledApplicationDirectories(home string) []string {
	return []string{
		filepath.Join(home, ".mozilla"),
		filepath.Join(home, ".cache", "mozilla"),
		filepath.Join(home, ".config", "alacritty"),
		filepath.Join(home, ".cache", "alacritty"),
		filepath.Join(home, ".config", "meld"),
		filepath.Join(home, ".local", "share", "meld"),
		filepath.Join(home, ".cache", "meld"),
	}
}

func removePreinstalledApplicationDirectories(home string) error {
	for _, path := range preinstalledApplicationDirectories(home) {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func cleanupJobFor(home string, missing func(...string) []string) job {
	steps := []step{
		nativeStep("Verificar instalação do Ghostty", func() error {
			return validateGhosttyInstalled(missing("ghostty"))
		}),
		pacmanSyncStep(),
		terminalStep(
			"Desinstalar "+strings.Join(preinstalledPackages, ", "),
			append([]string{"bash", "-c", removePreinstalledPackagesScript, "dotfiles-cleanup"}, preinstalledPackages...)...,
		),
		nativeStep("Remover dados dos aplicativos", func() error {
			return removePreinstalledApplicationDirectories(home)
		}),
	}
	return job{title: "Remover aplicativos pré-instalados", steps: steps}
}

func cleanupJob() job {
	home, _ := os.UserHomeDir()
	return cleanupJobFor(home, missingPkgs)
}

func (m model) cleanupItems() []item {
	return []item{{
		title: "Remover aplicativos",
		desc:  "Desinstalar Firefox, Alacritty e Meld e apagar seus dados",
		job:   cleanupJob,
	}}
}
