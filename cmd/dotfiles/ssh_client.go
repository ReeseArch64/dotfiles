package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var requiredSSHIdentityFiles = []string{
	"id_trabalho",
	"id_trabalho.pub",
	"id_pessoal",
	"id_pessoal.pub",
	"id_gitlab",
	"id_gitlab.pub",
	"id_github",
	"id_github.pub",
}

func missingSSHIdentityFiles(home string) []string {
	var missing []string
	for _, name := range requiredSSHIdentityFiles {
		info, err := os.Stat(filepath.Join(home, ".ssh", name))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, name)
		}
	}
	return missing
}

func installSSHClientConfig(setupDir, home string, now time.Time) error {
	if missing := missingSSHIdentityFiles(home); len(missing) > 0 {
		return errors.New("arquivos obrigatórios ausentes em ~/.ssh: " + strings.Join(missing, ", "))
	}
	source := configPath(setupDir, "ssh", "config")
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é um arquivo regular", source)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Chmod(sshDir, 0700); err != nil {
		return fmt.Errorf("ajustar permissões de %s: %w", sshDir, err)
	}
	destination := filepath.Join(sshDir, "config")
	if regularFileMatches(source, destination) {
		return os.Chmod(destination, 0600)
	}
	backup := ""
	if _, err := os.Lstat(destination); err == nil {
		backup, err = backupExistingPath(destination, now)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar %s: %w", destination, err)
	}
	if err = copyFileAtomic(source, destination); err == nil {
		err = os.Chmod(destination, 0600)
	}
	if err != nil {
		if backup != "" {
			err = errors.Join(err, restoreLocalBackup(backup, destination))
		} else {
			err = errors.Join(err, os.Remove(destination))
		}
		return fmt.Errorf("copiar configuração SSH: %w", err)
	}
	return nil
}
