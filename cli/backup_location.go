package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	backupMount = "/mnt/backups"
	backupRoot  = backupMount + "/dotfiles"
)

type backupLocation struct {
	path string
	base string
	sudo func(...string) error
}

func managedBackup(name string, sudo func(...string) error) backupLocation {
	return backupLocation{path: filepath.Join(backupRoot, name), base: backupMount, sudo: sudo}
}

func (b backupLocation) prepare() error {
	if b.base == "" {
		return privateBackupDirectory(b.path)
	}
	base, target := filepath.Clean(b.base), filepath.Clean(b.path)
	if !filepath.IsAbs(base) || !pathInside(base, target) || target == base {
		return fmt.Errorf("destino de backup fora de %s: %s", b.base, b.path)
	}
	if err := validateBackupPath(filepath.Dir(base)); err != nil {
		return err
	}
	var parents []string
	for path := filepath.Dir(target); ; path = filepath.Dir(path) {
		parents = append(parents, path)
		if path == base {
			break
		}
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if err := b.prepareDirectory(parents[i], 0755); err != nil {
			return err
		}
	}
	return b.prepareDirectory(target, 0700)
}

func (b backupLocation) prepareDirectory(path string, mode os.FileMode) error {
	uid, gid := os.Geteuid(), os.Getegid()
	repairMode := mode
	check := func() (ready, owned bool, err error) {
		info, err := os.Lstat(path)
		if err != nil {
			return false, false, err
		}
		if !info.IsDir() {
			return false, false, fmt.Errorf("backup requer diretório real: %s", path)
		}
		owned = int(info.Sys().(*syscall.Stat_t).Uid) == uid
		permissions := info.Mode().Perm()
		if mode == 0755 {
			repairMode = permissions | 0700
		}
		ready = owned && permissions == repairMode
		return ready, owned, nil
	}
	ready, owned, err := check()
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(path, mode); err == nil || errors.Is(err, os.ErrExist) {
			ready, owned, err = check()
		}
	}
	if ready {
		return nil
	}
	if owned {
		err = os.Chmod(path, repairMode)
	} else if b.sudo != nil && (err == nil || errors.Is(err, os.ErrPermission)) {
		err = b.sudo("sudo", "install", "-d", "-m", fmt.Sprintf("%04o", repairMode), "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), "--", path)
	}
	if err == nil {
		ready, _, err = check()
		if ready {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("preparar permissões de backup em %s: %w", path, err)
	}
	return fmt.Errorf("não foi possível aplicar as permissões de backup em %s", path)
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
