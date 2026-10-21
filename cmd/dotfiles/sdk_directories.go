package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type sdkMigrationOps struct {
	copyTree          func(string, string) error
	repairPermissions func(string) error
	rename            func(string, string) error
	symlink           func(string, string) error
}

type sdkMigration struct {
	home, target, stage, homeBackup, targetBackup, linkDir string
	hasHome, hasTarget, skip, existingLink                 bool
	homeMoved, targetMoved, targetInstalled, linkInstalled bool
}

func prepareSystemSDKDirectories() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return prepareSDKDirectories(home, "/mnt/sdk", sdkMigrationOps{
		copyTree:          copySDKTree,
		repairPermissions: repairSystemMigrationPermissions,
		rename:            os.Rename,
		symlink:           os.Symlink,
	})
}

func copySDKTree(source, target string) error {
	cmd := exec.Command("cp", "-a", "--no-target-directory", "--", source, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp -a %s → %s: %w: %s", source, target, err, output)
	}
	return nil
}

func prepareSDKDirectories(home, root string, ops sdkMigrationOps) error {
	return prepareHomeDirectoryLinks(home, root, ops, sdkDirectoryMappings)
}

func prepareHomeDirectoryLinks(home, root string, ops sdkMigrationOps, mappings [][2]string) error {
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, path := range []string{home, root} {
		if err := sdkRealDirectoryPath(path); err != nil {
			return err
		}
	}
	if sdkPathWithin(home, root) || sdkPathWithin(root, home) {
		return fmt.Errorf("HOME e destino não podem se sobrepor: %s, %s", home, root)
	}

	migrations := make([]sdkMigration, 0, len(mappings))
	for _, mapping := range mappings {
		migrations = append(migrations, sdkMigration{home: filepath.Join(home, mapping[0]), target: filepath.Join(root, mapping[1])})
	}
	return runDirectoryMigrations(migrations, ops)
}

var sdkDirectoryMappings = [][2]string{
	{".aur", "Aur"}, {".local", "Local"}, {".cache", "Cache"}, {"Android", "Android"}, {".gradle", "Gradle"},
	{".bun", "Bun"}, {".pi", "Pi"}, {".codex", "Codex"}, {".npm", "Npm"}, {".rustup", "Rustup"},
	{".agent-browser", "AgentBrowser"}, {".cargo", "Cargo"}, {".expo", "Expo"}, {".vscode", "Vscode"},
	{".vite-plus", "VitePlus"}, {"go", "Go"}, {".docker", "Docker"}, {".dotnet", "Dotnet"}, {".java", "Java"},
	{".copilot", "Copilot"}, {".azure", "Azure"}, {".aws", "Aws"}, {".gemini", "Gemini"}, {".railway", "Railway"}, {".tauri", "Tauri"},
}

func runDirectoryMigrations(migrations []sdkMigration, ops sdkMigrationOps) (result error) {
	// Preflight every mapping before creating staging directories or backups.
	for i := range migrations {
		if err := migrations[i].preflight(ops); err != nil {
			return err
		}
	}
	defer func() {
		if result != nil {
			for i := len(migrations) - 1; i >= 0; i-- {
				result = errors.Join(result, migrations[i].restore(ops))
			}
		}
		for _, m := range migrations {
			// Only disposable copies and temporary links are recursively removed.
			for _, path := range []string{m.stage, m.linkDir} {
				if path != "" {
					result = errors.Join(result, os.RemoveAll(path))
				}
			}
		}
	}()

	// Finish every merged copy before any original is moved or link activated.
	for i := range migrations {
		if err := migrations[i].prepare(ops); err != nil {
			return err
		}
	}
	for i := range migrations {
		if err := migrations[i].activate(ops); err != nil {
			return err
		}
	}
	return nil
}

func sdkPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func sdkRealDirectoryPath(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("verificar %s: %w", path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s deve ser um diretório real, sem links simbólicos", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func (m *sdkMigration) preflight(ops sdkMigrationOps) error {
	info, err := os.Lstat(m.target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	m.hasTarget = err == nil
	if m.hasTarget && !info.IsDir() {
		return fmt.Errorf("%s deve ser um diretório real", m.target)
	}
	info, err = os.Lstat(m.home)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	m.hasHome = err == nil
	if m.hasHome && info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(m.home)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(m.home), link)
		}
		if filepath.Clean(link) != m.target {
			return fmt.Errorf("link existente %s não aponta para %s", m.home, m.target)
		}
		m.skip = m.hasTarget
		m.existingLink = true
		m.hasHome = false
	} else if m.hasHome && !info.IsDir() {
		return fmt.Errorf("%s deve ser um diretório ou o link SDK correto", m.home)
	}
	if m.skip {
		return nil
	}
	for _, tree := range []struct {
		path   string
		exists bool
	}{{m.home, m.hasHome}, {m.target, m.hasTarget}} {
		if tree.exists {
			err := validateMigrationTree(tree.path)
			if errors.Is(err, fs.ErrPermission) && ops.repairPermissions != nil {
				if repairErr := ops.repairPermissions(tree.path); repairErr != nil {
					return fmt.Errorf("corrigir permissões de %s: %w", tree.path, repairErr)
				}
				err = validateMigrationTree(tree.path)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *sdkMigration) prepare(ops sdkMigrationOps) error {
	if m.skip {
		return nil
	}
	var err error
	m.stage, err = os.MkdirTemp(filepath.Dir(m.target), ".sdk-stage-*")
	if err != nil {
		return err
	}
	if m.hasTarget {
		if err := ops.copyTree(m.target, m.stage); err != nil {
			return err
		}
	}
	if m.hasHome {
		// Remove collisions only in the disposable target copy. WalkDir never
		// follows source symlinks; destination symlinks are unlinked, not followed.
		if err := filepath.WalkDir(m.home, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || path == m.home {
				return err
			}
			rel, err := filepath.Rel(m.home, path)
			if err != nil {
				return err
			}
			destination := filepath.Join(m.stage, rel)
			info, err := os.Lstat(destination)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() && info.IsDir() {
				return nil
			}
			return os.RemoveAll(destination)
		}); err != nil {
			return err
		}
		if err := ops.copyTree(m.home, m.stage); err != nil {
			return err
		}
	}
	if m.existingLink {
		return nil
	}
	m.linkDir, err = os.MkdirTemp(filepath.Dir(m.home), ".sdk-link-*")
	if err != nil {
		return err
	}
	if err := ops.symlink(m.target, filepath.Join(m.linkDir, "link")); err != nil {
		return err
	}
	if m.hasHome {
		m.homeBackup, err = reserveSDKBackup(m.home)
		if err != nil {
			return err
		}
	}
	if m.hasTarget {
		m.targetBackup, err = reserveSDKBackup(m.target)
	}
	return err
}

func reserveSDKBackup(path string) (string, error) {
	return reserveLocalBackup(path, time.Now())
}

func (m *sdkMigration) activate(ops sdkMigrationOps) error {
	if m.skip {
		return nil
	}
	if m.hasTarget {
		if err := moveToBackup(m.target, m.targetBackup, ops); err != nil {
			return err
		}
		m.targetMoved = true
	}
	if err := ops.rename(m.stage, m.target); err != nil {
		return err
	}
	m.targetInstalled = true
	if m.existingLink {
		return nil
	}
	if m.hasHome {
		if err := moveToBackup(m.home, m.homeBackup, ops); err != nil {
			return err
		}
		m.homeMoved = true
	}
	if err := ops.rename(filepath.Join(m.linkDir, "link"), m.home); err != nil {
		return err
	}
	m.linkInstalled = true
	return nil
}

func (m *sdkMigration) restore(ops sdkMigrationOps) error {
	var result error
	if m.linkInstalled {
		result = errors.Join(result, os.Remove(m.home))
	}
	if m.homeMoved {
		if err := restoreBackup(m.homeBackup, m.home, ops); err != nil {
			result = errors.Join(result, fmt.Errorf("restaurar %s (original em %s): %w", m.home, m.homeBackup, err))
		}
	}
	if m.targetInstalled {
		if err := ops.rename(m.target, m.stage); err != nil {
			// Do not overwrite the newly installed target if rollback cannot move it.
			return errors.Join(result, fmt.Errorf("restaurar %s (backup em %s): %w", m.target, m.targetBackup, err))
		}
	}
	if m.targetMoved {
		if err := restoreBackup(m.targetBackup, m.target, ops); err != nil {
			result = errors.Join(result, fmt.Errorf("restaurar %s (original em %s): %w", m.target, m.targetBackup, err))
		}
	}
	return result
}
