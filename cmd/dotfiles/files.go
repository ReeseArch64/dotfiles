package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Only workstation profiles inherit the shared configuration. Server and other
// profiles retain their original, isolated paths.
func sharedConfigDirectory(setupDir string) string {
	clean := filepath.Clean(setupDir)
	profile := filepath.Base(clean)
	if filepath.Base(filepath.Dir(clean)) != "setup" || (profile != "desktop" && profile != "laptop") {
		return ""
	}
	return filepath.Join(filepath.Dir(clean), "config")
}

func configPath(setupDir string, elements ...string) string {
	local := filepath.Join(append([]string{setupDir}, elements...)...)
	// This file is updated by GPG and must never resolve to a shared source.
	if filepath.Join(elements...) == filepath.Join("git", ".gitconfig") {
		return local
	}
	if _, err := os.Lstat(local); !os.IsNotExist(err) {
		return local
	}
	if shared := sharedConfigDirectory(setupDir); shared != "" && len(elements) > 0 {
		candidate := filepath.Join(append([]string{shared}, elements...)...)
		if _, err := os.Lstat(candidate); err == nil {
			return candidate
		}
	}
	return local
}

// A directory may contain both shared files and profile overrides. Materialize
// that union before comparing or installing it, never mutating either source.
func materializeConfigDirectory(setupDir string, elements ...string) (string, func(), error) {
	local := filepath.Join(append([]string{setupDir}, elements...)...)
	var sources []string
	if shared := sharedConfigDirectory(setupDir); shared != "" {
		candidate := filepath.Join(append([]string{shared}, elements...)...)
		if _, err := os.Lstat(candidate); err == nil {
			sources = append(sources, candidate)
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	if _, err := os.Lstat(local); err == nil {
		sources = append(sources, local)
	} else if !os.IsNotExist(err) {
		return "", nil, err
	}
	if len(sources) == 0 {
		return "", nil, fmt.Errorf("configuração ausente: %s", local)
	}
	if len(sources) == 1 {
		return sources[0], func() {}, nil
	}
	container, err := os.MkdirTemp("", "dotfiles-config-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(container) }
	merged := filepath.Join(container, "config")
	for _, source := range sources {
		if err := copyDirectory(source, merged); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("compor %s: %w", source, err)
		}
	}
	return merged, cleanup, nil
}

func copyConfigDirectoryWithBackup(setupDir, destination string, now time.Time, elements ...string) error {
	source, cleanup, err := materializeConfigDirectory(setupDir, elements...)
	if err != nil {
		return err
	}
	defer cleanup()
	return copyDirectoryWithBackup(source, destination, now)
}

func assetPath(dotfiles string, elements ...string) string {
	return filepath.Join(append([]string{dotfiles, "assets"}, elements...)...)
}

func regularFileMatches(source, destination string) bool {
	destinationInfo, err := os.Lstat(destination)
	if err != nil || !destinationInfo.Mode().IsRegular() {
		return false
	}
	sourceContent, err := os.ReadFile(source)
	if err != nil {
		return false
	}
	destinationContent, err := os.ReadFile(destination)
	return err == nil && bytes.Equal(sourceContent, destinationContent)
}

func copyFileAtomic(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é um arquivo regular", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("criar diretório de %s: %w", destination, err)
	}
	output, err := os.CreateTemp(filepath.Dir(destination), ".copy-*")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	tmpName := output.Name()
	defer os.Remove(tmpName)
	if _, err = io.Copy(output, input); err == nil {
		err = output.Chmod(info.Mode().Perm())
	}
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("copiar %s: %w", source, err)
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("substituir %s: %w", destination, err)
	}
	return nil
}

func directoriesEqual(source, destination string) bool {
	destinationInfo, err := os.Lstat(destination)
	if err != nil || !destinationInfo.IsDir() || destinationInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	count := 0
	equal := true
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		count++
		other := filepath.Join(destination, relative)
		info, err := os.Lstat(other)
		if err != nil {
			equal = false
			return nil
		}
		if entry.IsDir() != info.IsDir() || entry.Type()&os.ModeSymlink != info.Mode()&os.ModeSymlink {
			equal = false
			return nil
		}
		if entry.Type().IsRegular() && !regularFileMatches(path, other) {
			equal = false
		}
		return nil
	})
	if err != nil || !equal {
		return false
	}
	destinationCount := 0
	if filepath.WalkDir(destination, func(_ string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		destinationCount++
		return nil
	}) != nil {
		return false
	}
	return count == destinationCount
}

func copyDirectoryWithBackup(source, destination string, now time.Time) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolver %s: %w", source, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", source)
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("criar %s: %w", parent, err)
	}
	if directoriesEqual(source, destination) {
		return nil
	}
	staging, err := os.MkdirTemp(parent, ".directory-copy-*")
	if err != nil {
		return fmt.Errorf("criar diretório temporário: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, info.Mode().Perm()); err != nil {
		return fmt.Errorf("ajustar permissões temporárias: %w", err)
	}
	if err := copyDirectory(source, staging); err != nil {
		return fmt.Errorf("copiar %s: %w", source, err)
	}
	var backup string
	if _, err := os.Lstat(destination); err == nil {
		backup, err = backupExistingPath(destination, now)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar %s: %w", destination, err)
	}
	if err := os.Rename(staging, destination); err != nil {
		if backup != "" {
			err = errors.Join(err, restoreLocalBackup(backup, destination))
		}
		return fmt.Errorf("ativar configuração em %s: %w", destination, err)
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case entry.Type().IsRegular():
			return copyFileAtomic(path, target)
		default:
			return fmt.Errorf("tipo de arquivo não suportado: %s", path)
		}
	})
}
