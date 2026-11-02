package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSharedConfigTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigPathSharedFallbackAndProfilePriority(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "setup", "config", "app", "config")
	writeSharedConfigTestFile(t, shared, "shared")
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
		profile := setupDirectory(root, device)
		if got := configPath(profile, "app", "config"); got != shared {
			t.Fatalf("base compartilhada não resolvida: %s", got)
		}
		local := filepath.Join(profile, "app", "config")
		writeSharedConfigTestFile(t, local, device.String())
		if got := configPath(profile, "app", "config"); got != local {
			t.Fatalf("override do perfil não teve prioridade: %s", got)
		}
		missing := filepath.Join(profile, "app", "missing")
		if got := configPath(profile, "app", "missing"); got != missing {
			t.Fatalf("caminho ausente incorreto: %s", got)
		}
	}
}

func TestConfigPathDoesNotInheritForOtherProfiles(t *testing.T) {
	root := t.TempDir()
	writeSharedConfigTestFile(t, filepath.Join(root, "setup", "config", "app", "config"), "shared")
	for _, device := range []deviceProfile{deviceServer, deviceHomeProd, deviceMacBook} {
		profile := setupDirectory(root, device)
		local := filepath.Join(profile, "app", "config")
		if got := configPath(profile, "app", "config"); got != local {
			t.Fatalf("%s herdou configuração compartilhada: %s", device, got)
		}
		if _, _, err := materializeConfigDirectory(profile, "app"); err == nil {
			t.Fatalf("%s herdou diretório compartilhado", device)
		}
		writeSharedConfigTestFile(t, local, device.String())
		source, cleanup, err := materializeConfigDirectory(profile, "app")
		if err != nil {
			t.Fatal(err)
		}
		cleanup()
		if source != filepath.Dir(local) {
			t.Fatalf("%s não manteve a origem local: %s", device, source)
		}
	}
	if got := sharedConfigDirectory(filepath.Join(root, "other", "desktop")); got != "" {
		t.Fatalf("diretório fora de setup herdou configuração: %s", got)
	}
}

func TestSharedConfigNeverSuppliesMutableGitConfig(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "setup", "config", "git", ".gitconfig")
	writeSharedConfigTestFile(t, shared, "[user]\n signingkey = shared\n")
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
		profile := setupDirectory(root, device)
		if got := configPath(profile, "git", ".gitconfig"); got != filepath.Join(profile, "git", ".gitconfig") {
			t.Fatalf("configuração mutável veio da base: %s", got)
		}
		if err := applyGitSigningKey(profile, t.TempDir(), "new-key"); err == nil {
			t.Fatal("GPG não pode atualizar a base se o arquivo do perfil estiver ausente")
		}
	}
	if key, err := readGitSigningKey(shared); err != nil || key != "shared" {
		t.Fatalf("base compartilhada foi alterada: %s, %v", key, err)
	}
}

