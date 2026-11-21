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

func TestDesktopYaziRemoteAccess(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish não disponível")
	}
	content, err := os.ReadFile("../../setup/desktop/fish/config.fish")
	if err != nil {
		t.Fatal(err)
	}
	_, functions, found := strings.Cut(string(content), "# Yazi: pastas pessoais remotas via SSHFS (somente desktop).\n")
	if !found {
		t.Fatal("funções de acesso remoto ausentes")
	}
	for _, tc := range []struct {
		name, command, host                      string
		mounted, foreign, failMount, failUnmount bool
		want                                     string
		failed                                   bool
	}{
		{name: "homelab", command: "yhomelab", host: "homelab", want: "mount\nyazi\n"},
		{name: "notebook", command: "ynotebook", host: "notebook", want: "mount\nyazi\n"},
		{name: "reuse", command: "yhomelab", host: "homelab", mounted: true, want: "yazi\n"},
		{name: "failed-mount", command: "ynotebook", host: "notebook", failMount: true, want: "mount\n", failed: true},
		{name: "foreign-mount", command: "yhomelab", host: "homelab", mounted: true, foreign: true, failed: true},
		{name: "unmount", command: "yremote --unmount notebook", host: "notebook", mounted: true, want: "unmount\n"},
		{name: "busy-unmount", command: "yremote --unmount notebook", host: "notebook", mounted: true, failUnmount: true, want: "unmount\n", failed: true},
		{name: "already-unmounted", command: "yremote --unmount homelab", host: "homelab"},
		{name: "invalid-host", command: "yremote homeprod", failed: true},
		{name: "missing-host", command: "yremote", failed: true},
		{name: "extra-arguments", command: "yremote homelab notebook", failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			env := []string{"HOME=" + home, "EXPECTED_HOST=" + tc.host}
			for name, enabled := range map[string]bool{"MOUNTED": tc.mounted, "FOREIGN": tc.foreign, "FAIL_MOUNT": tc.failMount, "FAIL_UNMOUNT": tc.failUnmount} {
				value := "0"
				if enabled {
					value = "1"
				}
				env = append(env, name+"="+value)
			}
			mocks := `
function mountpoint
    test "$argv[3]" = "$HOME/mnt/$EXPECTED_HOST"; or return 99
    test "$MOUNTED" = 1
end
function findmnt
    test "$argv[3]" = "$HOME/mnt/$EXPECTED_HOST"; or return 99
    test "$argv[7]" = "$EXPECTED_HOST:"; or return 99
    test "$FOREIGN" = 0
end
function sshfs
    echo mount
    test "$argv[1]" = "$EXPECTED_HOST:"; or return 99
    test "$argv[2]" = "$HOME/mnt/$EXPECTED_HOST"; or return 99
    test -d "$argv[2]"; or return 99
    test "$FAIL_MOUNT" = 0
end
function yazi
    echo yazi
    test "$argv[1]" = "$HOME/mnt/$EXPECTED_HOST"
end
function fusermount3
    echo unmount
    test "$argv[1]" = -u; or return 99
    test "$argv[2]" = "$HOME/mnt/$EXPECTED_HOST"; or return 99
    test "$FAIL_UNMOUNT" = 0
end
`
			cmd := exec.Command(fish, "--no-config", "-c", mocks+functions+"\n"+tc.command)
			cmd.Env = append(os.Environ(), env...)
			out, err := cmd.Output()
			if (err != nil) != tc.failed || string(out) != tc.want {
				t.Fatalf("saída=%q erro=%v; esperado saída=%q falha=%v", out, err, tc.want, tc.failed)
			}
		})
	}
}
