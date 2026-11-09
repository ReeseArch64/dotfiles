package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFishConfigCopiesFileAndRemovesObsoleteShellFiles(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "fish", "config.fish")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("mise activate fish | source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range obsoleteShellFiles {
		if err := os.WriteFile(filepath.Join(home, name), []byte("old\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	preserved := filepath.Join(home, ".profile")
	if err := os.WriteFile(preserved, []byte("keep\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installFishConfig(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "fish", "config.fish")
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "mise activate fish | source\n" {
		t.Fatalf("configuração inesperada: %q", content)
	}
	removed := append([]string{legacy}, obsoleteShellFiles...)
	for _, path := range removed {
		if !filepath.IsAbs(path) {
			path = filepath.Join(home, path)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("arquivo obsoleto não foi removido em %s: %v", path, err)
		}
	}
	if content, err := os.ReadFile(preserved); err != nil || string(content) != "keep\n" {
		t.Fatalf("arquivo não relacionado foi alterado: conteúdo=%q erro=%v", content, err)
	}
}

func TestRepositoryFishAliases(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish não disponível")
	}
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		content, err := os.ReadFile(configPath(setupDir, "fish", "config.fish"))
		if err != nil {
			t.Fatal(err)
		}
		var aliases []string
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "alias pu=") {
				t.Fatal("alias obsoleto pu ainda existe")
			}
			if strings.HasPrefix(line, "alias gpa=") || strings.HasPrefix(line, "alias piu=") {
				aliases = append(aliases, line)
			}
		}
		if len(aliases) != 2 {
			t.Fatalf("aliases esperados ausentes: %v", aliases)
		}
		for _, tc := range []struct {
			name, command, failedRemote, want string
		}{
			{"push-all", "gpa", "", "git push origin\ngit push ogle\ngit push backup\n"},
			{"push-origin-fails", "gpa", "origin", "git push origin\n"},
			{"push-ogle-fails", "gpa", "ogle", "git push origin\ngit push ogle\n"},
			{"update-pi", "piu", "", "pi update --all\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				script := `function git
    echo git $argv
    if test "$argv[2]" = "$FAILED_REMOTE"
        return 1
    end
end
function pi
    echo pi $argv
end
` + strings.Join(aliases, "\n") + "\n" + tc.command
				cmd := exec.Command(fish, "--no-config", "-c", script)
				cmd.Env = append(os.Environ(), "FAILED_REMOTE="+tc.failedRemote)
				out, err := cmd.CombinedOutput()
				if (err != nil) != (tc.failedRemote != "") || string(out) != tc.want {
					t.Fatalf("execução inesperada: saída=%q erro=%v", out, err)
				}
			})
		}
	})
}

func TestRepositoryFishConfigExists(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		path := configPath(setupDir, "fish", "config.fish")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("configuração Fish não é arquivo regular: %s", path)
		}
	})
}