func TestSharedDirectoryCompositionPreservesOverridesAndBackups(t *testing.T) {
	testBackupStore(t)
	root, home := t.TempDir(), t.TempDir()
	shared := filepath.Join(root, "setup", "config", "niri")
	writeSharedConfigTestFile(t, filepath.Join(shared, "config.kdl"), "common root")
	writeSharedConfigTestFile(t, filepath.Join(shared, "cfg", "input.kdl"), "common input")
	writeSharedConfigTestFile(t, filepath.Join(shared, "cfg", "display.kdl"), "common display")
	profile := setupDirectory(root, deviceNotebook)
	localDisplay := filepath.Join(profile, "niri", "cfg", "display.kdl")
	writeSharedConfigTestFile(t, localDisplay, "laptop display")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	destination := filepath.Join(home, ".config", "niri")
	for range 2 {
		if err := installNiri(profile, home, now); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{
		"config.kdl": "common root", "cfg/input.kdl": "common input", "cfg/display.kdl": "laptop display",
	} {
		content, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		if err != nil || string(content) != want {
			t.Fatalf("composição incorreta em %s: %q, %v", name, content, err)
		}
	}
	if matches, err := testBackupPaths(localBackupRoot(), "files", destination); err != nil || len(matches) != 0 {
		t.Fatalf("reinstalação idêntica criou backups: %v, %v", matches, err)
	}
	writeSharedConfigTestFile(t, localDisplay, "updated laptop display")
	if err := installNiri(profile, home, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	backup := testBackupFor(t, destination)
	if content, err := os.ReadFile(filepath.Join(backup, "cfg", "display.kdl")); err != nil || string(content) != "laptop display" {
		t.Fatalf("backup não preservou a composição anterior: %q, %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(shared, "cfg", "display.kdl")); err != nil || string(content) != "common display" {
		t.Fatalf("base compartilhada foi alterada: %q, %v", content, err)
	}
	source, cleanup, err := materializeConfigDirectory(profile, "niri")
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("composição temporária não foi removida: %v", err)
	}
}

func TestInvalidProfileOverlayPreservesInstalledDirectory(t *testing.T) {
	testBackupStore(t)
	root, home := t.TempDir(), t.TempDir()
	shared := filepath.Join(root, "setup", "config", "niri", "config.kdl")
	writeSharedConfigTestFile(t, shared, "common")
	profile := setupDirectory(root, deviceNotebook)
	invalid := filepath.Join(profile, "niri", "config.kdl")
	if err := os.MkdirAll(filepath.Dir(invalid), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, invalid); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "niri")
	writeSharedConfigTestFile(t, filepath.Join(destination, "config.kdl"), "installed")
	if err := installNiri(profile, home, time.Now()); err == nil {
		t.Fatal("overlay inválido deveria falhar antes de alterar o destino")
	}
	if content, err := os.ReadFile(filepath.Join(destination, "config.kdl")); err != nil || string(content) != "installed" {
		t.Fatalf("configuração instalada foi alterada: %q, %v", content, err)
	}
}

func TestNoctaliaResolvesEachFileIndependently(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "setup", "config", "noctalia", "state.toml")
	writeSharedConfigTestFile(t, shared, "shared")
	profile := setupDirectory(root, deviceNotebook)
	local := filepath.Join(profile, "noctalia", "settings.toml")
	writeSharedConfigTestFile(t, local, "local")
	files := noctaliaFileDefs(profile, t.TempDir())
	if files[0].source != local || files[1].source != shared {
		t.Fatalf("origens individuais não foram resolvidas: %#v", files)
	}
}

func TestMiseRunsBesideResolvedConfiguration(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "setup", "config", "mise", "mise.toml")
	writeSharedConfigTestFile(t, shared, "[tools]\n")
	profile := setupDirectory(root, deviceNotebook)
	writeSharedConfigTestFile(t, filepath.Join(profile, "mise", "README.md"), "profile notes")
	args := commandTail(t, developmentToolsInstallStep(profile), 4)
	if args[2] != filepath.Dir(shared) {
		t.Fatalf("mise não usa o diretório da configuração compartilhada: %v", args)
	}
	local := filepath.Join(profile, "mise", "mise.toml")
	writeSharedConfigTestFile(t, local, "[tools]\n")
	args = commandTail(t, developmentToolsInstallStep(profile), 4)
	if args[2] != filepath.Dir(local) {
		t.Fatalf("mise não usa a configuração específica: %v", args)
	}
}

func TestRepositorySharedFilesAreNotDuplicated(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(root, "setup", "config")
	count := 0
	err = filepath.WalkDir(shared, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(shared, path)
		if err != nil {
			return err
		}
		count++
		for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
			profile := setupDirectory(root, device)
			local := filepath.Join(profile, relative)
			expected := path
			if _, err := os.Lstat(local); err == nil {
				expected = local
				if regularFileMatches(path, local) {
					t.Errorf("arquivo compartilhado duplicado no perfil: %s / %s", device, relative)
				}
			} else if !os.IsNotExist(err) {
				return err
			}
			if got := configPath(profile, relative); got != expected {
				t.Errorf("arquivo de configuração não resolvido: %s", got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("nenhum arquivo compartilhado encontrado")
	}
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
		if _, err := os.Stat(filepath.Join(setupDirectory(root, device), "git", ".gitconfig")); err != nil {
			t.Fatalf(".gitconfig do dispositivo ausente: %v", err)
		}
	}
}
