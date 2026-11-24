package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const homelabFishShell = "/usr/bin/fish"

func configureHomelabTerminal(c *sshConsole, username string, fish bool) error {
	if fish {
		fmt.Fprintln(c.output, "Configurar Fish como shell de login e mostrar o Fastfetch ao abrir uma sessão interativa.")
	} else {
		fmt.Fprintln(c.output, "Configurar Fastfetch para sessões SSH e copiar a foto para ~/.face se estiver ausente.")
	}
	if !filepath.IsAbs(c.home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(c.home, ".config")
	}
	if fish {
		fishSource := dotfilesAsset(c.home, "ubuntu", "homelab", "fish", "config.fish")
		if err := sshRegular(fishSource, false); err != nil {
			return fmt.Errorf("ler configuração do Fish: %w", err)
		}
		if err := c.ensureUbuntuPackages("fish"); err != nil {
			return err
		}
		if err := applyHomelabTerminalConfig(c, "Fish", "fish", fishSource, filepath.Join(config, "fish", "config.fish")); err != nil {
			return err
		}
		if err := setHomelabFishLogin(c, username); err != nil {
			return err
		}
		fmt.Fprintf(c.output, "Shell de login de %s confirmado como %s. Abra uma nova sessão SSH.\n", username, homelabFishShell)
	}
	if err := configureHomelabFastfetch(c, config); err != nil {
		return err
	}
	if !fish {
		fmt.Fprintln(c.output, "Fastfetch e foto de perfil configurados.")
		return nil
	}
	if err := c.ensureUbuntuPackages("bat", "bsdextrautils"); err != nil {
		return err
	}
	return ensureHomelabHushLogin(c.home)
}

func configureHomelabFastfetch(c *sshConsole, config string) error {
	source := dotfilesAsset(c.home, "ubuntu", "homelab", "fastfetch", "config.jsonc")
	if err := sshRegular(source, false); err != nil {
		return fmt.Errorf("ler configuração do Fastfetch: %w", err)
	}
	if err := c.ensureUbuntuPackages("chafa"); err != nil {
		return err
	}
	if err := installHomelabFastfetch(c); err != nil {
		return fmt.Errorf("instalar Fastfetch: %w", err)
	}
	lock := filepath.Join(c.home, "fastfetch")
	if err := ensureProfilePhoto(cachyosAsset(c.home, "fastfetch", "face.jpg"), filepath.Join(c.home, ".face"), lock); err != nil {
		return err
	}
	return applyHomelabTerminalConfig(c, "Fastfetch", "fastfetch", source, filepath.Join(config, "fastfetch", "config.jsonc"))
}

func applyHomelabTerminalConfig(c *sshConsole, category, app, source, destination string) error {
	backup, err := applyTerminalConfig(source, destination, filepath.Join(c.home, app), managedBackup(category, c.commands.interactive))
	if backup != "" {
		fmt.Fprintln(c.output, "Backup:", backup)
	}
	return err
}

func setHomelabFishLogin(c *sshConsole, username string) error {
	if _, err := c.commands.capture("", homelabFishShell, "--version"); err != nil {
		return fmt.Errorf("executar %s: %w", homelabFishShell, err)
	}
	readShell := func() (string, error) {
		entry, err := c.commands.capture("", "getent", "--", "passwd", username)
		if err != nil {
			return "", fmt.Errorf("consultar o shell de login de %s: %w", username, err)
		}
		fields := strings.Split(strings.TrimRight(entry, "\r\n"), ":")
		if len(fields) != 7 || fields[0] != username {
			return "", fmt.Errorf("registro de conta inválido para %s", username)
		}
		return fields[6], nil
	}
	shell, err := readShell()
	if err != nil || shell == homelabFishShell {
		return err
	}
	if err := c.commands.interactive("sudo", "chsh", "-s", homelabFishShell, "--", username); err != nil {
		return fmt.Errorf("definir o Fish como shell de login de %s: %w", username, err)
	}
	shell, err = readShell()
	if err != nil {
		return err
	}
	if shell != homelabFishShell {
		return fmt.Errorf("o shell de login de %s ficou registrado como %q. O esperado era %s", username, shell, homelabFishShell)
	}
	return nil
}

func ensureHomelabHushLogin(home string) error {
	path := filepath.Join(home, ".hushlogin")
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
	return nil
}
