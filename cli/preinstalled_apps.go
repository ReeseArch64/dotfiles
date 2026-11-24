package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func (p profile) canRemovePreinstalledApps(osID string) bool {
	return p.sshKeyName() != "" && osID == "cachyos"
}

func runPreinstalledCleanup(p profile, console sshConsole) error {
	if !p.canRemovePreinstalledApps(systemOSID()) || os.Geteuid() == 0 {
		return errors.New("execute a remoção no Desktop ou Laptop com CachyOS como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	console.home = home
	return removePreinstalledApps(&console)
}

func removePreinstalledApps(c *sshConsole) error {
	unlock, err := lockOperation(filepath.Join(c.home, "preinstalled-apps"), "remoção de aplicativos pré-instalados")
	if err != nil {
		return err
	}
	defer unlock()
	output, err := c.commands.capture("", "pacman", "-Qq")
	if err != nil {
		return fmt.Errorf("consultar pacotes instalados: %w", err)
	}
	installed := strings.Fields(output)
	if !slices.Contains(installed, "kitty") {
		return errors.New("instale o Kitty em Programas → Terminal antes de remover o Alacritty")
	}
	var packages []string
	for _, name := range []string{"alacritty", "meld", "shelly"} {
		if slices.Contains(installed, name) {
			packages = append(packages, name)
		}
	}
	if len(packages) > 0 {
		fmt.Fprintln(c.output, "Pacotes a desinstalar:", strings.Join(packages, ", "))
	} else {
		fmt.Fprintln(c.output, "Alacritty, Meld e Shelly já estão desinstalados.")
	}
	paths := []string{".config/alacritty", ".cache/alacritty", ".config/meld", ".local/share/meld", ".cache/meld"}
	fmt.Fprintln(c.output, "Caminhos de dados a apagar se existirem:")
	for _, path := range paths {
		fmt.Fprintln(c.output, "  "+filepath.Join(c.home, path))
	}
	if err := c.confirm("Desinstalar os aplicativos e apagar os dados listados?"); err != nil {
		return err
	}
	if len(packages) > 0 {
		if err := c.commands.interactive(append([]string{"sudo", "pacman", "-Rns", "--"}, packages...)...); err != nil {
			return fmt.Errorf("desinstalar aplicativos: %w", err)
		}
	}
	for _, path := range paths {
		parent, name := filepath.Split(filepath.Join(c.home, path))
		// Open the parent so migrated cache directories work and links within an app's data are removed as links.
		root, err := os.OpenRoot(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("abrir pasta de dados %s: %w", parent, err)
		}
		if err := errors.Join(root.RemoveAll(name), root.Close()); err != nil {
			return fmt.Errorf("remover dados em %s: %w", filepath.Join(c.home, path), err)
		}
	}
	return nil
}
