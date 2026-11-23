package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type sdkMigrationOps struct {
	ctx               context.Context
	backupRoot        string
	archiveHomeOnly   bool
	prepareBackup     func() error
	reportArchive     func(string, string)
	recoveryCopy      func(string, string) error
	copyTree          func(string, string) error
	repairPermissions func(string) error
	rename            func(string, string) error
	symlink           func(string, string) error
}

type sdkMigration struct {
	home, target, stage, homeBackup, targetBackup, linkDir string
	homeRetired, targetRetired                             string
	createdParents                                         []string
	hasHome, hasTarget, skip, existingLink, homeEmpty      bool
	homeMoved, targetMoved, targetInstalled, linkInstalled bool
}

func copySDKTree(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "cp", "-a", "--no-target-directory", "--", source, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copiar %s para %s: %w: %s", source, target, err, output)
	}
	return nil
}

func prepareDirectoryLinks(home, root string, mappings [][2]string, ops sdkMigrationOps) error {
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

	migrations := make([]sdkMigration, 0, len(mappings))
	for _, mapping := range mappings {
		source := filepath.Join(home, mapping[0])
		if mapping[0] == miseSDKPath {
			source = sdkMiseSource(home, root)
		}
		migrations = append(migrations, sdkMigration{home: source, target: filepath.Join(root, mapping[1])})
	}
	return runDirectoryMigrations(migrations, ops)
}

const miseSDKPath = ".local/share/mise"

var sdkDirectoryMappings = [][2]string{
	{".aur", "Aur"}, {".cache", "Cache"}, {"Android", "Android"}, {".gradle", "Gradle"},
	{".bun", "Bun"}, {".pi", "Pi"}, {".codex", "Codex"}, {".claude", "Claude"}, {".npm", "Npm"}, {".rustup", "Rustup"},
	{".agent-browser", "AgentBrowser"}, {".cargo", "Cargo"}, {".expo", "Expo"},
	{".vite-plus", "VitePlus"}, {".docker", "Docker"}, {".dotnet", "Dotnet"}, {".java", "Java"},
	{".copilot", "Copilot"}, {".azure", "Azure"}, {".aws", "Aws"}, {".gemini", "Gemini"}, {".railway", "Railway"}, {".tauri", "Tauri"},
	{miseSDKPath, "Mise"},
	{"go", "Go"}, {".render", "Render"}, {".maple", "Maple"},
}

func sdkMiseSource(home, root string) string {
	local := filepath.Join(home, ".local")
	legacy := filepath.Join(root, "Local")
	// Older CachyOS setups already moved .local to the SDK. Accept that exact link.
	if link, err := os.Readlink(local); err == nil {
		if !filepath.IsAbs(link) {
			link = filepath.Join(home, link)
		}
		if filepath.Clean(link) == legacy && sdkRealDirectoryPath(legacy) == nil {
			return filepath.Join(legacy, "share", "mise")
		}
	}
	return filepath.Join(home, miseSDKPath)
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
				if m.homeEmpty && m.homeRetired != "" {
					if err := os.Remove(m.homeRetired); err != nil {
						result = errors.Join(result, fmt.Errorf("pasta local preservada em %s: %w", m.homeRetired, err))
					} else {
						result = errors.Join(result, os.Remove(filepath.Dir(m.homeRetired)))
					}
					continue
				}
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
			if result != nil {
				for i := len(m.createdParents) - 1; i >= 0; i-- {
					if err := os.Remove(m.createdParents[i]); err != nil && !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTEMPTY) {
						result = errors.Join(result, err)
					}
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
	if err := validateBackupPath(filepath.Dir(m.home)); err != nil {
		return err
	}
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
	}{{m.home, m.hasHome}, {m.target, m.hasTarget && !ops.archiveHomeOnly}} {
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
	if ops.archiveHomeOnly && m.hasHome {
		m.homeEmpty, err = emptyMigrationDirectory(m.home)
		return err
	}
	return nil
}

func emptyMigrationDirectory(path string) (bool, error) {
	directory, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	_, err = directory.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

func (m *sdkMigration) prepare(ops sdkMigrationOps) error {
	if m.skip {
		return nil
	}
	var err error
	if !ops.archiveHomeOnly || !m.hasTarget {
		m.stage, err = os.MkdirTemp(filepath.Dir(m.target), ".sdk-stage-*")
		if err != nil {
			return err
		}
	}
	if ops.archiveHomeOnly {
		return m.prepareLink(ops)
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
	if err := m.prepareLink(ops); err != nil {
		return err
	}
	if m.hasTarget {
		m.targetBackup, err = reserveSDKBackup(ops.backupRoot, m.target, time.Now())
	}
	return err
}

func (m *sdkMigration) prepareLink(ops sdkMigrationOps) error {
	if m.existingLink {
		return nil
	}
	var err error
	if err := m.prepareLinkParent(); err != nil {
		return err
	}
	m.linkDir, err = os.MkdirTemp(filepath.Dir(m.home), ".sdk-link-*")
	if err != nil {
		return err
	}
	if err := ops.symlink(m.target, filepath.Join(m.linkDir, "link")); err != nil {
		return err
	}
	if m.hasHome && !m.homeEmpty {
		if ops.prepareBackup != nil {
			if err := ops.prepareBackup(); err != nil {
				return err
			}
		}
		m.homeBackup, err = reserveSDKBackup(ops.backupRoot, m.home, time.Now())
	}
	return err
}

func (m *sdkMigration) prepareLinkParent() error {
	parent := filepath.Dir(m.home)
	var missing []string
	for path := parent; ; path = filepath.Dir(path) {
		_, err := os.Lstat(path)
		if err == nil {
			if err := sdkRealDirectoryPath(path); err != nil {
				return err
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, path)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0755); err != nil {
			return err
		}
		m.createdParents = append(m.createdParents, missing[i])
	}
	return sdkRealDirectoryPath(parent)
}

func (m *sdkMigration) activate(ops sdkMigrationOps) error {
	if m.skip {
		return nil
	}
	if m.hasTarget && !ops.archiveHomeOnly {
		var err error
		m.targetRetired, err = moveToBackup(m.target, m.targetBackup, ops)
		if err != nil {
			return err
		}
		m.targetMoved = true
	}
	if m.stage != "" {
		if err := ops.rename(m.stage, m.target); err != nil {
			return err
		}
		m.targetInstalled = true
	}
	if m.existingLink {
		return nil
	}
	if m.hasHome {
		var err error
		if m.homeEmpty {
			m.homeRetired, err = retireDirectory(m.home, ops)
		} else {
			if ops.reportArchive != nil {
				ops.reportArchive(m.home, m.homeBackup)
			}
			m.homeRetired, err = moveToBackup(m.home, m.homeBackup, ops)
		}
		if err != nil {
			m.homeMoved = m.homeRetired != ""
			return err
		}
		m.homeMoved = true
		if m.homeEmpty {
			empty, err := emptyMigrationDirectory(m.homeRetired)
			if err != nil {
				return err
			}
			if !empty {
				return fmt.Errorf("%s recebeu arquivos durante a migração; feche os aplicativos que usam a pasta e tente novamente", m.home)
			}
		}
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
		if ops.archiveHomeOnly {
			if err := unix.Rmdir(m.target); err != nil && !errors.Is(err, os.ErrNotExist) {
				result = errors.Join(result, fmt.Errorf("destino preservado em %s: %w", m.target, err))
			}
			return result
		}
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
