package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const workspacesRoot = "/mnt/workspaces"

func runWorkspacesSetup(p profile, console sshConsole) error {
	if !p.canMigrateSDK(systemOSID()) || os.Geteuid() == 0 {
		return errors.New("configure /mnt/workspaces pelo menu Desktop ou Laptop no CachyOS como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := configureWorkspaces(home, workspacesRoot, console.commands.interactive); err != nil {
		return err
	}
	fmt.Fprintln(console.output, "~/Projects aponta para", workspacesRoot)
	return nil
}

func configureWorkspaces(home, root string, run func(...string) error) error {
	if !filepath.IsAbs(home) || pathInside(root, home) || pathInside(home, root) {
		return errors.New("a pasta pessoal precisa ser absoluta e separada do diretório de projetos")
	}
	if err := sdkRealDirectoryPath(home); err != nil {
		return err
	}
	if err := validateBackupPath(root); err != nil {
		return err
	}
	link := filepath.Join(home, "Projects")
	unlock, err := lockOperation(link, "configuração de Projects")
	if err != nil {
		return err
	}
	defer unlock()
	info, err := os.Lstat(link)
	exists := err == nil
	if exists {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s já existe. Preserve os dados e libere o caminho antes de criar o link", link)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(home, target)
		}
		if filepath.Clean(target) != root {
			return fmt.Errorf("%s aponta para %s. O destino esperado é %s", link, target, root)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ensureDirectoryRoot(root, 0700, run); err != nil || exists {
		return err
	}
	return os.Symlink(root, link)
}
