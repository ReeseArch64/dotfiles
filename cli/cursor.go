package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

const cursorTheme = "FrierenBLZ"

var miscInclude = regexp.MustCompile(`(?m)^[\t ]*include[\t ]+"(?:\./)?cfg/misc\.kdl"[\t ;]*(?://[^\n]*)?$`)

func setupCursor(p profile, output io.Writer, commands sshCommands) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a configuração do cursor pelo menu Desktop ou Laptop como usuário normal")
	}
	if _, err := exec.LookPath("niri"); err != nil {
		return errors.New("instale o Niri antes de aplicar a configuração do cursor")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config := os.Getenv("NIRI_CONFIG")
	if config == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		config = filepath.Join(root, "niri", "config.kdl")
	}
	config, err = filepath.Abs(config)
	if err != nil {
		return err
	}

	assets := filepath.Join(home, ".dotfiles", "cachyos")
	if binary, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(filepath.Dir(binary)), "cachyos")
		if _, err := os.Stat(filepath.Join(local, "niri", "cfg", "misc.kdl")); err == nil {
			assets = local
		}
	}

	result, err := installCursor(assets, config, home, managedBackup("Niri", commands.interactive), func(candidate string) error {
		_, err := commands.capture("", "niri", "validate", "--config", candidate)
		return err
	})
	for _, backup := range result {
		fmt.Fprintln(output, "Backup:", backup)
	}
	return err
}

func installCursor(assets, config, home string, location backupLocation, validate func(string) error) (backups []string, result error) {
	// Hold the lock through validation and the replacement of both assets.
	configDir := filepath.Dir(config)
	unlock, err := lockOperation(configDir, "cursor")
	if err != nil {
		return nil, fmt.Errorf("abrir configuração do Niri: %w", err)
	}
	defer unlock()

	root, err := os.ReadFile(config)
	if err != nil {
		return nil, fmt.Errorf("ler config.kdl do Niri: %w", err)
	}
	includes := miscInclude.FindAllIndex(root, 2)
	if len(includes) != 1 {
		return nil, errors.New("o config.kdl do Niri precisa incluir cfg/misc.kdl uma vez")
	}
	source := filepath.Join(assets, "niri", "cfg", "misc.kdl")
	misc, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("ler misc.kdl do Dotfiles: %w", err)
	}
	destination := filepath.Join(configDir, "cfg", "misc.kdl")
	if err := sshRegular(destination, true); err != nil {
		return nil, err
	}
	original, err := os.ReadFile(destination)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Sibling config keeps the base directory of every other Niri include.
	match := includes[0]
	candidate := append(bytes.Clone(root[:match[0]]), []byte("include "+strconv.Quote(source))...)
	candidate = append(candidate, root[match[1]:]...)
	file, err := os.CreateTemp(configDir, ".dotfiles-cursor-*.kdl")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(candidate)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return nil, err
	}
	if err := validate(file.Name()); err != nil {
		return nil, fmt.Errorf("a configuração candidata do Niri não passou na validação: %w", err)
	}

	themeSource := filepath.Join(assets, "cursor", cursorTheme)
	themeDestination := filepath.Join(home, ".icons", cursorTheme)
	themeBackup, finish, err := installCursorTheme(themeSource, themeDestination, location)
	if err != nil {
		return nil, err
	}
	if themeBackup != "" {
		backups = append(backups, themeBackup)
	}
	if finish != nil {
		defer func() {
			result = errors.Join(result, finish(result != nil))
		}()
	}
	if existed && bytes.Equal(original, misc) {
		return backups, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return nil, err
	}
	mode := os.FileMode(0644)
	if existed {
		info, err := os.Stat(destination)
		if err != nil {
			return nil, err
		}
		mode = info.Mode().Perm()
		backup, err := location.saveFile("misc.kdl.backup-*", original)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, writeConfigAtomic(destination, misc, mode)
}

func installCursorTheme(source, target string, location backupLocation) (string, func(bool) error, error) {
	for _, file := range []string{"index.theme", "cursors/default"} {
		if err := sshRegular(filepath.Join(source, file), false); err != nil {
			return "", nil, fmt.Errorf("tema %s incompleto: %w", cursorTheme, err)
		}
	}
	info, err := os.Lstat(target)
	if (err == nil && !info.IsDir()) || (err != nil && !errors.Is(err, os.ErrNotExist)) {
		return "", nil, fmt.Errorf("o destino do tema precisa ser um diretório real: %s", target)
	}
	return installAssetDirectory(source, target, cursorTheme, location)
}
