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

// The override is a process boundary for isolated runs, not a fallback on errors.
func localBackupRoot() string {
	if root, ok := os.LookupEnv("DOTFILES_BACKUP_DIR"); ok {
		return root
	}
	return backupDestination
}

func validateBackupDirectoryPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("diretório de backup deve ser absoluto: %q", path)
	}
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("backup requer diretório real: %s", ancestor)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	return nil
}

func privateBackupDirectory(path string) error {
	if err := validateBackupDirectoryPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	if err := sdkRealDirectoryPath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("backup requer diretório privado 0700 do usuário atual: %s", path)
	}
	return nil
}

func reserveBackupAt(root, category, origin string, at time.Time) (string, error) {
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
	if sdkPathWithin(root, origin) || sdkPathWithin(origin, root) || sdkPathWithin(root, resolvedOrigin) || sdkPathWithin(resolvedOrigin, root) {
		return "", fmt.Errorf("origem e backup não podem se sobrepor: %s, %s", origin, root)
	}
	parent := root
	for _, component := range []string{"", category, fmt.Sprintf("%x", sha256.Sum256([]byte(origin)))} {
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

func reserveLocalBackup(origin string, at time.Time) (string, error) {
	return reserveBackupAt(localBackupRoot(), "files", origin, at)
}

// A reserved, private container guarantees the destination cannot overwrite a
// previous generation. EXDEV copies retain metadata and links via GNU cp -a.
func moveToBackup(source, backup string, ops sdkMigrationOps) error {
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		return fmt.Errorf("backup já existe ou inacessível: %s: %v", backup, err)
	}
	err := ops.rename(source, backup)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := ops.copyTree(source, backup); err != nil {
		_ = forceRemoveAll(backup)
		return fmt.Errorf("copiar backup (origem intacta em %s): %w", source, err)
	}
	if err := forceRemoveAll(source); err != nil {
		recovery := restoreBackup(backup, source, ops)
		return errors.Join(fmt.Errorf("remover origem %s; backup completo em %s: %w", source, backup, err), recovery)
	}
	return nil
}

func backupExistingPath(source string, at time.Time) (string, error) {
	backup, err := reserveLocalBackup(source, at)
	if err != nil {
		return "", err
	}
	if err := moveToBackup(source, backup, sdkMigrationOps{copyTree: copySDKTree, rename: os.Rename}); err != nil {
		return "", fmt.Errorf("preservar %s (backup em %s): %w", source, backup, err)
	}
	return backup, nil
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
	if err = ops.copyTree(backup, staged); err == nil {
		err = forceRemoveAll(destination)
	}
	if err == nil {
		err = ops.rename(staged, destination)
	}
	if err != nil {
		return fmt.Errorf("restaurar %s; original em %s: %w", destination, backup, err)
	}
	return nil
}

func restoreLocalBackup(backup, destination string) error {
	return restoreBackup(backup, destination, sdkMigrationOps{copyTree: copySDKTree, rename: os.Rename})
}
