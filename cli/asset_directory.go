package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func installAssetDirectory(source, target, name string, location backupLocation) (string, func(bool) error, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("o caminho precisa ser um diretório: %s", source)
	}
	if sameAssetTree(source, target) {
		return "", nil, nil
	}
	_, err = os.Lstat(target)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", nil, err
	}
	staging, err := os.MkdirTemp(parent, ".dotfiles-assets-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(staging)
	candidate := filepath.Join(staging, name)
	if err := os.CopyFS(candidate, os.DirFS(source)); err != nil {
		return "", nil, err
	}
	var backup, retired string
	ops := systemSDKOps(context.Background(), location.path)
	if existed {
		if err := location.prepare(); err != nil {
			return "", nil, err
		}
		backupDir, err := os.MkdirTemp(location.path, name+".backup-*")
		if err != nil {
			return "", nil, err
		}
		backup = filepath.Join(backupDir, name)
		retired, err = moveToBackup(target, backup, ops)
		if err != nil {
			_ = os.Remove(backupDir)
			return "", nil, err
		}
	}
	restore := func() error {
		if backup != "" {
			return restoreSDKOriginal(backup, retired, target, ops)
		}
		return nil
	}
	if err := ops.rename(candidate, target); err != nil {
		return "", nil, errors.Join(err, restore())
	}
	return backup, func(rollback bool) error {
		if !rollback {
			if retired != "" {
				return forceRemoveAll(filepath.Dir(retired))
			}
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		return restore()
	}, nil
}

func sameAssetTree(source, target string) bool {
	different := errors.New("diretórios diferentes")
	count := 0
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		relative, _ := filepath.Rel(source, path)
		other := filepath.Join(target, relative)
		info, err := os.Lstat(other)
		if err != nil || entry.Type() != info.Mode().Type() {
			return different
		}
		if entry.Type().IsRegular() {
			a, firstErr := os.ReadFile(path)
			b, secondErr := os.ReadFile(other)
			if firstErr != nil || secondErr != nil || !bytes.Equal(a, b) {
				return different
			}
		}
		return nil
	})
	if err != nil {
		return false
	}
	err = filepath.WalkDir(target, func(_ string, _ fs.DirEntry, err error) error {
		count--
		return err
	})
	return err == nil && count == 0
}
