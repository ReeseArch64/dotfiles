package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const zenBrowserAurPackage = "zen-browser-bin"

var zenBackupRoots = []string{".config/zen", ".cache/zen", ".local/share/keyrings"}

type zenSymlink struct {
	name   string
	target string
}

type zenRestorePath struct {
	source      string
	destination string
	staging     string
	previous    string
	active      bool
}

func validateZenBackup(dotfiles string) error {
	archive := filepath.Join(dotfiles, "zen-backup.tar")
	info, err := os.Stat(archive)
	if err != nil {
		return fmt.Errorf("backup do Zen ausente em %s: %w", archive, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é um arquivo regular", archive)
	}
	return nil
}

func zenArchivePath(name string) (string, error) {
	if name == "" || path.IsAbs(name) {
		return "", fmt.Errorf("caminho inválido no backup: %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("caminho inválido no backup: %q", name)
	}
	for _, root := range zenBackupRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return clean, nil
		}
	}
	return "", fmt.Errorf("caminho inesperado no backup: %s", name)
}

func extractZenArchive(archive, destination string) error {
	input, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", archive, err)
	}
	defer input.Close()

	reader := tar.NewReader(input)
	directoryModes := map[string]os.FileMode{}
	var symlinks []zenSymlink
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ler %s: %w", archive, err)
		}
		name, err := zenArchivePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("criar %s: %w", target, err)
			}
			directoryModes[target] = os.FileMode(header.Mode).Perm()
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("criar %s: %w", filepath.Dir(target), err)
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("criar %s: %w", target, err)
			}
			_, copyErr := io.Copy(output, reader)
			closeErr := output.Close()
			if copyErr != nil {
				return fmt.Errorf("extrair %s: %w", name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("fechar %s: %w", target, closeErr)
			}
			if err := os.Chmod(target, os.FileMode(header.Mode).Perm()); err != nil {
				return fmt.Errorf("ajustar permissões de %s: %w", target, err)
			}
		case tar.TypeSymlink:
			if path.IsAbs(header.Linkname) {
				return fmt.Errorf("link absoluto inválido no backup: %s", name)
			}
			resolved, err := zenArchivePath(path.Join(path.Dir(name), header.Linkname))
			if err != nil {
				return fmt.Errorf("destino inválido do link %s: %w", name, err)
			}
			if resolved == "" {
				return fmt.Errorf("destino vazio do link %s", name)
			}
			symlinks = append(symlinks, zenSymlink{name: target, target: header.Linkname})
		default:
			return fmt.Errorf("tipo não suportado no backup para %s", name)
		}
	}
	for _, link := range symlinks {
		if err := os.MkdirAll(filepath.Dir(link.name), 0755); err != nil {
			return fmt.Errorf("criar %s: %w", filepath.Dir(link.name), err)
		}
		if err := os.Symlink(link.target, link.name); err != nil {
			return fmt.Errorf("criar link %s: %w", link.name, err)
		}
	}
	directories := make([]string, 0, len(directoryModes))
	for directory := range directoryModes {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := os.Chmod(directory, directoryModes[directory]); err != nil {
			return fmt.Errorf("ajustar permissões de %s: %w", directory, err)
		}
	}
	return nil
}

func extractZenBackup(dotfiles string) error {
	archive := filepath.Join(dotfiles, "zen-backup.tar")
	if err := validateZenBackup(dotfiles); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(dotfiles, ".zen-backup-")
	if err != nil {
		return fmt.Errorf("criar diretório temporário: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := extractZenArchive(archive, staging); err != nil {
		return err
	}
	for _, root := range zenBackupRoots {
		info, err := os.Stat(filepath.Join(staging, filepath.FromSlash(root)))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("diretório obrigatório ausente no backup: %s", root)
		}
	}
	destination := filepath.Join(dotfiles, "zen-backup")
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("remover %s: %w", destination, err)
	}
	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("ativar %s: %w", destination, err)
	}
	return nil
}

func copyZenDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
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
			return copyFileAtomic(current, target)
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(current)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) {
				return fmt.Errorf("link absoluto inválido: %s", current)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(current), link))
			relativeTarget, err := filepath.Rel(source, resolved)
			if err != nil || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("link fora do backup: %s", current)
			}
			return os.Symlink(link, target)
		default:
			return fmt.Errorf("tipo de arquivo não suportado: %s", current)
		}
	})
}

