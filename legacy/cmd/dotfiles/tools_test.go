package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTerminalToolsUseSupportedMprocsAndPostingInstallers(t *testing.T) {
	configured := terminalToolsJobFor(t.TempDir(), t.TempDir(), func(name string) (string, error) {
		if name == "lvim" {
			return "/usr/bin/lvim", nil
		}
		return "", errors.New("ausente")
	}, deviceNotebook)
	labels := jobStepLabels(configured)
	mprocsIndex := slices.Index(labels, "Instalar mprocs via Dekit")
	postingIndex := slices.Index(labels, "Instalar Posting via uv")
	uvIndex := -1
	for i, current := range configured.steps {
		if current.cmd != nil && slices.Contains(current.cmd().Args, "pacman") && slices.Contains(current.cmd().Args, "uv") {
			uvIndex = i
			break
		}
	}
	if mprocsIndex < 0 || postingIndex < 0 {
		t.Fatalf("instaladores ausentes: %v", labels)
	}
	if uvIndex >= 0 && (uvIndex >= mprocsIndex || uvIndex >= postingIndex) {
		t.Fatalf("uv precisa ser instalado antes das ferramentas: %v", labels)
	}
	if got := commandTail(t, configured.steps[mprocsIndex], 3); !slices.Equal(got, []string{"bash", "-c", mprocsInstallCommand}) {
		t.Fatalf("instalação mprocs inesperada: %v", got)
	}
	wantPosting := []string{"uv", "tool", "install", "--python", "3.13", "posting"}
	if got := commandTail(t, configured.steps[postingIndex], len(wantPosting)); !slices.Equal(got, wantPosting) {
		t.Fatalf("instalação Posting inesperada: %v", got)
	}
}

func TestMprocsInstallerCreatesCompatibleCommand(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	writeTestExecutable(t, bin, "curl", `cat <<'SCRIPT'
#!/bin/sh
mkdir -p "$HOME/.local/bin"
printf '%s\n' '#!/bin/sh' 'printf "%s\\n" "$*" > "$HOME/dekit-args"' > "$HOME/.local/bin/dekit"
chmod 755 "$HOME/.local/bin/dekit"
SCRIPT`)
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("TERM", "xterm-256color")
	current := terminalStep("Instalar mprocs via Dekit", "bash", "-c", mprocsInstallCommand)
	if out, err := current.cmd().CombinedOutput(); err != nil {
		t.Fatalf("instalação falhou: %v: %s", err, out)
	}
	command := filepath.Join(home, ".local", "bin", "mprocs")
	if out, err := os.ReadFile(command); err != nil || !strings.Contains(string(out), `dekit" mprocs "$@"`) {
		t.Fatalf("comando compatível ausente: err=%v conteúdo=%q", err, out)
	}
	if out, err := os.ReadFile(filepath.Join(home, "dekit-args")); err == nil {
		t.Fatalf("Dekit executado durante instalação: %q", out)
	}
	if out, err := exec.Command(command, "--version").CombinedOutput(); err != nil {
		t.Fatalf("mprocs falhou: %v: %s", err, out)
	}
	args, err := os.ReadFile(filepath.Join(home, "dekit-args"))
	if err != nil || strings.TrimSpace(string(args)) != "mprocs --version" {
		t.Fatalf("argumentos incompatíveis: err=%v args=%q", err, args)
	}
}

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	wantYay := []string{"neovim", "scc"}
	if !slices.Equal(terminalToolPackages, wantYay) {
		t.Fatalf("pacotes yay inesperados: %v", terminalToolPackages)
	}
	wantAur := []string{"viddy", "usql-bin", "proton-vpn-cli", "proton-pass-cli-bin"}
	if !slices.Equal(terminalToolAurPackages, wantAur) {
		t.Fatalf("pacotes AUR inesperados: %v", terminalToolAurPackages)
	}
	wantPacman := []string{"yazi", "sshfs", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "chafa", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "ninja", "git-delta", "ventoy", "eza", "bat", "wget", "curl", "uv"}
	if !slices.Equal(terminalToolPacmanPackages, wantPacman) {
		t.Fatalf("pacotes Pacman inesperados: %v", terminalToolPacmanPackages)
	}
}

func TestTerminalToolsExcludeUnsupportedViddyBinPackage(t *testing.T) {
	if slices.Contains(terminalToolAurPackages, "viddy-bin") {
		t.Fatal("usar o pacote viddy em vez de viddy-bin")
	}
}

func TestLunarVimInstallStepUsesReleaseBranch(t *testing.T) {
	current := lunarVimInstallStep()
	want := []string{"bash", "-c", lunarVimInstallCommand}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando LunarVim inesperado: %v", got)
	}
}

func TestTerminalToolsJobInstallsMissingLunarVim(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "", errors.New("ausente")
	}, deviceNotebook)
	if len(configured.steps) == 0 || configured.steps[len(configured.steps)-1].label != "Instalar LunarVim" {
		t.Fatal("instalação do LunarVim ausente")
	}
}

func TestTerminalToolsJobConfiguresFastfetch(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	}, deviceNotebook)
	want := map[string]bool{
		"Copiar configuração para ~/.config/fastfetch": false,
	}
	for _, current := range configured.steps {
		if _, ok := want[current.label]; ok {
			want[current.label] = true
		}
	}
	for label, found := range want {
		if !found {
			t.Fatalf("etapa ausente: %s", label)
		}
	}
}

func TestTerminalToolsJobKeepsExistingLunarVim(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	}, deviceNotebook)
	for _, current := range configured.steps {
		if current.label == "Instalar LunarVim" {
			t.Fatal("LunarVim instalado novamente")
		}
	}
}

func TestDesktopTerminalToolsInstallOnlyPendingTools(t *testing.T) {
	m := newModel(t.TempDir())
	m.screen = screenDevelopment
	for _, entry := range m.items() {
		if entry.title != "Ferramentas de terminal" {
			continue
		}
		labels := jobStepLabels(entry.job())
		for _, label := range []string{"Copiar configuração para ~/.config/fastfetch", "Instalar viddy via yay", "Instalar proton-pass-cli-bin via yay"} {
			if slices.Contains(labels, label) {
				t.Fatalf("etapa já migrada no Desktop: %s", label)
			}
		}
		if slices.Contains(labels, "Instalar neovim via yay") && !slices.Contains(labels, "Instalar LunarVim") {
			t.Fatalf("Neovim aparece fora da instalação do LunarVim: %v", labels)
		}
		for _, pkg := range missingPkgs("scc", "usql-bin", "proton-vpn-cli") {
			if !slices.Contains(labels, yayInstallStep(pkg).label) {
				t.Fatalf("pacote AUR pendente ausente no Desktop: %s", pkg)
			}
		}
		official := missingPkgs("hurl", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "hyperfine", "just", "cmake", "ninja", "ventoy", "curl", "uv")
		if len(official) != 0 && !slices.Contains(labels, pacmanInstallStep(official...).label) {
			t.Fatalf("pacotes Pacman no Desktop: %v", labels)
		}
		return
	}
	t.Fatal("ação de ferramentas ausente")
}
