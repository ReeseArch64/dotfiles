package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type homelabBackup struct {
	Contents     []byte
	ResolverLink string
}

func homelabBackupAsUser(ctx context.Context, account *user.User, category string) func(homelabBackup) (string, error) {
	return func(backup homelabBackup) (string, error) {
		data, err := json.Marshal(backup)
		if err != nil {
			return "", err
		}
		return saveBackupAsUser(ctx, account, data, "--homelab", "save-backup", category)
	}
}

func writeHomelabBackup(category string) error {
	if os.Geteuid() == 0 {
		return errors.New("execute o backup como usuário normal")
	}
	var filename string
	switch category {
	case "DNS":
		filename = "resolved.conf"
	case "Swap":
		filename = "fstab"
	default:
		return errors.New("categoria de backup inválida")
	}
	var backup homelabBackup
	if err := json.NewDecoder(os.Stdin).Decode(&backup); err != nil {
		return err
	}
	location := filepath.Join(backupRoot, category)
	if err := privateBackupDirectory(location); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(location, strings.ToLower(category)+"-*")
	if err != nil {
		return err
	}
	if err := writeConfigAtomic(filepath.Join(directory, filename), backup.Contents, 0600); err != nil {
		return errors.Join(err, os.Remove(directory))
	}
	if category == "DNS" && backup.ResolverLink != "" {
		if err := os.Symlink(backup.ResolverLink, filepath.Join(directory, "resolv.conf")); err != nil {
			return errors.Join(err, os.RemoveAll(directory))
		}
	}
	fmt.Println(directory)
	return nil
}

func readHomelabSystemFile(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("o caminho precisa ser um arquivo regular: %s", path)
	}
	contents, err := os.ReadFile(path)
	return contents, info.Mode().Perm(), err
}
