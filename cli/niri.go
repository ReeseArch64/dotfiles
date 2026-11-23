package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func runNiriSetup(p profile) error {
	if p.prefix != "desktop" || os.Geteuid() == 0 {
		return errors.New("configure o Niri pelo perfil Desktop como usuário normal")
	}
	if _, err := exec.LookPath("niri"); err != nil {
		return errors.New("instale o Niri antes de aplicar a configuração")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(config) {
		return errors.New("a pasta pessoal e a pasta de configuração precisam ser absolutas")
	}
	target := filepath.Join(config, "niri")
	if custom := os.Getenv("NIRI_CONFIG"); custom != "" && filepath.Clean(custom) != filepath.Join(target, "config.kdl") {
		return fmt.Errorf("NIRI_CONFIG aponta para %s. A ação configura %s", custom, target)
	}
	source := filepath.Dir(cachyosAsset(home, "niri", "config.kdl"))
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup("Niri", console.commands.interactive)
	printActionHeader()
	fmt.Fprintln(console.output, "Aplicar a configuração completa do Desktop em", target)
	fmt.Fprintln(console.output, "A pasta anterior recebe backup em", location.path)
	if err := console.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	backup, err := configureNiri(source, target, home, location, func(candidate string) error {
		_, err := console.commands.capture("", "niri", "validate", "--config", candidate)
		return err
	})
	if backup != "" {
		fmt.Fprintln(console.output, "Backup:", backup)
	}
	if err == nil {
		fmt.Fprintln(console.output, "Configuração do Niri aplicada.")
	} else {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configureNiri(source, target, home string, location backupLocation, validate func(string) error) (string, error) {
	unlock, err := lockOperation(target, "configuração do Niri")
	if err != nil {
		return "", err
	}
	defer unlock()
	if info, err := os.Lstat(target); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("a configuração do Niri precisa ser um diretório real: %s", target)
		}
		if err := validateMigrationTree(target); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := validate(filepath.Join(source, "config.kdl")); err != nil {
		return "", fmt.Errorf("validar a configuração do Niri: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "Pictures", "Screenshots"), 0755); err != nil {
		return "", err
	}
	backup, finish, err := installAssetDirectory(source, target, "niri", location)
	if err == nil && finish != nil {
		err = finish(false)
	}
	return backup, err
}
