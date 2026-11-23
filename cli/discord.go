package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func (installer packageInstaller) setupDiscord() error {
	c := installer.console
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(config) {
		return errors.New("o diretório de configuração do Discord precisa ter um caminho absoluto")
	}
	betterDiscord := filepath.Join(config, "BetterDiscord")
	source := cachyosAsset(c.home, "discord", "BetterDiscord")
	files, err := betterDiscordConfigFiles(source, betterDiscord)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Instalar o Discord Stable pelo Pacman, aplicar BetterDiscord e importar os temas e plugins do Dotfiles.")
	fmt.Fprintln(c.output, "Esta ação modifica o Discord nativo do usuário. Feche o Discord antes da aplicação.")
	if err := c.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	unlock, err := lockOperation(betterDiscord, "configuração do BetterDiscord")
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.ensurePackages("discord", "procps-ng"); err != nil {
		return err
	}
	bdcli := installer.findDiscordExecutable("bdcli")
	if bdcli == "" {
		if err := installer.ensureYay(); err != nil {
			return err
		}
		if err := c.commands.interactive("yay", "-S", "--needed", "--", "bdcli-bin"); err != nil {
			return err
		}
		bdcli = installer.findDiscordExecutable("bdcli")
		if bdcli == "" {
			return errors.New("executável bdcli indisponível após a instalação")
		}
	}
	discord := filepath.Join(config, "discord")
	resources, err := waitForDiscordResources(c, discord, func() error {
		path := installer.findDiscordExecutable("discord")
		if path == "" {
			return errors.New("executável discord indisponível após a instalação")
		}
		cmd := exec.Command(path)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	})
	if err != nil {
		return err
	}
	location := managedBackup("BetterDiscord", c.commands.interactive)
	backup, err := applyConfigFiles(files, location, "betterdiscord")
	if backup != "" {
		fmt.Fprintln(c.output, "Configurações anteriores em", backup)
	}
	if err != nil {
		return err
	}
	asar := filepath.Join(betterDiscord, "data", "betterdiscord.asar")
	if err := installBetterDiscord(c.commands, bdcli, resources, asar); err != nil {
		return err
	}
	return retireLegacyDiscordLoader(c, filepath.Dir(resources), asar, location)
}

func (installer packageInstaller) findDiscordExecutable(name string) string {
	return findExecutable(installer.which, name, "",
		filepath.Join(installer.console.home, ".local", "bin", name), filepath.Join("/usr/bin", name))
}

func waitForDiscordResources(c *sshConsole, root string, launch func() error) (string, error) {
	resources, err := discordResources(root)
	if err != nil {
		return "", err
	}
	if resources == "" {
		if err := launch(); err != nil {
			return "", fmt.Errorf("abrir o Discord: %w", err)
		}
		if _, err := c.prompt("Aguarde o Discord inicializar, encerre-o pela bandeja e pressione Enter... "); err != nil {
			return "", err
		}
		resources, err = discordResources(root)
		if err != nil {
			return "", err
		}
		if resources == "" {
			return "", fmt.Errorf("instalação nativa do Discord indisponível em %s após a inicialização", root)
		}
	}
	for {
		_, err := c.commands.capture("", "pgrep", "--euid", strconv.Itoa(os.Geteuid()), "--exact", "Discord|discord")
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			resources, err := discordResources(root)
			if err == nil && resources == "" {
				err = fmt.Errorf("instalação nativa do Discord indisponível em %s", root)
			}
			return resources, err
		}
		if err != nil {
			return "", fmt.Errorf("verificar se o Discord foi encerrado: %w", err)
		}
		if _, err := c.prompt("Encerre o Discord pela bandeja e pressione Enter... "); err != nil {
			return "", err
		}
	}
}

func discordResources(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var selected string
	var latest []int
	for _, entry := range entries {
		version, ok := strings.CutPrefix(entry.Name(), "app-")
		if !ok || !entry.IsDir() {
			continue
		}
		var parts []int
		for _, part := range strings.Split(version, ".") {
			number, err := strconv.Atoi(part)
			if err != nil || strconv.Itoa(number) != part || number < 0 {
				break
			}
			parts = append(parts, number)
		}
		if len(parts) != 3 || strings.Count(version, ".") != 2 || slices.Compare(parts, latest) <= 0 {
			continue
		}
		resources := filepath.Join(root, entry.Name(), "resources")
		for _, name := range []string{"app.asar", "betterdiscord.app.asar"} {
			info, err := os.Lstat(filepath.Join(resources, name))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			if info.Mode().IsRegular() {
				if err := sdkRealDirectoryPath(resources); err != nil {
					return "", err
				}
				selected, latest = resources, parts
				break
			}
		}
	}
	return selected, nil
}

func betterDiscordConfigFiles(source, destination string) ([]configFile, error) {
	var files []configFile
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("arquivo do BetterDiscord não regular: %s", path)
		}
		name, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rest, ok := strings.CutPrefix(name, "data/"); ok {
			name = filepath.Join("data", "stable", rest)
		}
		target := filepath.Join(destination, name)
		if err := validateBackupPath(filepath.Dir(target)); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files = append(files, configFile{target, data, 0600})
		}
		return err
	})
	if err == nil && len(files) == 0 {
		err = errors.New("configuração do BetterDiscord vazia")
	}
	return files, err
}

func installBetterDiscord(commands sshCommands, bdcli, resources, asar string) error {
	if err := validateBackupPath(filepath.Dir(asar)); err != nil {
		return err
	}
	previous := ""
	if info, err := os.Lstat(asar); err == nil {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("arquivo do BetterDiscord inválido: %s", asar)
		}
		staging, err := os.MkdirTemp(filepath.Dir(asar), ".dotfiles-betterdiscord-*")
		if err != nil {
			return err
		}
		defer os.Remove(staging)
		previous = filepath.Join(staging, "previous.asar")
		if err := os.Rename(asar, previous); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err := commands.interactive("env", "-u", "BDCLI_DEV_BUILD", bdcli, "install", "--path", resources)
	if err == nil {
		if info, statErr := os.Lstat(asar); statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			err = fmt.Errorf("a CLI do BetterDiscord não criou um ASAR válido em %s", asar)
		}
	}
	if err != nil {
		if previous != "" {
			if errRestore := os.Rename(previous, asar); errRestore != nil {
				return errors.Join(err, fmt.Errorf("ASAR anterior preservado em %s: %w", previous, errRestore))
			}
		} else if errRemove := os.Remove(asar); errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			return errors.Join(err, errRemove)
		}
		return fmt.Errorf("instalar BetterDiscord no Discord nativo: %w", err)
	}
	if previous != "" {
		return os.Remove(previous)
	}
	return nil
}

func retireLegacyDiscordLoader(c *sshConsole, app, asar string, location backupLocation) error {
	modules := filepath.Join(app, "modules")
	entries, err := os.ReadDir(modules)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	const stock = "module.exports = require('./core.asar');"
	legacy := fmt.Sprintf("require(%q);\n%s", asar, stock)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "discord_desktop_core-") {
			continue
		}
		path := filepath.Join(modules, entry.Name(), "discord_desktop_core", "index.js")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(contents)) != legacy {
			continue
		}
		if err := sdkRealDirectoryPath(filepath.Dir(path)); err != nil {
			return err
		}
		backup, err := applyConfigFiles([]configFile{{path, []byte(stock + "\n"), info.Mode().Perm()}}, location, "discord-core")
		if backup != "" {
			fmt.Fprintln(c.output, "Carregador antigo preservado em", backup)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
