package main

import (
	"fmt"
	"path/filepath"
)

const androidCLIInstallCommand = "curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash"

func (installer packageInstaller) installAndroidCLI() error {
	c := installer.console
	local := filepath.Join(c.home, ".local", "bin", "android")
	unlock, err := lockOperation(local, "instalação do Android CLI")
	if err != nil {
		return err
	}
	defer unlock()
	for _, name := range []string{local, "android"} {
		binary, err := installer.which(name)
		if err != nil || !filepath.IsAbs(binary) {
			continue
		}
		if _, err := c.commands.capture("", binary, "--version"); err == nil {
			fmt.Fprintln(c.output, "Android CLI já está instalado em", binary)
			return nil
		}
	}
	fmt.Fprintln(c.output, "Instalar o Android CLI do Google em", local)
	if err := c.confirm("Digite CONFIRMO para instalar: ", "CONFIRMO"); err != nil {
		return err
	}
	if err := c.ensurePackages("curl"); err != nil {
		return err
	}
	if err := c.commands.interactive("bash", "-o", "pipefail", "-c", androidCLIInstallCommand); err != nil {
		return fmt.Errorf("instalar Android CLI: %w", err)
	}
	if _, err := c.commands.capture("", local, "--version"); err != nil {
		return fmt.Errorf("verificar Android CLI em %s: %w", local, err)
	}
	return nil
}
