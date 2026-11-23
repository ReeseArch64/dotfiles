package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var niriManagedFiles = []string{
	"cfg/animation.kdl", "cfg/autostart.kdl", "cfg/keybinds.kdl", "cfg/layout.kdl",
	"cfg/misc.kdl", "cfg/rules.kdl", "noctalia.kdl",
}

func runNiriSetup(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Niri pelo menu Desktop ou Laptop como usuário normal")
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
	source := filepath.Dir(filepath.Dir(dotfilesAsset(home, "cachyos", "niri", "cfg", "rules.kdl")))
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup("Niri", console.commands.interactive)
	printActionHeader()
	fmt.Fprintln(console.output, "Aplicar os arquivos do Dotfiles em", target)
	fmt.Fprintln(console.output, "Preservar config.kdl, cfg/input.kdl e cfg/display.kdl.")
	fmt.Fprintln(console.output, "Arquivos substituídos recebem backup em", location.path)
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
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("prepare a configuração local do Niri em %s antes de aplicar os arquivos", target)
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("a configuração do Niri precisa ser um diretório real: %s", target)
	}
	config := filepath.Join(target, "config.kdl")
	if info, err := os.Stat(config); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("a configuração local do Niri precisa existir em %s", config)
	}
	cfg := filepath.Join(target, "cfg")
	if info, err := os.Lstat(cfg); err == nil && !info.IsDir() {
		return "", fmt.Errorf("a pasta do Niri precisa ser um diretório real: %s", cfg)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := validateMigrationTree(target); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".dotfiles-niri-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err := copySDKTree(context.Background(), target, staging); err != nil {
		return "", err
	}
	stagedCfg := filepath.Join(staging, "cfg")
	if err := os.Mkdir(stagedCfg, 0755); errors.Is(err, os.ErrExist) {
		if info, err := os.Lstat(stagedCfg); err != nil || !info.IsDir() {
			return "", fmt.Errorf("a cópia de cfg do Niri precisa ser um diretório real: %s", stagedCfg)
		}
	} else if err != nil {
		return "", err
	}
	files := make([]configFile, 0, len(niriManagedFiles))
	for _, name := range niriManagedFiles {
		contents, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return "", err
		}
		candidate := filepath.Join(staging, name)
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(candidate); err != nil {
				return "", err
			}
		}
		if err := writeConfigAtomic(candidate, contents, 0644); err != nil {
			return "", err
		}
		files = append(files, configFile{filepath.Join(target, name), contents, 0644})
	}
	if err := validate(filepath.Join(staging, "config.kdl")); err != nil {
		return "", fmt.Errorf("validar a configuração do Niri: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "Pictures", "Screenshots"), 0755); err != nil {
		return "", err
	}
	return applyConfigFiles(files, location, "niri")
}