func prepareZenRestorePath(source, destination string) (zenRestorePath, error) {
	info, err := os.Stat(source)
	if err != nil {
		return zenRestorePath{}, fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.IsDir() {
		return zenRestorePath{}, fmt.Errorf("%s não é um diretório", source)
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return zenRestorePath{}, fmt.Errorf("criar %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, ".zen-restore-")
	if err != nil {
		return zenRestorePath{}, fmt.Errorf("criar diretório temporário: %w", err)
	}
	if err := os.Chmod(staging, info.Mode().Perm()); err != nil {
		os.RemoveAll(staging)
		return zenRestorePath{}, fmt.Errorf("ajustar permissões de %s: %w", staging, err)
	}
	if err := copyZenDirectory(source, staging); err != nil {
		os.RemoveAll(staging)
		return zenRestorePath{}, fmt.Errorf("preparar restauração de %s: %w", destination, err)
	}
	return zenRestorePath{source: source, destination: destination, staging: staging}, nil
}

func rollbackZenRestore(paths []zenRestorePath) {
	for i := len(paths) - 1; i >= 0; i-- {
		current := paths[i]
		if current.active {
			_ = os.RemoveAll(current.destination)
		}
		if current.previous != "" {
			_ = os.Rename(current.previous, current.destination)
		}
		if current.staging != "" {
			_ = os.RemoveAll(current.staging)
		}
	}
}

func restoreZenBackup(dotfiles, home string) error {
	backup := filepath.Join(dotfiles, "zen-backup")
	paths := make([]zenRestorePath, 0, len(zenBackupRoots))
	for _, root := range zenBackupRoots {
		current, err := prepareZenRestorePath(
			filepath.Join(backup, filepath.FromSlash(root)),
			filepath.Join(home, filepath.FromSlash(root)),
		)
		if err != nil {
			rollbackZenRestore(paths)
			return err
		}
		paths = append(paths, current)
	}
	for i := range paths {
		if _, err := os.Lstat(paths[i].destination); err == nil {
			placeholder, err := os.MkdirTemp(filepath.Dir(paths[i].destination), ".zen-previous-")
			if err != nil {
				rollbackZenRestore(paths)
				return fmt.Errorf("preparar substituição de %s: %w", paths[i].destination, err)
			}
			if err := os.Remove(placeholder); err != nil {
				_ = os.RemoveAll(placeholder)
				rollbackZenRestore(paths)
				return fmt.Errorf("preparar %s: %w", placeholder, err)
			}
			paths[i].previous = placeholder
			if err := os.Rename(paths[i].destination, paths[i].previous); err != nil {
				rollbackZenRestore(paths)
				return fmt.Errorf("preservar %s: %w", paths[i].destination, err)
			}
		} else if !os.IsNotExist(err) {
			rollbackZenRestore(paths)
			return fmt.Errorf("verificar %s: %w", paths[i].destination, err)
		}
		if err := os.Rename(paths[i].staging, paths[i].destination); err != nil {
			rollbackZenRestore(paths)
			return fmt.Errorf("restaurar %s: %w", paths[i].destination, err)
		}
		paths[i].staging = ""
		paths[i].active = true
	}
	for _, current := range paths {
		if current.previous != "" {
			if err := os.RemoveAll(current.previous); err != nil {
				return fmt.Errorf("remover dados substituídos em %s: %w", current.previous, err)
			}
		}
		if err := os.RemoveAll(current.source); err != nil {
			return fmt.Errorf("remover dados restaurados de %s: %w", current.source, err)
		}
	}
	return nil
}

func zenBrowserJobFor(dotfiles, home string, packageSteps []step) job {
	steps := []step{nativeStep("Verificar zen-backup.tar", func() error {
		return validateZenBackup(dotfiles)
	})}
	steps = append(steps, packageSteps...)
	steps = append(steps,
		nativeStep("Descompactar backup para zen-backup", func() error {
			return extractZenBackup(dotfiles)
		}),
		nativeStep("Substituir dados do Zen Browser", func() error {
			return restoreZenBackup(dotfiles, home)
		}),
	)
	return job{
		title: "Configurar Zen Browser",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Zen Browser instalado e backup restaurado.")
		},
	}
}

func zenBrowserJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return zenBrowserJobFor(dotfiles, home, ensureShellyPkgs("aur", zenBrowserAurPackage))
}
