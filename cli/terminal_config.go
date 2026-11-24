package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func runTerminalConfig(p profile, app string, console sshConsole) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a configuração pelo menu Desktop ou Laptop como usuário normal")
	}
	var name, filename string
	switch app {
	case "fish":
		name, filename = "Fish", "config.fish"
	case "fastfetch":
		name, filename = "Fastfetch", "config.jsonc"
	default:
		return errors.New("ação de configuração inválida")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	source := cachyosAsset(home, app, filename)
	destination := filepath.Join(home, ".config", app, filename)
	location := managedBackup(name, console.commands.interactive)
	lock := filepath.Join(home, app)
	packages := terminalConfigPackages(app, console.commands)
	if app == "fish" && findMise(home, exec.LookPath) == "" {
		packages = append(packages, "mise")
	}
	fmt.Fprintln(console.output, "Aplicar a configuração em", destination)
	fmt.Fprintln(console.output, "Instalar se estiverem ausentes:", strings.Join(packages, ", "))
	if app == "fastfetch" {
		fmt.Fprintln(console.output, "Copiar a foto de perfil para ~/.face se ainda não existir.")
	}
	fmt.Fprintln(console.output, "A configuração anterior recebe backup em", location.path)
	err = console.ensurePackages(packages...)
	if err == nil && app == "fastfetch" {
		err = ensureProfilePhoto(cachyosAsset(home, app, "face.jpg"), filepath.Join(home, ".face"), lock)
	}
	if err == nil {
		var backup string
		backup, err = applyTerminalConfig(source, destination, lock, location)
		if backup != "" {
			fmt.Fprintln(console.output, "Backup:", backup)
		}
	}
	return err
}

func cachyosAsset(home, app, name string) string {
	return dotfilesAsset(home, "cachyos", app, name)
}

func dotfilesAsset(home string, parts ...string) string {
	asset := filepath.Join(parts...)
	if binary, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(filepath.Dir(binary)), asset)
		if _, err := os.Stat(local); err == nil {
			return local
		}
	}
	return filepath.Join(home, ".dotfiles", asset)
}

func terminalConfigPackages(app string, commands sshCommands) []string {
	if app == "fastfetch" {
		return []string{"fastfetch", "chafa"}
	}
	packages := []string{"fish", "eza", "bat", "util-linux", "man-db"}
	for _, name := range []string{"cachyos-fish-config", "cachyos-rate-mirrors"} {
		if _, err := commands.capture("", "pacman", "-Si", "--", name); err == nil {
			packages = append(packages, name)
		}
	}
	if slices.Contains(packages, "cachyos-fish-config") {
		packages = append(packages, "libnotify")
	}
	return packages
}

func ensureProfilePhoto(source, destination, lock string) error {
	unlock, err := lockOperation(lock, "foto de perfil")
	if err != nil {
		return err
	}
	defer unlock()
	if info, err := os.Stat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("a foto de perfil precisa ser um arquivo: %s", destination)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	image, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("ler foto de perfil do Dotfiles: %w", err)
	}
	return writeConfigAtomic(destination, image, 0644)
}

func applyTerminalConfig(source, destination, lock string, location backupLocation) (string, error) {
	contents, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("ler configuração do Dotfiles: %w", err)
	}
	unlock, err := lockOperation(lock, "configuração do terminal")
	if err != nil {
		return "", err
	}
	defer unlock()

	var backup string
	mode := os.FileMode(0644)
	info, err := os.Lstat(destination)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("caminho não regular: %s", destination)
		}
		original, err := os.ReadFile(destination)
		if err != nil || bytes.Equal(original, contents) {
			return "", err
		}
		backup, err = location.saveFile(filepath.Base(destination)+".backup-*", original)
		if err != nil {
			return "", err
		}
		mode = info.Mode().Perm()
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return "", err
		}
	default:
		return "", err
	}
	return backup, writeConfigAtomic(destination, contents, mode)
}
