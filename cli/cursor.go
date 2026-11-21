package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const cursorTheme = "FrierenBLZ"

var miscInclude = regexp.MustCompile(`(?m)^[\t ]*include[\t ]+"(?:\./)?cfg/misc\.kdl"[\t ;]*(?://[^\n]*)?$`)

func runCursorAction(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a configuração do cursor pelo menu Desktop ou Laptop como usuário normal")
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

	printActionHeader()
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	result, err := installCursor(assets, config, home, func(candidate string) error {
		_, err := console.commands.capture("", "niri", "validate", "--config", candidate)
		return err
	})
	if err != nil {
		fmt.Fprintln(console.output, err)
	} else {
		fmt.Fprintln(console.output, "Cursor FrierenBLZ configurado no Niri.")
		for _, backup := range result {
			fmt.Fprintln(console.output, "Backup:", backup)
		}
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func installCursor(assets, config, home string, validate func(string) error) (backups []string, result error) {
	// Hold the lock through validation and the replacement of both assets.
	configDir := filepath.Dir(config)
	unlock, err := lockOperation(filepath.Join(configDir, ".dotfiles-cursor.lock"), "cursor")
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
	themeBackup, undo, err := installCursorTheme(themeSource, themeDestination)
	if err != nil {
		return nil, err
	}
	if themeBackup != "" {
		backups = append(backups, themeBackup)
	}
	if undo != nil {
		defer func() {
			if result != nil {
				result = errors.Join(result, undo())
			}
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
		backup, err := os.CreateTemp(filepath.Dir(destination), "misc.kdl.backup-*")
		if err != nil {
			return nil, err
		}
		_, writeErr := backup.Write(original)
		if err := errors.Join(writeErr, backup.Close()); err != nil {
			return nil, err
		}
		backups = append(backups, backup.Name())
	}
	return backups, writeConfigAtomic(destination, misc, mode)
}

func installCursorTheme(source, target string) (string, func() error, error) {
	for _, file := range []string{"index.theme", "cursors/default"} {
		if err := sshRegular(filepath.Join(source, file), false); err != nil {
			return "", nil, fmt.Errorf("tema %s incompleto: %w", cursorTheme, err)
		}
	}
	if sameCursorTree(source, target) {
		return "", nil, nil
	}
	info, err := os.Lstat(target)
	existed := err == nil
	if (err == nil && !info.IsDir()) || (err != nil && !errors.Is(err, os.ErrNotExist)) {
		return "", nil, fmt.Errorf("o destino do tema precisa ser um diretório real: %s", target)
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", nil, err
	}
	staging, err := os.MkdirTemp(parent, ".dotfiles-theme-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(staging)
	candidate := filepath.Join(staging, cursorTheme)
	if err := os.CopyFS(candidate, os.DirFS(source)); err != nil {
		return "", nil, err
	}
	var backup, backupDir string
	if existed {
		backupDir, err = os.MkdirTemp(parent, cursorTheme+".backup-*")
		if err != nil {
			return "", nil, err
		}
		backup = filepath.Join(backupDir, cursorTheme)
		if err := os.Rename(target, backup); err != nil {
			_ = os.Remove(backupDir)
			return "", nil, err
		}
	}
	restore := func() error {
		if backup != "" {
			if err := os.Rename(backup, target); err != nil {
				return err
			}
			return os.Remove(backupDir)
		}
		return nil
	}
	if err := os.Rename(candidate, target); err != nil {
		return "", nil, errors.Join(err, restore())
	}
	return backup, func() error {
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		return restore()
	}, nil
}

func sameCursorTree(source, target string) bool {
	different := errors.New("temas diferentes")
	count := 0
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		relative, _ := filepath.Rel(source, path)
		other := filepath.Join(target, relative)
		info, err := os.Lstat(other)
		if err != nil || entry.Type() != info.Mode().Type() {
			return different
		}
		if entry.Type().IsRegular() {
			a, firstErr := os.ReadFile(path)
			b, secondErr := os.ReadFile(other)
			if firstErr != nil || secondErr != nil || !bytes.Equal(a, b) {
				return different
			}
		}
		return nil
	})
	if err != nil {
		return false
	}
	err = filepath.WalkDir(target, func(_ string, _ fs.DirEntry, err error) error {
		count--
		return err
	})
	return err == nil && count == 0
}
