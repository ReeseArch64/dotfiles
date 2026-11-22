package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type sdkMigrationOps struct {
	ctx               context.Context
	backupRoot        string
	recoveryCopy      func(string, string) error
	copyTree          func(string, string) error
	repairPermissions func(string) error
	rename            func(string, string) error
	symlink           func(string, string) error
}

type sdkMigration struct {
	home, target, stage, homeBackup, targetBackup, linkDir string
	homeRetired, targetRetired                             string
	hasHome, hasTarget, skip, existingLink                 bool
	homeMoved, targetMoved, targetInstalled, linkInstalled bool
}

func copySDKTree(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "cp", "-a", "--no-target-directory", "--", source, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copiar %s para %s: %w: %s", source, target, err, output)
	}
	return nil
}

func prepareSDKDirectories(home, root string, ops sdkMigrationOps) error {
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
	if pathInside(home, root) || pathInside(root, home) {
		return fmt.Errorf("HOME e destino não podem se sobrepor: %s, %s", home, root)
	}

	migrations := make([]sdkMigration, 0, len(sdkDirectoryMappings))
	for _, mapping := range sdkDirectoryMappings {
		migrations = append(migrations, sdkMigration{home: filepath.Join(home, mapping[0]), target: filepath.Join(root, mapping[1])})
	}
	return runDirectoryMigrations(migrations, ops)
}

var sdkDirectoryMappings = [][2]string{
	{".aur", "Aur"}, {".cache", "Cache"}, {"Android", "Android"}, {".gradle", "Gradle"},
	{".bun", "Bun"}, {".pi", "Pi"}, {".codex", "Codex"}, {".claude", "Claude"}, {".npm", "Npm"}, {".rustup", "Rustup"},
	{".agent-browser", "AgentBrowser"}, {".cargo", "Cargo"}, {".expo", "Expo"},
	{".vite-plus", "VitePlus"}, {".docker", "Docker"}, {".dotnet", "Dotnet"}, {".java", "Java"},
	{".copilot", "Copilot"}, {".azure", "Azure"}, {".aws", "Aws"}, {".gemini", "Gemini"}, {".railway", "Railway"}, {".tauri", "Tauri"},
}

func runDirectoryMigrations(migrations []sdkMigration, ops sdkMigrationOps) (result error) {
	// Preflight every mapping before creating staging directories or backups.
	for i := range migrations {
		if err := ops.ctx.Err(); err != nil {
			return err
		}
		if err := migrations[i].preflight(ops); err != nil {
			return err
		}
	}
	defer func() {
		if result != nil {
			recovery := ops
			recovery.copyTree = ops.recoveryCopy
			for i := len(migrations) - 1; i >= 0; i-- {
				result = errors.Join(result, migrations[i].restore(recovery))
			}
		} else {
			for _, m := range migrations {
				for _, retired := range []string{m.homeRetired, m.targetRetired} {
					if retired != "" {
						result = errors.Join(result, forceRemoveAll(filepath.Dir(retired)))
					}
				}
			}
		}
		for _, m := range migrations {
			// Only disposable copies and temporary links are recursively removed.
			for _, path := range []string{m.stage, m.linkDir} {
				if path != "" {
					result = errors.Join(result, forceRemoveAll(path))
				}
			}
		}
	}()

	// Finish every merged copy before any original is moved or link activated.
	for i := range migrations {
		if err := ops.ctx.Err(); err != nil {
			return err
		}
		if err := migrations[i].prepare(ops); err != nil {
			return err
		}
	}
	for i := range migrations {
		if err := ops.ctx.Err(); err != nil {
			return err
		}
		if err := migrations[i].activate(ops); err != nil {
			return err
		}
	}
	return ops.ctx.Err()
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
			return forceRemoveAll(destination)
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
		m.homeBackup, err = reserveSDKBackup(ops.backupRoot, m.home, time.Now())
		if err != nil {
			return err
		}
	}
	if m.hasTarget {
		m.targetBackup, err = reserveSDKBackup(ops.backupRoot, m.target, time.Now())
	}
	return err
}

func (m *sdkMigration) activate(ops sdkMigrationOps) error {
	if m.skip {
		return nil
	}
	if m.hasTarget {
		var err error
		m.targetRetired, err = moveToBackup(m.target, m.targetBackup, ops)
		if err != nil {
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
		var err error
		m.homeRetired, err = moveToBackup(m.home, m.homeBackup, ops)
		if err != nil {
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
		if err := restoreSDKOriginal(m.homeBackup, m.homeRetired, m.home, ops); err != nil {
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
		if err := restoreSDKOriginal(m.targetBackup, m.targetRetired, m.target, ops); err != nil {
			result = errors.Join(result, fmt.Errorf("restaurar %s (original em %s): %w", m.target, m.targetBackup, err))
		}
	}
	return result
}
