package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestMprocsFailureDoesNotBlockOtherTerminalTools(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "pacman", `[ "$1" = "-Q" ] && [ "$2" != "mprocs" ]`)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	configured := terminalToolsJobFor(t.TempDir(), t.TempDir(), func(string) (string, error) { return "/usr/bin/lvim", nil })
	var index int = -1
	for i, current := range configured.steps {
		if current.label == "Instalar mprocs via Shelly" {
			index = i
			break
		}
	}
	if index < 1 || index+1 >= len(configured.steps) {
		t.Fatalf("passo mprocs ausente ou sem continuação: %v", jobStepLabels(configured))
	}
	r := newJobRun(configured)
	r.cur = index
	r.states[index] = stepRunning
	if r.handle(stepDoneMsg{err: errors.New("makepkg falhou")}) == nil && !r.done {
		t.Fatal("próximo passo não executou")
	}
	if r.failed || r.states[index] != stepIgnored || r.cur <= index {
		t.Fatalf("falha do mprocs bloqueou instalação: failed=%v states=%v cur=%d", r.failed, r.states, r.cur)
	}
	if configured.steps[index-1].label != "Atualizar bases do Pacman" || !configured.steps[index-1].optional {
		t.Fatal("atualização exclusiva do mprocs também precisa ser opcional")
	}
	if configured.steps[index+1].optional {
		t.Fatal("outras ferramentas não podem ficar opcionais")
	}
}

func TestMprocsOptionalInstallerReturnsFailureWithoutWaiting(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "pacman", `exit 1`)
	writeTestExecutable(t, bin, "shelly", `exit 1`)
	writeTestExecutable(t, bin, "git", `exit 1`)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TERM", "xterm-256color")
	steps := optionalMprocsSteps()
	if len(steps) != 2 || !steps[0].optional || !steps[1].optional {
		t.Fatalf("etapas opcionais incorretas: %v", jobStepLabels(job{steps: steps}))
	}
	if got := commandTail(t, steps[1], 6); !slices.Equal(got, []string{"bash", "-c", shellyFallbackScript, "dotfiles-shelly-fallback", "aur", "mprocs"}) {
		t.Fatalf("instalação mprocs inesperada: %v", got)
	}
	out, err := steps[1].cmd().CombinedOutput()
	if err == nil || !strings.Contains(string(out), "Etapa opcional falhou; continuando a configuração") || strings.Contains(string(out), "Pressione qualquer tecla") {
		t.Fatalf("falha não foi ignorada sem prompt: err=%v out=%s", err, out)
	}
}

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	wantShelly := []string{"neovim", "scc"}
	if !slices.Equal(terminalToolPackages, wantShelly) {
		t.Fatalf("pacotes Shelly inesperados: %v", terminalToolPackages)
	}
	wantAur := []string{"viddy", "mprocs", "posting", "usql-bin", "proton-pass-cli-bin"}
	if !slices.Equal(terminalToolAurPackages, wantAur) {
		t.Fatalf("pacotes AUR inesperados: %v", terminalToolAurPackages)
	}
	wantPacman := []string{"yazi", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "btop", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "git-delta", "ventoy", "eza", "bat", "wget", "curl"}
	if !slices.Equal(terminalToolPacmanPackages, wantPacman) {
		t.Fatalf("pacotes Pacman inesperados: %v", terminalToolPacmanPackages)
	}
}

func TestTerminalToolsExcludeUnsupportedViddyBinPackage(t *testing.T) {
	if slices.Contains(terminalToolAurPackages, "viddy-bin") {
		t.Fatal("viddy-bin usa uma expansão de array incompatível com o Shelly")
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
	})
	if len(configured.steps) == 0 || configured.steps[len(configured.steps)-1].label != "Instalar LunarVim" {
		t.Fatal("instalação do LunarVim ausente")
	}
}

func TestTerminalToolsJobConfiguresFastfetchAndBtop(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	})
	want := map[string]bool{
		"Copiar configuração para ~/.config/fastfetch": false,
		"Copiar configuração para ~/.config/btop":      false,
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
	})
	for _, current := range configured.steps {
		if current.label == "Instalar LunarVim" {
			t.Fatal("LunarVim instalado novamente")
		}
	}
}
