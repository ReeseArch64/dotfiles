package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMiseLoginMenus(t *testing.T) {
	for _, hostname := range []string{"desktop", "laptop", "homelab", "homeprod"} {
		menu := newMenu(hostname)
		menu.screen = screenProgramming
		entry := menu.items()[len(menu.items())-1]
		supported := hostname == "desktop" || hostname == "laptop"
		if entry.goTo != screenMiseLogins || entry.disabled == supported {
			t.Fatalf("submenu de login para %s: %+v", hostname, entry)
		}
		if !supported {
			continue
		}
		next, _ := menu.selectItem(entry)
		list := next.(model)
		want := []string{"AWS", "Google Cloud", "Azure", "Railway", "GitHub", "GitLab", "Heroku", "Firebase", "Cloudflare CLI", "Cloudflare Wrangler", "Vercel", "Neon", "npm"}
		if got := menuTitles(list.items()); !slices.Equal(got, want) {
			t.Fatalf("ferramentas no menu: %v", got)
		}
		for index, item := range list.items() {
			next, _ := list.selectItem(item)
			page := next.(model)
			actions := page.items()
			if len(actions) != 1 || actions[0].title != "Fazer login" || actions[0].disabled || screenTitle(page.screen) != item.title {
				t.Fatalf("submenu de %s: %v", item.title, actions)
			}
			command, err := actionCommand(actions[0].action, actions[0].title)
			if err != nil || !slices.Equal(command.Args[1:], []string{"--mise-login", miseLogins[index].argv[0]}) {
				t.Fatalf("ação de %s: %v, %v", item.title, command, err)
			}
		}
		list.height = 18
		last, _ := list.handleKey("up")
		view := last.(model).View()
		if !strings.Contains(view, "npm login") || lipgloss.Height(view) != list.height {
			t.Fatalf("último login não aparece no terminal compacto: %s", view)
		}
	}
}

func TestMiseLoginDispatch(t *testing.T) {
	for _, tt := range []struct {
		name      string
		page      screen
		lookupErr error
		installed string
		wantArgs  []string
		wantErr   string
	}{
		{"GitHub", screenLoginGitHub, nil, "", []string{"--", "/mise/bin/gh", "auth", "login"}, ""},
		{"GitLab", screenLoginGitLab, nil, "", []string{"--", "/mise/bin/glab", "auth", "login"}, ""},
		{"Heroku instalado e inativo", screenLoginHeroku, errors.New("não está ativo"), "11.0.0", []string{"heroku@11.0.0", "--", "/mise/bin/heroku", "login"}, ""},
		{"GitHub ausente", screenLoginGitHub, errors.New("ferramenta ausente"), "", nil, "executável gh não encontrado"},
		{"AWS instalada e inativa", screenLoginAWS, errors.New("não está ativo"), "2.33.0", []string{"awscli@2.33.0", "--", "/mise/bin/aws", "login"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			login, ok := miseLoginForScreen(tt.page)
			if !ok {
				t.Fatal("login ausente")
			}
			t.Setenv("PATH", t.TempDir())
			prefix := []string{"env", "MISE_AUTO_INSTALL=0", "mise", "--cd", "/"}
			var invoked []string
			console := sshConsole{
				commands: sshCommands{
					capture: func(_ string, args ...string) (string, error) {
						path := "/mise/bin/" + login.argv[0] + "\n"
						switch {
						case slices.Equal(args, append(prefix, "--quiet", "which", login.argv[0])):
							return path, tt.lookupErr
						case slices.Equal(args, append(prefix, "--quiet", "latest", "--installed", login.tool)):
							return tt.installed + "\n", nil
						case tt.installed != "" && slices.Equal(args, append(prefix, "--quiet", "which", login.argv[0], "--tool", login.tool+"@"+tt.installed)):
							return path, nil
						}
						t.Fatalf("resolução inesperada: %v", args)
						return "", nil
					},
					interactive: func(args ...string) error {
						invoked = args
						return nil
					},
				},
			}
			err := login.run(&console)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || invoked != nil {
					t.Fatalf("falha esperada %q, comando %v, erro %v", tt.wantErr, invoked, err)
				}
				return
			}
			want := append(append(prefix, "exec"), tt.wantArgs...)
			if err != nil || !slices.Equal(invoked, want) {
				t.Fatalf("execução: %v, erro %v, esperava %v", invoked, err, want)
			}
		})
	}
}

func TestMiseLoginFallsBackToRealExecutableOnPATH(t *testing.T) {
	shims, bin := t.TempDir(), t.TempDir()
	t.Setenv("PATH", shims+string(os.PathListSeparator)+bin)
	mise, executable := filepath.Join(bin, "mise"), filepath.Join(bin, "az")
	for _, path := range []string{mise, executable} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(mise, filepath.Join(shims, "az")); err != nil {
		t.Fatal(err)
	}
	login, _ := miseLoginForScreen(screenLoginAzure)
	args, err := login.command(sshCommands{capture: func(string, ...string) (string, error) {
		return "", errors.New("ferramenta ausente no Mise")
	}})
	if err != nil || !slices.Equal(args, []string{executable}) {
		t.Fatalf("execução pelo PATH: %v, erro %v", args, err)
	}
}
