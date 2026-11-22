package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type configFile struct {
	path     string
	contents []byte
	mode     os.FileMode
}

type stagedConfigFile struct {
	target, staged, original string
	previous, replacement    os.FileInfo
}

func applyConfigFiles(files []configFile, backup backupLocation, prefix string) (backupDir string, result error) {
	var staged []stagedConfigFile
	for _, file := range files {
		info, err := os.Lstat(file.path)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return "", err
		case info.Mode().IsRegular():
			current, err := os.ReadFile(file.path)
			if err != nil {
				return "", err
			}
			if bytes.Equal(current, file.contents) && info.Mode().Perm() == file.mode {
				continue
			}
		case info.Mode()&os.ModeSymlink != 0:
			// Replace the legacy link itself and preserve it in the backup.
		default:
			return "", fmt.Errorf("destino da configuração não é arquivo regular nem link: %s", file.path)
		}
		dir := filepath.Dir(file.path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
		work, err := os.MkdirTemp(dir, ".dotfiles-"+prefix+"-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(work)
		candidate := filepath.Join(work, "candidate")
		if err := writeConfigAtomic(candidate, file.contents, file.mode); err != nil {
			return "", err
		}
		replacement, err := os.Stat(candidate)
		if err != nil {
			return "", err
		}
		staged = append(staged, stagedConfigFile{target: file.path, staged: candidate, previous: info, replacement: replacement})
	}

	for i := range staged {
		file := &staged[i]
		if file.previous == nil {
			continue
		}
		if backupDir == "" {
			if err := backup.prepare(); err != nil {
				return "", err
			}
			backupDir, result = os.MkdirTemp(backup.path, prefix+"-*")
			if result != nil {
				return "", result
			}
		}
		file.original = filepath.Join(backupDir, filepath.Base(file.target))
		if err := copySDKTree(context.Background(), file.target, file.original); err != nil {
			return backupDir, err
		}
	}

	applied := 0
	defer func() {
		if result != nil {
			for _, file := range slices.Backward(staged[:applied]) {
				result = errors.Join(result, file.restore())
			}
		}
	}()
	for _, file := range staged {
		current, err := os.Lstat(file.target)
		switch {
		case file.previous == nil && errors.Is(err, os.ErrNotExist):
		case err == nil && file.previous != nil && os.SameFile(file.previous, current) &&
			file.previous.Mode() == current.Mode() && file.previous.Size() == current.Size() && file.previous.ModTime().Equal(current.ModTime()):
		default:
			return backupDir, fmt.Errorf("arquivo de configuração mudou durante a preparação: %s", file.target)
		}
		if err := os.Rename(file.staged, file.target); err != nil {
			return backupDir, err
		}
		applied++
	}
	return backupDir, nil
}

func (file stagedConfigFile) restore() error {
	current, err := os.Lstat(file.target)
	if err != nil || !os.SameFile(file.replacement, current) {
		return fmt.Errorf("arquivo de configuração mudou durante a restauração: %s", file.target)
	}
	if file.original == "" {
		return os.Remove(file.target)
	}
	if err := copySDKTree(context.Background(), file.original, file.staged); err != nil {
		return err
	}
	return os.Rename(file.staged, file.target)
}
