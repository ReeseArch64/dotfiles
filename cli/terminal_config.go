package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func runTerminalConfig(p profile, app string) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a configuração pelo menu Desktop ou Laptop como usuário normal")
	}
	var name, filename string
	switch app {
	case "fish":
		name, filename = "Fish", "config.fish"
	case "fastfetch":
		name, filename = "Fastfetch", "config.json"
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
	asset := filepath.Join("cachyos", app, filename)
	source := filepath.Join(home, ".dotfiles", asset)
	if binary, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(filepath.Dir(binary)), asset)
		if _, err := os.Stat(local); err == nil {
			source = local
		}
	}
	destination := filepath.Join(home, ".config", app, filename)
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup(name, console.commands.interactive)
	printActionHeader()
	fmt.Fprintln(console.output, "Aplicar a configuração em", destination)
	fmt.Fprintln(console.output, "A configuração anterior recebe backup em", location.path)
	if err = console.confirm("Digite CONFIRMO para aplicar: ", "CONFIRMO"); err == nil {
		var backup string
		backup, err = applyTerminalConfig(source, destination, filepath.Join(home, ".dotfiles-"+app+".lock"), location)
		if backup != "" {
			fmt.Fprintln(console.output, "Backup:", backup)
		}
	}
	if err != nil {
		fmt.Fprintln(console.output, err)
	} else {
		fmt.Fprintln(console.output, "Configuração aplicada.")
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
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
