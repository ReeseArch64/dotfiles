package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func configureHomelabTerminal(c *sshConsole, username string, fish bool) error {
	if fish {
		fmt.Fprintln(c.output, "Configurar Fish como shell de login e mostrar o Fastfetch ao abrir uma sessão interativa.")
	} else {
		fmt.Fprintln(c.output, "Configurar Fastfetch para sessões SSH e copiar a foto para ~/.face se estiver ausente.")
	}
	if err := c.confirm("Digite CONFIRMO para aplicar: ", "CONFIRMO"); err != nil {
		return err
	}
	if !filepath.IsAbs(c.home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(c.home, ".config")
	}
	fastfetchSource := dotfilesAsset(c.home, "ubuntu", "homelab", "fastfetch", "config.jsonc")
	fishSource := dotfilesAsset(c.home, "ubuntu", "homelab", "fish", "config.fish")
	if err := sshRegular(fastfetchSource, false); err != nil {
		return fmt.Errorf("ler configuração do Fastfetch: %w", err)
	}
	packages := []string{"chafa"}
	if fish {
		if err := sshRegular(fishSource, false); err != nil {
			return fmt.Errorf("ler configuração do Fish: %w", err)
		}
		packages = append(packages, "fish", "bat", "bsdextrautils")
	}
	if err := c.ensureUbuntuPackages(packages...); err != nil {
		return err
	}
	if err := installHomelabFastfetch(c); err != nil {
		return fmt.Errorf("instalar Fastfetch: %w", err)
	}
	lock := filepath.Join(c.home, ".dotfiles-fastfetch.lock")
	if err := ensureProfilePhoto(cachyosAsset(c.home, "fastfetch", "face.jpg"), filepath.Join(c.home, ".face"), lock); err != nil {
		return err
	}
	if err := applyHomelabTerminalConfig(c, "Fastfetch", "fastfetch", fastfetchSource, filepath.Join(config, "fastfetch", "config.jsonc")); err != nil {
		return err
	}
	if !fish {
		fmt.Fprintln(c.output, "Fastfetch e foto de perfil configurados.")
		return nil
	}
	if err := applyHomelabTerminalConfig(c, "Fish", "fish", fishSource, filepath.Join(config, "fish", "config.fish")); err != nil {
		return err
	}
	if err := setHomelabFishLogin(c, username); err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Fish configurado como shell de login. Abra uma nova sessão SSH para usar a configuração.")
	return nil
}

func applyHomelabTerminalConfig(c *sshConsole, category, app, source, destination string) error {
	backup, err := applyTerminalConfig(source, destination, filepath.Join(c.home, ".dotfiles-"+app+".lock"), managedBackup(category, c.commands.interactive))
	if backup != "" {
		fmt.Fprintln(c.output, "Backup:", backup)
	}
	return err
}

func setHomelabFishLogin(c *sshConsole, username string) error {
	path := filepath.Join(c.home, ".hushlogin")
	if err := sshRegular(path, true); err != nil {
		return err
	}
	marker, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if marker != nil {
		if err := marker.Close(); err != nil {
			return errors.Join(err, os.Remove(path))
		}
	}
	if err := c.commands.interactive("sudo", "chsh", "-s", "/usr/bin/fish", "--", username); err != nil {
		if marker != nil {
			err = errors.Join(err, os.Remove(path))
		}
		return fmt.Errorf("definir o Fish como shell de login: %w", err)
	}
	return nil
}
