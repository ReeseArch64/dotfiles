package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func runWallpapersAction(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("copie os papéis de parede pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	source := dotfilesAsset(home, "cachyos", "wallpapers")
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	printActionHeader()
	backup, err := installWallpapers(source, home, managedBackup("Wallpapers", console.commands.interactive))
	if backup != "" {
		fmt.Fprintln(console.output, "Backup:", backup)
	}
	if err != nil {
		fmt.Fprintln(console.output, err)
	} else {
		fmt.Fprintln(console.output, "Papéis de parede disponíveis em", filepath.Join(home, ".wallpapers"))
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func installWallpapers(source, home string, location backupLocation) (string, error) {
	unlock, err := lockOperation(filepath.Join(home, ".dotfiles-wallpapers.lock"), "papéis de parede")
	if err != nil {
		return "", err
	}
	defer unlock()
	backup, finish, err := installAssetDirectory(source, filepath.Join(home, ".wallpapers"), "wallpapers", location)
	if err == nil && finish != nil {
		err = finish(false)
	}
	return backup, err
}
