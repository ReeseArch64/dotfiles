package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func forEachRepositoryProfile(t *testing.T, check func(*testing.T, string), devices ...deviceProfile) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		devices = []deviceProfile{deviceDesktop, deviceNotebook}
	}
	for _, device := range devices {
		t.Run(device.String(), func(t *testing.T) {
			check(t, setupDirectory(root, device))
		})
	}
}

func TestModelSelectsSetupDirectory(t *testing.T) {
	for _, tc := range []struct {
		device  deviceProfile
		profile string
	}{{deviceDesktop, "desktop"}, {deviceNotebook, "laptop"}} {
		m := newModelForDevice("/repo", tc.device)
		if m.dotfiles != "/repo" || m.setupDir != filepath.Join("/repo", "setup", tc.profile) {
			t.Fatalf("raízes inesperadas: repo=%s setup=%s", m.dotfiles, m.setupDir)
		}
		if got := configPath(m.setupDir, "git", ".gitconfig"); got != filepath.Join(m.setupDir, "git", ".gitconfig") {
			t.Fatalf("caminho de configuração inesperado: %s", got)
		}
	}
}

func TestProfileSubmenusAndStatusUseSelectedSetup(t *testing.T) {
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
		t.Run(device.String(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			root := t.TempDir()
			m := newModelForDevice(root, device)
			for _, profile := range []deviceProfile{deviceDesktop, deviceNotebook} {
				setupDir := setupDirectory(root, profile)
				for name, content := range map[string]string{
					"git/.gitconfig":     "[user]\n signingkey = " + profile.String() + "\n",
					"git/.gitattributes": profile.String(),
					"git/.gitignore":     profile.String(),
					"git/config":         profile.String(),
					"niri/config.kdl":    profile.String(),
					"ghostty/config":     profile.String(),
					"mise/mise.toml":     "[tools]\n",
				} {
					path := configPath(setupDir, filepath.FromSlash(name))
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(content), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, action := range []struct {
				screen       screen
				title, label string
			}{
				{screenGit, "Configurar Git", "Copiar ~/.gitconfig"},
				{screenDesktop, "Niri", "Copiar configuração para ~/.config/niri"},
				{screenDesktop, "Ghostty", "Copiar configuração para ~/.config/ghostty"},
			} {
				m.screen = action.screen
				found := false
				for _, it := range m.items() {
					if it.title != action.title {
						continue
					}
					for _, current := range it.job().steps {
						if current.label == action.label {
							found = true
							if _, err := current.run(); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				if !found {
					t.Fatalf("ação ausente: %s / %s", action.title, action.label)
				}
			}
			for _, path := range []string{".gitconfig", ".config/niri/config.kdl", ".config/ghostty/config"} {
				content, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(path)))
				if err != nil || !strings.Contains(string(content), device.String()) {
					t.Fatalf("perfil incorreto em %s: %s, %v", path, content, err)
				}
			}
			m.screen = screenGit
			m.refresh()
			for _, link := range m.git.links {
				if !strings.HasPrefix(link.src, m.setupDir+string(filepath.Separator)) {
					t.Fatalf("fonte Git fora do perfil: %s", link.src)
				}
			}
			for name, header := range map[string]string{"publica": "PUBLIC", "privada": "PRIVATE"} {
				if err := os.WriteFile(filepath.Join(root, "minha_chave_"+name+".asc"), []byte("-----BEGIN PGP "+header+" KEY BLOCK-----\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m.screen = screenGPG
			m.refresh()
			if m.gpg.configErr != nil || m.gpg.signingKey != device.String() || filepath.Dir(m.gpg.paths.private) != root {
				t.Fatalf("raízes GPG incorretas: %#v", m.gpg)
			}
			for _, current := range m.gpgItems()[0].job().steps {
				if current.label == "Importar chave privada" {
					want := []string{"gpg", "--import", filepath.Join(root, "minha_chave_privada.asc")}
					if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
						t.Fatalf("importação fora do repositório: %v", got)
					}
				}
			}
			if err := applyGitSigningKey(m.setupDir, home, "new-key"); err != nil {
				t.Fatal(err)
			}
			other := deviceDesktop
			if device == deviceDesktop {
				other = deviceNotebook
			}
			if key, err := readGitSigningKey(configPath(setupDirectory(root, other), "git", ".gitconfig")); err != nil || key != other.String() {
				t.Fatalf("outro perfil alterado: %s, %v", key, err)
			}
		})
	}
}

func TestRepositoryProfilesProvideAllConfigurationSources(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		for _, name := range []string{"discord", "obsidian", "ollama", turingScreenPackage} {
			info, err := os.Stat(configPath(setupDir, name))
			if filepath.Base(setupDir) == "desktop" {
				if err != nil || !info.IsDir() {
					t.Fatalf("configuração exclusiva %s ausente no desktop: %v", name, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("configuração exclusiva %s não deve existir no laptop: %v", name, err)
			}
		}
		for _, name := range []string{"btop", "cursor", "fastfetch", "fish", "ghostty", "git", "mise", "niri", "noctalia", "pi", "ssh"} {
			if info, err := os.Stat(configPath(setupDir, name)); err != nil || !info.IsDir() {
				t.Fatalf("configuração %s ausente em %s: %v", name, setupDir, err)
			}
		}
		if err := validatePiConfig(setupDir); err != nil {
			t.Fatal(err)
		}
		if err := installMiseConfig(setupDir, t.TempDir()); err != nil {
			t.Fatal(err)
		}
		if err := installNiri(setupDir, t.TempDir(), time.Now()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestProfileConfigurationSubmenusAndDevelopment(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, source string) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		device := deviceDesktop
		if filepath.Base(source) == "laptop" {
			device = deviceNotebook
		}
		m := newModelForDevice(t.TempDir(), device)
		for _, name := range []string{"pi", "noctalia", "fish", "mise"} {
			if err := copyDirectory(configPath(source, name), configPath(m.setupDir, name)); err != nil {
				t.Fatal(err)
			}
		}
		pluginDir := filepath.Join(home, ".local", "state", "noctalia", "plugins", "materialized", "community")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatal(err)
		}
		installNoctaliaTestPlugins(t, pluginDir)
		for _, screen := range []screen{screenNoctalia, screenPiAgent} {
			m.screen = screen
			for _, current := range m.items()[0].job().steps {
				if current.run != nil {
					if _, err := current.run(); err != nil {
						t.Fatal(err)
					}
				}
			}
			m.refresh()
		}
		if !m.piAgent.settingsOK || !m.piAgent.themeOK || m.piAgent.configErr != nil {
			t.Fatalf("Pi não usa perfil selecionado: %#v", m.piAgent)
		}
		for _, file := range m.noctalia.files {
			if !file.ok || !strings.HasPrefix(file.source, m.setupDir+string(filepath.Separator)) {
				t.Fatalf("Noctalia não usa perfil selecionado: %#v", file)
			}
		}
		m.screen = screenDevelopment
		for _, it := range m.items() {
			if it.title != "Ambiente de Desenvolvimento" {
				continue
			}
			for _, current := range it.job().steps {
				switch current.label {
				case "Symlink ~/.config/mise/config.toml", "Copiar configuração para ~/.config/fish/config.fish":
					if _, err := current.run(); err != nil {
						t.Fatal(err)
					}
				case "Instalar ferramentas configuradas via mise":
					want := []string{"mise", "--cd", configPath(m.setupDir, "mise"), "install"}
					if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
						t.Fatalf("mise não usa perfil selecionado: %v", got)
					}
				}
			}
		}
		if target, err := os.Readlink(filepath.Join(home, ".config", "mise", "config.toml")); err != nil || target != configPath(m.setupDir, "mise", "mise.toml") {
			t.Fatalf("mise incorreto: %s, %v", target, err)
		}
	})
}

func TestFindSetupRepository(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "bin", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "setup"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "setup"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := findSetupRepository(nested); got != root {
		t.Fatalf("repositório incorreto: %s", got)
	}
	t.Setenv("DOTFILES_DIR", root)
	if got := findDotfiles(); got != root {
		t.Fatalf("DOTFILES_DIR ignorado: %s", got)
	}
}
