package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPreinstalledAppsMenu(t *testing.T) {
	for _, tt := range []struct {
		hostname, osID string
		allowed        bool
	}{
		{"desktop", "cachyos", true}, {"laptop", "cachyos", true},
		{"desktop", "arch", false}, {"homelab", "ubuntu", false}, {"homeprod", "ubuntu", false},
	} {
		menu := newMenu(tt.hostname)
		menu.screen, menu.osID = screenSystem, tt.osID
		items := menu.items()
		entry := items[len(items)-1]
		if entry.goTo != screenPreinstalled || entry.disabled == tt.allowed {
			t.Fatalf("submenu no perfil %s/%s: %+v", tt.hostname, tt.osID, entry)
		}
		if !tt.allowed {
			continue
		}
		next, _ := menu.selectItem(entry)
		item := next.(model).items()[0]
		command, err := actionCommand(item.action, item.title)
		if item.disabled || err != nil || !slices.Equal(command.Args[1:], []string{"--remove-preinstalled"}) {
			t.Fatalf("ação de remoção: %v, %v", command, err)
		}
	}
}

func TestRemovePreinstalledApps(t *testing.T) {
	failure := errors.New("pacman falhou")
	for _, tt := range []struct {
		name, listing, answer string
		queryErr, removeErr   error
		runPacman, clear      bool
	}{
		{"Kitty ausente", "alacritty\n", "CONFIRMO\n", nil, nil, false, false},
		{"consulta falhou", "", "CONFIRMO\n", failure, nil, false, false},
		{"cancelado", "kitty\nalacritty\nshelly\n", "CANCELAR\n", nil, nil, false, false},
		{"remoção falhou", "kitty\nalacritty\nshelly\n", "CONFIRMO\n", nil, failure, true, false},
		{"remove somente os instalados", "kitty\nalacritty\nshelly\nfirefox\n", "CONFIRMO\n", nil, nil, true, true},
		{"limpa restos de desinstalação", "kitty\nfirefox\n", "CONFIRMO\n", nil, nil, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home, cache, linkedData := t.TempDir(), t.TempDir(), t.TempDir()
			if err := os.Symlink(cache, filepath.Join(home, ".cache")); err != nil {
				t.Fatal(err)
			}
			alacritty := filepath.Join(home, ".config", "alacritty")
			meld := filepath.Join(home, ".local", "share", "meld")
			persist := filepath.Join(home, ".config", "kitty", "kitty.conf")
			cacheMeld := filepath.Join(cache, "meld")
			cacheLink := filepath.Join(cache, "alacritty")
			sdkTestFile(t, filepath.Join(alacritty, "config"), "alacritty")
			sdkTestFile(t, filepath.Join(meld, "config"), "meld")
			sdkTestFile(t, filepath.Join(cacheMeld, "data"), "cache do meld")
			sdkTestFile(t, filepath.Join(linkedData, "data"), "dados externos")
			sdkTestFile(t, persist, "kitty")
			if err := os.Symlink(linkedData, cacheLink); err != nil {
				t.Fatal(err)
			}
			var appRemoval []string
			console := sshConsole{home: home, input: bufio.NewReader(strings.NewReader(tt.answer)), output: &bytes.Buffer{}, commands: sshCommands{
				capture: func(_ string, args ...string) (string, error) {
					if !slices.Equal(args, []string{"pacman", "-Qq"}) {
						t.Fatalf("comando inesperado: %v", args)
					}
					return tt.listing, tt.queryErr
				},
				interactive: func(args ...string) error {
					appRemoval = args
					return tt.removeErr
				},
			}}
			if err := removePreinstalledApps(&console); (err == nil) != tt.clear {
				t.Fatalf("erro inesperado: %v", err)
			}
			var want []string
			if tt.runPacman {
				want = []string{"sudo", "pacman", "-Rns", "--", "alacritty", "shelly"}
			}
			if !slices.Equal(appRemoval, want) {
				t.Fatalf("remoção = %v, esperava %v", appRemoval, want)
			}
			for _, path := range []string{alacritty, meld, cacheMeld, cacheLink} {
				if _, err := os.Lstat(path); (tt.clear && !errors.Is(err, os.ErrNotExist)) || (!tt.clear && err != nil) {
					t.Fatalf("dados em %s: %v", path, err)
				}
			}
			sdkTestContent(t, persist, "kitty")
			sdkTestContent(t, filepath.Join(linkedData, "data"), "dados externos")
		})
	}
}
