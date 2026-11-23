package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func reserveSDKBackup(root, origin string, at time.Time) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("diretório de backup deve ser absoluto: %q", root)
	}
	origin, err := filepath.Abs(origin)
	if err != nil {
		return "", err
	}
	// Resolve ancestors, but not the final component: a backed-up symlink is
	// an object in its own right, not a request to copy its referent.
	resolvedOrigin := origin
	if parent, err := filepath.EvalSymlinks(filepath.Dir(origin)); err == nil {
		resolvedOrigin = filepath.Join(parent, filepath.Base(origin))
	}
	if pathInside(origin, root) || pathInside(root, origin) || pathInside(resolvedOrigin, root) || pathInside(root, resolvedOrigin) {
		return "", fmt.Errorf("origem e backup não podem se sobrepor: %s, %s", origin, root)
	}
	parent := root
	for _, component := range []string{"", fmt.Sprintf("%x", sha256.Sum256([]byte(origin)))} {
		parent = filepath.Join(parent, component)
		if err := privateBackupDirectory(parent); err != nil {
			return "", fmt.Errorf("preparar backup: %w", err)
		}
	}
	directory, err := os.MkdirTemp(parent, at.UTC().Format("20060102T150405.000000000Z")+"-*")
	if err != nil {
		return "", err
	}
	metadata, err := json.Marshal(struct {
		Origin  string `json:"origin"`
		Created string `json:"created_utc"`
	}{origin, at.UTC().Format(time.RFC3339Nano)})
	if err == nil {
		err = os.WriteFile(filepath.Join(directory, "origin.json"), append(metadata, '\n'), 0600)
	}
	if err != nil {
		_ = os.RemoveAll(directory)
		return "", err
	}
	return filepath.Join(directory, "original"), nil
}

// A reserved, private container guarantees the destination cannot overwrite a
// previous generation. EXDEV copies retain metadata and links via GNU cp -a.
func moveToBackup(source, backup string, ops sdkMigrationOps) (string, error) {
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		return "", fmt.Errorf("backup já existe ou inacessível: %s: %v", backup, err)
	}
	err := ops.rename(source, backup)
	if err == nil {
		return "", nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return "", err
	}
	if ops.archiveHomeOnly {
		// Stop writes via the old HOME path before copying to a different filesystem.
		retired, err := retireDirectory(source, ops)
		if err != nil {
			return "", err
		}
		if err := ops.copyTree(retired, backup); err != nil {
			return retired, fmt.Errorf("copiar backup de %s: %w", source, errors.Join(err, forceRemoveAll(backup)))
		}
		return retired, nil
	}
	if err := ops.copyTree(source, backup); err != nil {
		_ = forceRemoveAll(backup)
		return "", fmt.Errorf("copiar backup (origem intacta em %s): %w", source, err)
	}
	return retireDirectory(source, ops)
}

func retireDirectory(source string, ops sdkMigrationOps) (string, error) {
	container, err := os.MkdirTemp(filepath.Dir(source), ".dotfiles-original-*")
	if err != nil {
		return "", err
	}
	retired := filepath.Join(container, "original")
	if err := ops.rename(source, retired); err != nil {
		_ = os.Remove(container)
		return "", err
	}
	return retired, nil
}

func restoreSDKOriginal(backup, retired, destination string, ops sdkMigrationOps) error {
	if retired == "" {
		return restoreBackup(backup, destination, ops)
	}
	if err := ops.rename(retired, destination); err != nil {
		return fmt.Errorf("restaurar %s; original preservado em %s: %w", destination, retired, err)
	}
	return os.Remove(filepath.Dir(retired))
}

// Copy before touching the destination; never consume the last good central
// original, including when staging, removal, or final activation fails.
func restoreBackup(backup, destination string, ops sdkMigrationOps) error {
	container, err := os.MkdirTemp(filepath.Dir(destination), ".backup-restore-*")
	if err != nil {
		return fmt.Errorf("restaurar %s; original em %s: %w", destination, backup, err)
	}
	defer forceRemoveAll(container)
	staged := filepath.Join(container, "original")
	err = ops.copyTree(backup, staged)
	if err == nil {
		err = ops.rename(staged, destination)
	}
	if err != nil {
		return fmt.Errorf("restaurar %s; original em %s: %w", destination, backup, err)
	}
	return nil
}
