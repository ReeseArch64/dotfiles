package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

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
