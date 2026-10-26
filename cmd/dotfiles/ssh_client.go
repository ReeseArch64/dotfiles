package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var gitSSHIdentityFiles = []string{
	"id_gitlab",
	"id_gitlab.pub",
	"id_github",
	"id_github.pub",
}

func requiredSSHIdentityFiles(device deviceProfile) []string {
	var deviceFiles []string
	switch device {
	case deviceDesktop:
		deviceFiles = []string{
			"id_trabalho_desktop", "id_trabalho_desktop.pub",
			"id_pessoal_desktop", "id_pessoal_desktop.pub",
		}
	case deviceNotebook:
		deviceFiles = []string{
			"id_trabalho_notebook", "id_trabalho_notebook.pub",
			"id_trabalho_desktop.pub", "id_pessoal_desktop.pub",
		}
	case deviceServer:
		return []string{"id_pessoal_desktop.pub"}
	default:
		return nil
	}
	return append(deviceFiles, gitSSHIdentityFiles...)
}

func missingSSHIdentityFiles(home string, names []string) []string {
	var missing []string
	for _, name := range names {
		info, err := os.Stat(filepath.Join(home, ".ssh", name))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, name)
		}
	}
	return missing
}

func incomingSSHIdentities(device deviceProfile) ([]string, error) {
	switch device {
	case deviceDesktop:
		return nil, nil
	case deviceNotebook:
		return []string{"id_trabalho_desktop.pub", "id_pessoal_desktop.pub"}, nil
	case deviceServer:
		return []string{"id_pessoal_desktop.pub"}, nil
	default:
		return nil, fmt.Errorf("perfil sem identidades SSH de entrada: %s", device)
	}
}

func publicKeyID(line string) (string, string, bool) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		kind := fields[i]
		if !strings.HasPrefix(kind, "ssh-") && !strings.HasPrefix(kind, "ecdsa-") && !strings.HasPrefix(kind, "sk-") {
			continue
		}
		if decoded, err := base64.StdEncoding.DecodeString(fields[i+1]); err == nil && len(decoded) > 0 {
			return kind, fields[i+1], true
		}
	}
	return "", "", false
}

func writeAuthorizedKeys(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".authorized-keys-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(content)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryName, path)
	}
	return err
}

func authorizeSSHIdentities(home string, publicNames []string, now time.Time) error {
	sshDir := filepath.Join(home, ".ssh")
	lines := make([]string, 0, len(publicNames))
	seen := map[string]bool{}
	for _, publicName := range publicNames {
		publicPath := filepath.Join(sshDir, publicName)
		info, err := os.Lstat(publicPath)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("chave pública obrigatória ausente ou inválida: %s", publicPath)
		}
		publicContent, err := os.ReadFile(publicPath)
		if err != nil {
			return fmt.Errorf("ler %s: %w", publicPath, err)
		}
		publicLine := strings.TrimSpace(string(publicContent))
		if strings.ContainsAny(publicLine, "\r\n") {
			return fmt.Errorf("%s deve conter exatamente uma chave pública", publicPath)
		}
		keyType, keyBlob, valid := publicKeyID(publicLine)
		publicFields := strings.Fields(publicLine)
		if !valid || len(publicFields) < 2 || publicFields[0] != keyType || publicFields[1] != keyBlob {
			return fmt.Errorf("chave pública inválida: %s", publicPath)
		}
		identity := keyType + " " + keyBlob
		if seen[identity] {
			return fmt.Errorf("identidades SSH duplicadas no conjunto permitido: %s", publicName)
		}
		seen[identity] = true
		lines = append(lines, publicLine)
	}
	if err := os.Chmod(sshDir, 0700); err != nil {
		return fmt.Errorf("ajustar permissões de %s: %w", sshDir, err)
	}

	authorizedPath := filepath.Join(sshDir, "authorized_keys")
	var existing []byte
	if info, err := os.Lstat(authorizedPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s não é um arquivo regular", authorizedPath)
		}
		existing, err = os.ReadFile(authorizedPath)
		if err != nil {
			return fmt.Errorf("ler %s: %w", authorizedPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar %s: %w", authorizedPath, err)
	}

	var content []byte
	if len(lines) > 0 {
		content = []byte(strings.Join(lines, "\n") + "\n")
	}
	if string(existing) == string(content) {
		return os.Chmod(authorizedPath, 0600)
	}

	if len(existing) > 0 {
		backup, err := backupExistingPath(authorizedPath, now)
		if err != nil {
			return err
		}
		if err = writeAuthorizedKeys(authorizedPath, content); err != nil {
			err = errors.Join(err, restoreLocalBackup(backup, authorizedPath))
			return fmt.Errorf("atualizar %s: %w", authorizedPath, err)
		}
		return nil
	}
	if err := writeAuthorizedKeys(authorizedPath, content); err != nil {
		return fmt.Errorf("atualizar %s: %w", authorizedPath, err)
	}
	return nil
}

func installSSHClientConfig(setupDir, home string, device deviceProfile, now time.Time) error {
	if missing := missingSSHIdentityFiles(home, requiredSSHIdentityFiles(device)); len(missing) > 0 {
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
