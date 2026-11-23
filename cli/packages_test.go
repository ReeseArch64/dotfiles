package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newPackageInstallerTest(t *testing.T, answers string) (packageInstaller, *[]string) {
	t.Helper()
	var commands []string
	console := &sshConsole{
		home: t.TempDir(), input: bufio.NewReader(strings.NewReader(answers)), output: &bytes.Buffer{},
		commands: sshCommands{
			capture: func(_ string, args ...string) (string, error) {
				if len(args) == 3 && args[0] == "pacman" && args[1] == "-Q" {
					return "", errors.New("pacote não instalado")
				}
				t.Fatalf("comando inesperado: %v", args)
				return "", nil
			},
			interactive: func(args ...string) error {
				commands = append(commands, strings.Join(args, " "))
				return nil
			},
		},
	}
	return packageInstaller{
		console: console,
		which: func(name string) (string, error) {
			if name != "yay" {
				t.Fatalf("executável inesperado: %s", name)
			}
			return "/usr/bin/yay", nil
		},
		build: func(path string) error {
			t.Fatalf("compilação inesperada em %s", path)
			return nil
		},
	}, &commands
}

func TestPackageActionsInstallApprovedPackages(t *testing.T) {
	tests := []struct {
		action string
		want   []string
	}{
		{"essential-tools", []string{"sudo pacman -S --needed -- base-devel curl wget rate-mirrors"}},
		{"install-kitty", []string{"sudo pacman -S --needed -- kitty"}},
		{"install-artcraft-launcher", []string{"yay -S --needed -- artcraft-launcher-bin"}},
		{"install-android-studio", []string{"yay -S --needed -- android-studio"}},
		{"install-zed", []string{"sudo pacman -S --needed -- zed"}},
		{"install-firefox", []string{
			"sudo pacman -S --needed -- firefox xdg-utils",
			"env -u BROWSER xdg-settings set default-web-browser firefox.desktop",
		}},
		{"terminal-tools", []string{
			"sudo pacman -S --needed -- bat eza btop fzf ripgrep fd jq yq zoxide ncdu tealdeer wl-clipboard atuin starship yazi sshfs glow ffmpeg mpv yt-dlp scrcpy android-tools hurl hyperfine just cmake ninja",
			"yay -S --needed -- viddy ventoy scc usql-bin mprocs",
		}},
		{"install-yay", nil},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
			if err := installer.run(tt.action, ""); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(*commands, tt.want) {
				t.Fatalf("comandos = %q, esperado %q", *commands, tt.want)
			}
			if _, err := os.Stat(filepath.Join(installer.console.home, ".aur")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("yay disponível deveria evitar a preparação do clone: %v", err)
			}
		})
	}
	installer, commands := newPackageInstallerTest(t, "CANCELAR\n")
	if err := installer.run("terminal-tools", ""); err == nil || len(*commands) != 0 {
		t.Fatalf("cancelamento executou comandos: %v, %v", *commands, err)
	}
}

func TestInstallPostingReusesUV(t *testing.T) {
	const install = " tool install --python 3.13 posting<3"
	const pacman = "sudo pacman -S --needed -- uv"
	const mise = "/opt/mise/bin/mise"
	const uv = "/opt/mise/installs/uv/0.13.9/bin/uv"
	const prefix = "env MISE_AUTO_INSTALL=0 " + mise + " --cd /"
	for _, tc := range []struct {
		name, answer string
		mise, active bool
		path         bool
		want         []string
	}{
		{name: "mise ativo sem uv no PATH", mise: true, active: true, want: []string{prefix + " exec -- " + uv + install}},
		{name: "mise instalado e inativo", mise: true, want: []string{prefix + " exec uv@0.13.9 -- " + uv + install}},
		{name: "uv no PATH", path: true, want: []string{"/opt/uv/bin/uv" + install}},
		{name: "instalar uv ausente", want: []string{pacman, "/usr/bin/uv" + install}},
		{name: "cancelamento", answer: "CANCELAR", mise: true, active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := tc.answer
			if answer == "" {
				answer = "CONFIRMO"
			}
			installer, commands := newPackageInstallerTest(t, answer+"\n")
			installer.which = func(name string) (string, error) {
				switch {
				case name == "mise" && tc.mise:
					return mise, nil
				case name == "uv" && tc.path:
					return "/opt/uv/bin/uv", nil
				case name == "/usr/bin/uv" && slices.Contains(*commands, pacman):
					return name, nil
				}
				return "", exec.ErrNotFound
			}
			capture := installer.console.commands.capture
			installer.console.commands.capture = func(input string, args ...string) (string, error) {
				switch strings.Join(args, " ") {
				case prefix + " --quiet which uv":
					if tc.active {
						return uv + "\n", nil
					}
					return "", exec.ErrNotFound
				case prefix + " --quiet latest --installed uv":
					return "0.13.9\n", nil
				case prefix + " --quiet which uv --tool uv@0.13.9":
					return uv + "\n", nil
				default:
					if tc.mise || tc.path || tc.answer != "" {
						t.Fatalf("consulta de pacote indevida: %v", args)
					}
					return capture(input, args...)
				}
			}
			err := installer.run("install-posting", "")
			if (err != nil) != (tc.answer == "CANCELAR") || !slices.Equal(*commands, tc.want) {
				t.Fatalf("comandos = %v, esperado %v, erro = %v", *commands, tc.want, err)
			}
		})
	}
}

func TestInstallPostingRespectsCustomToolBinDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UV_TOOL_BIN_DIR", dir)
	installer, commands := newPackageInstallerTest(t, "")
	installer.which = func(name string) (string, error) {
		if name == filepath.Join(dir, "posting") {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	if err := installer.run("install-posting", ""); err != nil || len(*commands) != 0 {
		t.Fatalf("Posting já instalado: %v, %v", *commands, err)
	}
}

func TestInstallLunarVimReusesMiseAndPinnedNeovim(t *testing.T) {
	installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
	mise := filepath.Join(installer.console.home, ".local", "bin", "mise")
	installer.which = func(name string) (string, error) {
		if name == mise {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	capture := installer.console.commands.capture
	installer.console.commands.capture = func(input string, args ...string) (string, error) {
		switch {
		case slices.Equal(args, []string{"env", "MISE_AUTO_INSTALL=0", mise, "--cd", "/", "--quiet", "which", "nvim", "--tool", lunarVimNeovim}):
			return "/opt/mise/neovim/bin/nvim\n", nil
		case slices.Equal(args, []string{"pacman", "-Q", "mise"}):
			t.Fatal("Mise existente não deve ser consultado no Pacman")
		}
		return capture(input, args...)
	}
	download := errors.New("download interrompido para o teste")
	interactive := installer.console.commands.interactive
	installer.console.commands.interactive = func(args ...string) error {
		if args[0] == mise {
			t.Fatalf("Neovim existente não deve ser reinstalado: %v", args)
		}
		if err := interactive(args...); err != nil {
			return err
		}
		if args[0] == "curl" {
			return download
		}
		return nil
	}
	err := installer.run("install-lunarvim", "")
	if !errors.Is(err, download) || len(*commands) != 2 || (*commands)[0] != "sudo pacman -S --needed -- git curl base-devel ripgrep fd" {
		t.Fatalf("comandos = %v, erro = %v", *commands, err)
	}
}

func TestProtonPassChecksFirefoxAndStartsLogin(t *testing.T) {
	install := "yay -S --needed -- proton-pass-cli-bin"
	login := "pass-cli login"
	tests := []struct {
		name, mozilla, answer, fail, wantErr string
		firefoxMissing, alreadyInstalled     bool
		want                                 []string
	}{
		{name: "Firefox ausente", mozilla: "dir", firefoxMissing: true, wantErr: "instale o Firefox"},
		{name: "Mozilla ausente", wantErr: "abra o Firefox"},
		{name: "Mozilla é arquivo", mozilla: "file", wantErr: "precisa ser um diretório"},
		{name: "cancelamento", mozilla: "dir", answer: "CANCELAR", wantErr: "cancelad"},
		{name: "instalação seguida de login", mozilla: "dir", want: []string{install, login}},
		{name: "pacote já instalado", mozilla: "dir", alreadyInstalled: true, want: []string{login}},
		{name: "instalação falha", mozilla: "dir", fail: install, wantErr: "falha simulada", want: []string{install}},
		{name: "login falha", mozilla: "dir", fail: login, wantErr: "executar pass-cli login", want: []string{install, login}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := tt.answer
			if answer == "" {
				answer = "CONFIRMO"
			}
			installer, commands := newPackageInstallerTest(t, answer+"\n")
			path := filepath.Join(installer.console.home, ".config", "mozilla")
			switch tt.mozilla {
			case "dir":
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "file":
				sdkTestFile(t, path, "")
			}
			which, capture, run := installer.which, installer.console.commands.capture, installer.console.commands.interactive
			installer.which = func(name string) (string, error) {
				if name == "firefox" {
					if tt.firefoxMissing {
						return "", exec.ErrNotFound
					}
					return "/usr/bin/firefox", nil
				}
				return which(name)
			}
			installer.console.commands.capture = func(input string, args ...string) (string, error) {
				if tt.alreadyInstalled && slices.Equal(args, []string{"pacman", "-Q", "proton-pass-cli-bin"}) {
					return "proton-pass-cli-bin 1.0", nil
				}
				return capture(input, args...)
			}
			installer.console.commands.interactive = func(args ...string) error {
				if err := run(args...); err != nil {
					return err
				}
				if strings.Join(args, " ") == tt.fail {
					return errors.New("falha simulada")
				}
				return nil
			}
			err := installer.run("proton-pass", "")
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("erro = %v, esperado %q", err, tt.wantErr)
			}
			if !slices.Equal(*commands, tt.want) {
				t.Fatalf("comandos = %q, esperado %q", *commands, tt.want)
			}
		})
	}
}

func TestDriversFindHeadersForInstalledKernels(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
		modules := t.TempDir()
		kernels := map[string]string{
			"6.18-cachyos": "linux-cachyos",
			"6.12-lts":     "linux-cachyos-lts",
			"6.12.1-lts":   "linux-cachyos-lts",
		}
		for release := range kernels {
			sdkTestFile(t, filepath.Join(modules, release, "vmlinuz"), "imagem de teste")
		}
		sdkTestFile(t, filepath.Join(modules, "outro", "modules.dep"), "")
		capture := installer.console.commands.capture
		installer.console.commands.capture = func(input string, args ...string) (string, error) {
			switch args[1] {
			case "-Qqo":
				return kernels[filepath.Base(filepath.Dir(args[3]))] + "\n", nil
			case "-Si":
				if unavailable && args[3] == "linux-cachyos-headers" {
					return "", errors.New("não consta nos repositórios")
				}
				return "disponível", nil
			default:
				return capture(input, args...)
			}
		}
		err := installer.run("drivers", modules)
		if unavailable {
			if err == nil || !strings.Contains(err.Error(), "linux-cachyos-headers") || len(*commands) != 0 {
				t.Fatalf("headers indisponíveis: %v, %v", *commands, err)
			}
			continue
		}
		want := []string{"sudo pacman -S --needed -- linux-cachyos-lts-headers linux-cachyos-headers base-devel vulkan-tools mesa-utils"}
		if err != nil || !slices.Equal(*commands, want) {
			t.Fatalf("headers: %v, %v", *commands, err)
		}
	}
}

