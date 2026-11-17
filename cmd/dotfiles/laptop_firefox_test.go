package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaptopFirefoxRequiresExistingMozillaDirectory(t *testing.T) {
	home := t.TempDir()
	if err := validateLaptopFirefoxHome(home, 123); err == nil {
		t.Fatal("perfil Mozilla ausente aceito")
	}
	path := filepath.Join(home, ".config", "mozilla")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("validação criou perfil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateLaptopFirefoxHome(home, 123); err == nil {
		t.Fatal("arquivo aceito como perfil")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateLaptopFirefoxHome(home, 123); err != nil {
		t.Fatal(err)
	}
	if err := validateLaptopFirefoxHome(home, 0); err == nil {
		t.Fatal("CLI root aceita")
	}
}

func TestPrepareLaptopFirefoxRestoreCreatesOnlyRequestedDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "backups", "Restore Firefox")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "backups", "other.txt")
	if err := os.WriteFile(other, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(path, "existing-profile.zip")
	if err := os.WriteFile(profile, []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	run := func(argv ...string) error {
		calls = append(calls, slices.Clone(argv))
		if err := os.MkdirAll(argv[len(argv)-1], 0755); err != nil {
			return err
		}
		return os.Chmod(argv[len(argv)-1], 0700)
	}
	for range 2 {
		if err := prepareLaptopFirefoxRestore(path, 123, 456, run); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"sudo", "-n", "install", "-d", "-m", "0700", "-o", "123", "-g", "456", "--", path}
	if len(calls) != 2 || !slices.Equal(calls[0], want) {
		t.Fatalf("comando inesperado: %v", calls)
	}
	for file, content := range map[string]string{other: "keep", profile: "backup"} {
		actual, err := os.ReadFile(file)
		if err != nil || string(actual) != content {
			t.Fatalf("backup alterado: %s", file)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0700 {
		t.Fatal("modo incorreto")
	}
	entries, _ := os.ReadDir(path)
	if len(entries) != 1 || entries[0].Name() != "existing-profile.zip" {
		t.Fatal("teste de escrita deixou resíduos")
	}
}

func TestPrepareLaptopFirefoxRestoreCreatesMissingParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mnt", "backups", "Restore Firefox")
	if err := prepareLaptopFirefoxRestore(path, 123, 456, func(argv ...string) error {
		return os.MkdirAll(argv[len(argv)-1], 0700)
	}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatal("pasta não criada")
	}
}

func TestPrepareLaptopFirefoxRestoreRejectsLinksAndSudoFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, filepath.Join(link, "Restore Firefox")} {
		if err := prepareLaptopFirefoxRestore(path, 123, 456, func(...string) error { t.Fatal("sudo não pode executar através de link"); return nil }); err == nil {
			t.Fatal("link aceito")
		}
	}
	if err := prepareLaptopFirefoxRestore(filepath.Join(root, "missing"), 123, 456, func(...string) error { return errors.New("sudo failed") }); err == nil || !strings.Contains(err.Error(), "sudo failed") {
		t.Fatalf("erro sudo não propagado: %v", err)
	}
}

func TestLaptopFirefoxDefaultIgnoresBrowserOnlyInChild(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(home, "browser-env")
	t.Setenv("BROWSER", "chromium")
	t.Setenv("BROWSER_ENV_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeTestExecutable(t, bin, "clear", "exit 0")
	writeTestExecutable(t, bin, "xdg-settings", `printf '%s' "${BROWSER-unset}" > "$BROWSER_ENV_LOG"`)
	j := laptopFirefoxJobFor(home, 123, 456, nil, func(...string) error { return nil })
	if output, err := j.steps[len(j.steps)-1].cmd().CombinedOutput(); err != nil {
		t.Fatalf("configurar padrão: %v: %s", err, output)
	}
	contents, err := os.ReadFile(log)
	if err != nil || string(contents) != "unset" {
		t.Fatalf("BROWSER chegou ao xdg-settings: %q, %v", contents, err)
	}
	if os.Getenv("BROWSER") != "chromium" {
		t.Fatal("ambiente da sessão foi alterado")
	}
}

func TestLaptopFirefoxMenuAndJobOrdering(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	m.screen = screenWebBrowser
	items := m.items()
	if got := itemTitles(items); !slices.Equal(got, []string{"Firefox"}) {
		t.Fatalf("menu inesperado: %v", got)
	}
	if items[0].job == nil {
		t.Fatal("ação Firefox ausente")
	}
	home := t.TempDir()
	job := laptopFirefoxJobFor(home, 123, 456, []step{{label: "Instalar Firefox"}}, func(...string) error { t.Fatal("sudo não esperado no teste"); return nil })
	if job.steps[0].label != "Validar ~/.config/mozilla existente" {
		t.Fatal("validação deve preceder instalação e sudo")
	}
	if _, err := job.steps[0].run(); err == nil {
		t.Fatal("validação não bloqueou configuração")
	}
	last := job.steps[len(job.steps)-1].cmd().Args
	want := []string{"env", "-u", "BROWSER", "xdg-settings", "set", "default-web-browser", "firefox.desktop"}
	if !slices.Equal(last[len(last)-len(want):], want) {
		t.Fatalf("padrão Firefox incorreto: %v", last)
	}
	if laptopFirefoxRestorePath != "/mnt/backups/Restore Firefox" {
		t.Fatal("destino de restauração incorreto")
	}
}
