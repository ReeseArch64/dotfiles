package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var requiredSSHIdentityFiles = []string{
	"id_github_reesearch64",
	"id_github_reesearch64.pub",
	"id_gitlab_reesearch64",
	"id_gitlab_reesearch64.pub",
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

func installSSHClientConfig(dotfiles, home string, now time.Time) error {
	if missing := missingSSHIdentityFiles(home); len(missing) > 0 {
		return errors.New("arquivos obrigatórios ausentes em ~/.ssh: " + strings.Join(missing, ", "))
	}
	source := configPath(dotfiles, "ssh", "config")
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
		backup = destination + ".backup-" + now.Format("20060102-150405")
		if _, err := os.Lstat(backup); err == nil {
			return fmt.Errorf("backup já existe: %s", backup)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", backup, err)
		}
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("preservar configuração atual em %s: %w", backup, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar %s: %w", destination, err)
	}
	if err := copyFileAtomic(source, destination); err == nil {
		err = os.Chmod(destination, 0600)
	}
	if err != nil {
		_ = os.Remove(destination)
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("copiar configuração SSH: %w", err)
	}
	return nil
}

func sshClientConfigJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return job{
		title: "Configurar cliente SSH",
		steps: []step{nativeStep("Validar chaves e copiar ~/.ssh/config", func() error {
			return installSSHClientConfig(dotfiles, home, time.Now())
		})},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para ~/.ssh/config")
		},
	}
}
