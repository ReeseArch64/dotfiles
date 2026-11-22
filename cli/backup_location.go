package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const backupRoot = "/mnt/backups/dotfiles"

type backupLocation struct {
	path string
	sudo func(...string) error
}

func managedBackup(name string, sudo func(...string) error) backupLocation {
	return backupLocation{path: filepath.Join(backupRoot, name), sudo: sudo}
}

func (b backupLocation) prepare() error {
	if filepath.Dir(b.path) != backupRoot {
		return privateBackupDirectory(b.path)
	}
	// The shared parent is controlled by root. Each feature has its own
	// private directory, and privileged writers never traverse user-owned paths.
	for path := backupRoot; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err == nil && (!info.IsDir() || info.Sys().(*syscall.Stat_t).Uid != 0 || info.Mode().Perm()&0022 != 0) {
			return fmt.Errorf("diretório de backup precisa ser controlado pelo root: %s", path)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if path == "/" {
			break
		}
	}
	create := func(path string, mode os.FileMode, uid, gid int) error {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if b.sudo == nil {
			if err := os.MkdirAll(path, mode); err != nil {
				return err
			}
			return os.Chmod(path, mode)
		}
		return b.sudo("sudo", "install", "-d", "-m", fmt.Sprintf("%04o", mode), "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), "--", path)
	}
	if err := create(backupRoot, 0755, 0, 0); err != nil {
		return fmt.Errorf("criar diretório de backups: %w", err)
	}
	if err := create(b.path, 0700, os.Geteuid(), os.Getegid()); err != nil {
		return fmt.Errorf("criar diretório de backup %s: %w", b.path, err)
	}
	return privateBackupDirectory(b.path)
}

func (b backupLocation) saveFile(pattern string, data []byte) (string, error) {
	if err := b.prepare(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(b.path, pattern)
	if err != nil {
		return "", err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if err := errors.Join(err, file.Close()); err != nil {
		return "", errors.Join(err, os.Remove(file.Name()))
	}
	return file.Name(), nil
}

func validateBackupPath(path string) error {
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
	if err := validateBackupPath(path); err != nil {
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
