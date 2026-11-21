package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func prepareSystemTrashDirectory() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return prepareTrashDirectory(home, data, sdkMigrationOps{
		copyTree: copySDKTree, rename: os.Rename, symlink: os.Symlink,
	})
}

// Keep the home trash on the home's filesystem even when .local lives elsewhere.
// Callers must close applications using the trash before the first migration.
func prepareTrashDirectory(home, data string, ops sdkMigrationOps) (result error) {
	if !filepath.IsAbs(home) || !filepath.IsAbs(data) {
		return fmt.Errorf("home e XDG_DATA_HOME devem ser absolutos")
	}
	if err := sdkRealDirectoryPath(home); err != nil {
		return err
	}
	target := filepath.Join(home, ".trash")
	// Resolve the data directory, allowing the intentional ~/.local symlink.
	if err := os.MkdirAll(data, 0700); err != nil {
		return err
	}
	data, err := filepath.EvalSymlinks(data)
	if err != nil {
		return err
	}
	source := filepath.Join(data, "Trash")
	if sdkPathWithin(source, target) || sdkPathWithin(target, source) {
		return fmt.Errorf("origem e destino da lixeira não podem se sobrepor")
	}
	homeInfo, err := os.Stat(home)
	if err != nil {
		return err
	}
	for _, path := range []string{target, source} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if path == source && info.Mode()&os.ModeSymlink != 0 {
			// preflight below accepts only the link to ~/.trash.
			continue
		}
		if err := validateTrashDirectory(path); err != nil {
			return err
		}
		if path == target && info.Sys().(*syscall.Stat_t).Dev != homeInfo.Sys().(*syscall.Stat_t).Dev {
			return fmt.Errorf("%s deve estar no mesmo sistema de arquivos do home", target)
		}
	}
	m := sdkMigration{home: source, target: target}
	if err := m.preflight(ops); err != nil {
		return err
	}
	if m.skip {
		return privateTrashDirectories(target)
	}
	if m.hasHome && m.hasTarget {
		if err := checkTrashCollisions(source, target); err != nil {
			return err
		}
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, m.restore(ops))
		}
		for _, path := range []string{m.stage, m.linkDir} {
			if path != "" {
				result = errors.Join(result, forceRemoveAll(path))
			}
		}
	}()
	if err := m.prepare(ops); err != nil {
		return err
	}
	if err := privateTrashDirectories(m.stage); err != nil {
		return err
	}
	return m.activate(ops)
}

func validateTrashDirectory(root string) error {
	for _, path := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "info")} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && path != root {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || int(info.Sys().(*syscall.Stat_t).Uid) != os.Geteuid() {
			return fmt.Errorf("lixeira requer diretório real do usuário atual: %s", path)
		}
	}
	return nil
}

func privateTrashDirectories(root string) error {
	for _, path := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "info")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

// Refuse collisions rather than overwrite a trashed item or its restore metadata.
func checkTrashCollisions(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == source {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		// An orphaned payload or .trashinfo must not be paired with an item
		// from the other trash merely because their names happen to match.
		var counterpart string
		if filepath.Dir(rel) == "files" {
			counterpart = filepath.Join(target, "info", filepath.Base(rel)+".trashinfo")
		} else if filepath.Dir(rel) == "info" && strings.HasSuffix(rel, ".trashinfo") {
			counterpart = filepath.Join(target, "files", strings.TrimSuffix(filepath.Base(rel), ".trashinfo"))
		}
		if counterpart != "" {
			if _, err := os.Lstat(counterpart); err == nil {
				return fmt.Errorf("colisão entre arquivo e metadados da lixeira em %s", rel)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		other, err := os.Lstat(filepath.Join(target, rel))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if (rel == "files" || rel == "info") && entry.IsDir() && other.IsDir() {
			return nil
		}
		return fmt.Errorf("colisão na lixeira em %s; resolva o conflito antes de migrar (nenhum item foi substituído)", rel)
	})
}
