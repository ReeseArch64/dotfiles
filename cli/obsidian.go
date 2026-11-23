package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func setupObsidian(c *sshConsole) error {
	destination := filepath.Join(c.home, ".obsidian")
	unlock, err := lockOperation(destination, "configuração do Obsidian")
	if err != nil {
		return err
	}
	defer unlock()
	backup, err := applyObsidian(filepath.Dir(cachyosAsset(c.home, "obsidian", "appearance.json")), destination,
		managedBackup("Obsidian", c.commands.interactive))
	if backup != "" {
		fmt.Fprintln(c.output, "Backup do Obsidian em", backup)
	}
	return err
}

func applyObsidian(source, destination string, location backupLocation) (string, error) {
	if err := validateBackupPath(filepath.Join(destination, "snippets")); err != nil {
		return "", err
	}
	var files []configFile
	for _, name := range []string{"appearance.json", "snippets/noctalia.css"} {
		contents, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return "", err
		}
		files = append(files, configFile{filepath.Join(destination, name), contents, 0644})
	}
	return applyConfigFiles(files, location, "obsidian")
}