func TestYayBootstrap(t *testing.T) {
	for _, state := range []string{"clone novo", "clone existente", "clone alterado", "sem git"} {
		t.Run(state, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
			target := filepath.Join(installer.console.home, ".aur", "yay")
			ready, gitReady := false, state != "sem git"
			installer.which = func(name string) (string, error) {
				if (name == "git" && gitReady) || (name == "yay" && ready) {
					return "/usr/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			createClone := func() {
				if err := os.MkdirAll(filepath.Join(target, ".git"), 0700); err != nil {
					t.Fatal(err)
				}
				sdkTestFile(t, filepath.Join(target, "PKGBUILD"), "")
			}
			if state == "clone existente" || state == "clone alterado" {
				createClone()
			}
			capture, run := installer.console.commands.capture, installer.console.commands.interactive
			installer.console.commands.capture = func(input string, args ...string) (string, error) {
				if args[0] == "git" {
					switch args[3] {
					case "config":
						return yayRepository + "\n", nil
					case "status":
						if state == "clone alterado" {
							return " M PKGBUILD\n", nil
						}
						return "", nil
					}
				}
				return capture(input, args...)
			}
			installer.console.commands.interactive = func(args ...string) error {
				if slices.Equal(args, []string{"sudo", "pacman", "-S", "--needed", "--", "git"}) {
					gitReady = true
				}
				if args[0] == "git" {
					if want := []string{"git", "clone", "--", yayRepository, target}; !slices.Equal(args, want) {
						t.Fatalf("clone = %q, esperado %q", args, want)
					}
					createClone()
				}
				return run(args...)
			}
			installer.build = func(directory string) error {
				if directory != target {
					t.Fatalf("compilação em %s", directory)
				}
				*commands = append(*commands, "makepkg -si")
				ready = true
				return nil
			}
			err := installer.run("install-yay", "")
			switch state {
			case "clone alterado":
				if err == nil || len(*commands) != 0 {
					t.Fatalf("instalação deveria falhar: %q, %v", *commands, err)
				}
			default:
				want := []string{"sudo pacman -S --needed -- base-devel", "makepkg -si"}
				if state != "clone existente" {
					want = slices.Insert(want, 1, "git clone -- "+yayRepository+" "+target)
				}
				if state == "sem git" {
					want = slices.Insert(want, 0, "sudo pacman -S --needed -- git")
				}
				if err != nil || !slices.Equal(*commands, want) {
					t.Fatalf("instalação = %q, erro %v, esperado %q", *commands, err, want)
				}
			}
		})
	}
}

func TestAURDirectoryUsesSDKLink(t *testing.T) {
	home, sdk := t.TempDir(), t.TempDir()
	destination := filepath.Join(sdk, "Aur")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".aur")
	if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
	path, err := aurDirectory(home, sdk)
	if err != nil || path != link {
		t.Fatalf("pasta AUR = %s, erro = %v", path, err)
	}
	if _, err := aurDirectory(home, t.TempDir()); err == nil {
		t.Fatal("aceitou link para fora da pasta SDK")
	}
}
