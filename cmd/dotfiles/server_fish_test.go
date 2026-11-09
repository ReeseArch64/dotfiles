package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallServerFishPreservesRecoveryShellFiles(t *testing.T) {
	testBackupStore(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	for _, name := range []string{".bashrc", ".bash_profile", ".profile", ".config/fish/functions/custom.fish"} {
		sdkTestWrite(t, filepath.Join(home, name), "keep")
	}
	for range 2 {
		if err := installServerFish(root, home); err != nil {
			t.Fatal(err)
		}
	}
	if !regularFileMatches(serverFishSource(root), filepath.Join(home, ".config/fish/config.fish")) {
		t.Fatal("configuração Fish não foi instalada")
	}
	for _, name := range []string{".bashrc", ".bash_profile", ".profile", ".config/fish/functions/custom.fish"} {
		sdkTestContent(t, filepath.Join(home, name), "keep")
	}
}

func TestServerFishGreetingAndNoninteractiveSessions(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish não disponível")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		interactive, image bool
	}{
		{"noninteractive", false, true},
		{"SSH-with-image", true, true},
		{"SSH-without-image", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, bin := t.TempDir(), t.TempDir()
			log := filepath.Join(home, "fastfetch.log")
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("SSH_CLIENT", "192.168.1.104 12345 22")
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			t.Setenv("FETCH_LOG", log)
			writeTestExecutable(t, bin, "fastfetch", `printf '%s\n' "$*" >> "$FETCH_LOG"`)
			writeTestExecutable(t, bin, "chafa", "exit 0")
			if tc.image {
				sdkTestWrite(t, filepath.Join(home, ".face"), "image")
			}
			if err := installServerFish(root, home); err != nil {
				t.Fatal(err)
			}
			// Escape the path as Fish syntax; filenames may contain spaces.
			source := "source '" + strings.ReplaceAll(filepath.Join(home, ".config/fish/config.fish"), "'", "\\'") + "'"
			args := []string{"--no-config"}
			if tc.interactive {
				args = append(args, "--interactive")
				source += "; fish_greeting"
			}
			args = append(args, "-c", source)
			out, err := exec.Command(fish, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("Fish falhou: %v: %s", err, out)
			}
			data, readErr := os.ReadFile(log)
			if !tc.interactive {
				if len(out) != 0 || !os.IsNotExist(readErr) {
					t.Fatalf("sessão não interativa executou greeting: %q, %q, %v", out, data, readErr)
				}
				return
			}
			want := "--config " + filepath.Join(home, ".config/fastfetch/config.json")
			if !tc.image {
				want += " --logo-type builtin --logo ubuntu"
			}
			if readErr != nil || string(data) != want+"\n" {
				t.Fatalf("Fastfetch inesperado: %q, %v; esperado %q", data, readErr, want)
			}
		})
	}
}

func TestServerFishMenusAndLoginShell(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceProfile{deviceServer, deviceHomeProd, deviceDesktop, deviceNotebook} {
		m := newModelForDevice(root, device)
		found := false
		for _, current := range m.items() {
			if current.title != "Configurar Fish" {
				continue
			}
			found = true
			configured := current.job()
			if _, err := configured.steps[0].run(); err != nil {
				t.Fatal(err)
			}
			last := configured.steps[len(configured.steps)-1]
			username, err := currentUsername()
			if err != nil {
				t.Fatal(err)
			}
			if got := commandTail(t, last, 4); !slices.Equal(got, []string{"chsh", "-s", "/usr/bin/fish", username}) {
				t.Fatalf("shell de login incorreto: %v", got)
			}
		}
		if found != (device == deviceServer || device == deviceHomeProd) {
			t.Fatalf("menu Fish no perfil incorreto: %s", device)
		}
	}
}

func TestServerFishInstallerUsesAPTOnlyOnUbuntu(t *testing.T) {
	for _, osID := range []string{"ubuntu", "arch"} {
		t.Run(osID, func(t *testing.T) {
			bin, root := t.TempDir(), t.TempDir()
			log := filepath.Join(root, "apt.log")
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			t.Setenv("APT_LOG", log)
			writeTestExecutable(t, bin, "apt-get", `printf '%s\n' "$*" >> "$APT_LOG"`)
			osRelease := filepath.Join(root, "os-release")
			sdkTestWrite(t, osRelease, "ID="+osID+"\n")
			script := strings.NewReplacer("[[ $EUID == 0 ]]", "true", "/etc/os-release", osRelease).Replace(serverFishInstallScript)
			out, err := exec.Command("bash", "-c", script).CombinedOutput()
			data, readErr := os.ReadFile(log)
			if osID != "ubuntu" {
				if err == nil || !os.IsNotExist(readErr) {
					t.Fatalf("sistema incompatível executou instalação: %v, %s, %s", err, out, data)
				}
				return
			}
			if err != nil || readErr != nil || string(data) != "update\ninstall --yes fish chafa bat bsdextrautils\n" {
				t.Fatalf("instalação inesperada: %v, %s, %s", err, out, data)
			}
		})
	}
}
