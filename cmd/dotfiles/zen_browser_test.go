package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type zenTarEntry struct {
	name     string
	content  string
	typeflag byte
	linkname string
	mode     int64
}

func writeZenTestArchive(t *testing.T, archive string, entries []zenTarEntry) {
	t.Helper()
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0644
		}
		header := &tar.Header{
			Name:     entry.name,
			Mode:     mode,
			Size:     int64(len(entry.content)),
			Typeflag: entry.typeflag,
			Linkname: entry.linkname,
		}
		if entry.typeflag == tar.TypeDir || entry.typeflag == tar.TypeSymlink {
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.content != "" {
			if _, err := writer.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func validZenTestEntries() []zenTarEntry {
	return []zenTarEntry{
		{name: ".config/zen/", typeflag: tar.TypeDir, mode: 0700},
		{name: ".config/zen/profile.txt", content: "profile\n", typeflag: tar.TypeReg},
		{name: ".config/zen/lock", typeflag: tar.TypeSymlink, linkname: "session-lock"},
		{name: ".cache/zen/", typeflag: tar.TypeDir, mode: 0700},
		{name: ".cache/zen/cache.txt", content: "cache\n", typeflag: tar.TypeReg},
		{name: ".local/share/keyrings/", typeflag: tar.TypeDir, mode: 0700},
		{name: ".local/share/keyrings/login.keyring", content: "secret\n", typeflag: tar.TypeReg, mode: 0600},
	}
}

func TestValidateZenBackupRequiresRegularArchive(t *testing.T) {
	dotfiles := t.TempDir()
	if err := validateZenBackup(dotfiles); err == nil || !strings.Contains(err.Error(), "zen-backup.tar") {
		t.Fatalf("erro inesperado para backup ausente: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dotfiles, "zen-backup.tar"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := validateZenBackup(dotfiles); err == nil || !strings.Contains(err.Error(), "arquivo regular") {
		t.Fatalf("erro inesperado para diretório: %v", err)
	}
}

func TestExtractZenBackupAllowsExpectedDirectoriesAndSafeSymlink(t *testing.T) {
	dotfiles := t.TempDir()
	writeZenTestArchive(t, filepath.Join(dotfiles, "zen-backup.tar"), validZenTestEntries())

	if err := extractZenBackup(dotfiles); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dotfiles, "zen-backup", ".config", "zen", "profile.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "profile\n" {
		t.Fatalf("perfil inesperado: %q", content)
	}
	target, err := os.Readlink(filepath.Join(dotfiles, "zen-backup", ".config", "zen", "lock"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "session-lock" {
		t.Fatalf("link inesperado: %s", target)
	}
}

func TestExtractZenBackupRejectsUnexpectedPaths(t *testing.T) {
	dotfiles := t.TempDir()
	writeZenTestArchive(t, filepath.Join(dotfiles, "zen-backup.tar"), []zenTarEntry{
		{name: "../outside", content: "unsafe", typeflag: tar.TypeReg},
	})

	err := extractZenBackup(dotfiles)
	if err == nil || !strings.Contains(err.Error(), "caminho inválido") {
		t.Fatalf("caminho inseguro não foi rejeitado: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dotfiles), "outside")); !os.IsNotExist(err) {
		t.Fatalf("arquivo externo criado: %v", err)
	}
}

func TestRestoreZenBackupReplacesCurrentDirectories(t *testing.T) {
	dotfiles := t.TempDir()
	home := t.TempDir()
	writeZenTestArchive(t, filepath.Join(dotfiles, "zen-backup.tar"), validZenTestEntries())
	if err := extractZenBackup(dotfiles); err != nil {
		t.Fatal(err)
	}
	for _, root := range zenBackupRoots {
		destination := filepath.Join(home, filepath.FromSlash(root))
		if err := os.MkdirAll(destination, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, "old"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := restoreZenBackup(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		".config/zen/profile.txt":             "profile\n",
		".cache/zen/cache.txt":                "cache\n",
		".local/share/keyrings/login.keyring": "secret\n",
	}
	for name, want := range checks {
		content, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("conteúdo inesperado em %s: %q", name, content)
		}
	}
	for _, root := range zenBackupRoots {
		if _, err := os.Lstat(filepath.Join(home, filepath.FromSlash(root), "old")); !os.IsNotExist(err) {
			t.Fatalf("dados anteriores não foram substituídos em %s", root)
		}
		if _, err := os.Lstat(filepath.Join(dotfiles, "zen-backup", filepath.FromSlash(root))); !os.IsNotExist(err) {
			t.Fatalf("dados restaurados não foram movidos de %s", root)
		}
	}
}

func TestZenBrowserJobOrdersValidationInstallationAndRestore(t *testing.T) {
	packageStep := nativeStep("Instalar zen-browser-bin via AUR", func() error { return nil })
	configured := zenBrowserJobFor("/tmp/dotfiles", "/tmp/home", []step{packageStep})
	want := []string{
		"Verificar zen-backup.tar",
		"Instalar zen-browser-bin via AUR",
		"Descompactar backup para zen-backup",
		"Substituir dados do Zen Browser",
	}
	if got := jobStepLabels(configured); !slices.Equal(got, want) {
		t.Fatalf("passos inesperados: %v", got)
	}
	if zenBrowserAurPackage != "zen-browser-bin" {
		t.Fatalf("pacote AUR inesperado: %s", zenBrowserAurPackage)
	}
}
