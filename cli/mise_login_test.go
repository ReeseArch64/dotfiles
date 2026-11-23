package main

import (
	"bufio"
	"errors"
	"io"
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
		want := []string{"AWS", "Google Cloud", "Azure", "Railway", "GitHub", "Firebase", "Cloudflare CLI", "Cloudflare Wrangler", "Vercel", "Neon", "npm", "pnpm", "uv"}
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
		if !strings.Contains(view, "uv auth login") || lipgloss.Height(view) != list.height {
			t.Fatalf("último login não aparece no terminal compacto: %s", view)
		}
	}
}

func TestMiseLoginDispatch(t *testing.T) {
	for _, tt := range []struct {
		name      string
		page      screen
		input     string
		lookupErr error
		wantArgs  []string
		wantErr   string
	}{
		{"GitHub", screenLoginGitHub, "", nil, []string{"/mise/bin/gh", "auth", "login"}, ""},
		{"uv", screenLoginUV, "packages.example.org\n", nil, []string{"/mise/bin/uv", "auth", "login", "--", "packages.example.org"}, ""},
		{"uv sem serviço", screenLoginUV, "\n", nil, nil, "informe o domínio"},
		{"GitHub ausente", screenLoginGitHub, "", errors.New("ferramenta ausente"), nil, "Programação → Mise"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			login, ok := miseLoginForScreen(tt.page)
			if !ok {
				t.Fatal("login ausente")
			}
			prefix := []string{"env", "MISE_GLOBAL_CONFIG_FILE=/config/mise/config.toml", "MISE_AUTO_INSTALL=0", "mise", "--cd", "/"}
			var invoked []string
			console := sshConsole{
				input:  bufio.NewReader(strings.NewReader(tt.input)),
				output: io.Discard,
				commands: sshCommands{
					capture: func(_ string, args ...string) (string, error) {
						want := append(prefix, "--quiet", "which", login.argv[0])
						if !slices.Equal(args, want) {
							t.Fatalf("resolução: %v, esperava %v", args, want)
						}
						return "/mise/bin/" + login.argv[0] + "\n", tt.lookupErr
					},
					interactive: func(args ...string) error {
						invoked = args
						return nil
					},
				},
			}
			err := login.run(&console, "/config/mise/config.toml")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || invoked != nil {
					t.Fatalf("falha esperada %q, comando %v, erro %v", tt.wantErr, invoked, err)
				}
				return
			}
			want := append(append(prefix, "exec", "--"), tt.wantArgs...)
			if err != nil || !slices.Equal(invoked, want) {
				t.Fatalf("execução: %v, erro %v, esperava %v", invoked, err, want)
			}
		})
	}
}
