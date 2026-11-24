package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var misePackages = []string{
	"base-devel", "curl", "git", "unzip", "re2c",
	"openssl", "bzip2", "zlib", "readline", "libyaml", "libffi", "sqlite", "tk",
	"libxml2", "libxslt", "libzip", "oniguruma", "icu", "gmp", "libsodium",
	"libpng", "libjpeg-turbo", "freetype2", "libwebp", "libsecret",
}

func runMiseSetup(p profile, console sshConsole) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Mise pelo menu Desktop ou Laptop como usuário normal")
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
		return errors.New("as pastas pessoal e de configuração precisam ter caminhos absolutos")
	}
	console.home = home
	source := cachyosAsset(home, "mise", "mise.toml")
	destination := filepath.Join(config, "mise", "config.toml")
	location := managedBackup("Mise", console.commands.interactive)
	fmt.Fprintln(console.output, "Instalar o Mise e os pacotes necessários para as ferramentas, incluindo a compilação do PHP.")
	fmt.Fprintln(console.output, "Aplicar", destination, "e executar mise install.")
	fmt.Fprintln(console.output, "A configuração anterior recebe backup em", location.path)
	return configureMise(console, source, destination, location, exec.LookPath)
}

func configureMise(console sshConsole, source, destination string, location backupLocation, lookup func(string) (string, error)) error {
	contents, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("ler mise.toml do Dotfiles: %w", err)
	}
	unlock, err := lockOperation(filepath.Join(console.home, "mise"), "configuração do Mise")
	if err != nil {
		return err
	}
	defer unlock()
	mise, err := ensureMise(&console, lookup, misePackages...)
	if err != nil {
		return fmt.Errorf("instalar pacotes do Mise: %w", err)
	}
	backup, err := applyConfigFiles([]configFile{{destination, contents, 0600}}, location, "mise")
	if backup != "" {
		fmt.Fprintln(console.output, "Backup do Mise:", backup)
	}
	if err != nil {
		return err
	}
	if err := console.commands.interactive(mise, "trust", destination); err != nil {
		return fmt.Errorf("confiar na configuração do Mise: %w", err)
	}
	if err := console.commands.interactive("env", "MISE_GLOBAL_CONFIG_FILE="+destination, mise, "--cd", "/", "install"); err != nil {
		return fmt.Errorf("executar mise install: %w", err)
	}
	return nil
}
