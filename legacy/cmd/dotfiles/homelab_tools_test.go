package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomelabToolsMenu(t *testing.T) {
	for _, device := range []deviceProfile{deviceServer, deviceHomeProd, deviceDesktop, deviceNotebook} {
		m := newModelForDevice("/repo", device)
		found := false
		for _, it := range m.items() {
			if it.title != "Ferramentas de Administração" {
				continue
			}
			found = true
			j := it.job()
			want := []string{"sudo", "bash", "/repo/setup/homelab/tools/install.sh"}
			if got := commandTail(t, j.steps[len(j.steps)-1], len(want)); !slices.Equal(got, want) {
				t.Fatal(got)
			}
		}
		if found != (device == deviceServer) {
			t.Fatalf("ferramentas no perfil incorreto: %v", device)
		}
	}
}

func TestHomelabToolsInstaller(t *testing.T) {
	for _, tc := range []struct {
		name, osID, arch, fail string
		existing               bool
	}{
		{name: "amd64", osID: "ubuntu", arch: "amd64"},
		{name: "arm64", osID: "ubuntu", arch: "arm64"},
		{name: "preserve commands", osID: "ubuntu", arch: "amd64", existing: true},
		{name: "wrong OS", osID: "debian", arch: "amd64"},
		{name: "wrong architecture", osID: "ubuntu", arch: "riscv64"},
		{name: "APT failure", osID: "ubuntu", arch: "amd64", fail: "apt"},
		{name: "checksum failure", osID: "ubuntu", arch: "amd64", fail: "checksum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			destination := t.TempDir()
			log := filepath.Join(t.TempDir(), "commands")
			t.Setenv("PATH", destination+":"+bin+":"+os.Getenv("PATH"))
			t.Setenv("COMMAND_LOG", log)
			t.Setenv("TEST_ARCH", tc.arch)
			t.Setenv("TEST_FAIL", tc.fail)
			writeTestExecutable(t, bin, "dpkg", `printf '%s\n' "$TEST_ARCH"`)
			writeTestExecutable(t, bin, "apt-get", `echo "apt-get $*" >> "$COMMAND_LOG"
[ "$TEST_FAIL" != apt ]`)
			writeTestExecutable(t, bin, "sha256sum", `echo checksum >> "$COMMAND_LOG"
cat >/dev/null
[ "$TEST_FAIL" != checksum ]`)
			writeTestExecutable(t, bin, "curl", `echo "curl $*" >> "$COMMAND_LOG"
for last; do :; done
cp "$TEST_ARCHIVE" "$last"`)
			for _, name := range []string{"jq", "rg", "fzf", "git", "ncdu", "fdfind", "batcat"} {
				writeTestExecutable(t, bin, name, "exit 0")
			}
			if tc.existing {
				writeTestExecutable(t, destination, "fd", "# custom fd\nexit 0")
				writeTestExecutable(t, destination, "bat", "# custom bat\nexit 0")
			}
			fixture := t.TempDir()
			writeTestExecutable(t, fixture, "lazydocker", "exit 0")
			archive := filepath.Join(t.TempDir(), "release.tar.gz")
			if out, err := exec.Command("tar", "-czf", archive, "-C", fixture, "lazydocker").CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v: %s", err, out)
			}
			t.Setenv("TEST_ARCHIVE", archive)
			osRelease := filepath.Join(t.TempDir(), "os-release")
			if err := os.WriteFile(osRelease, []byte("ID="+tc.osID+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Sandbox all privileged paths; never run APT or install into the host.
			script := readHomelabFile(t, configPath(homelabSetup(t), "tools", "install.sh"))
			script = strings.NewReplacer(
				"[[ $EUID == 0 ]]", "true",
				"/etc/os-release", osRelease,
				"/usr/local/bin", destination,
				"/usr/bin/fdfind", filepath.Join(bin, "fdfind"),
				"/usr/bin/batcat", filepath.Join(bin, "batcat"),
				"command -v fd", "test -x "+filepath.Join(destination, "fd"),
				"command -v bat", "test -x "+filepath.Join(destination, "bat"),
			).Replace(script)
			out, err := exec.Command("bash", "-c", script).CombinedOutput()
			wantSuccess := tc.osID == "ubuntu" && tc.arch != "riscv64" && tc.fail == ""
			if (err == nil) != wantSuccess {
				t.Fatalf("resultado inesperado: %v: %s", err, out)
			}
			commands, _ := os.ReadFile(log)
			if tc.osID != "ubuntu" || tc.arch == "riscv64" {
				if len(commands) != 0 {
					t.Fatalf("sistema incompatível executou instalação: %s", commands)
				}
				return
			}
			if !strings.Contains(string(commands), "apt-get install --yes jq ripgrep fd-find fzf bat git ncdu") && tc.fail != "apt" {
				t.Fatal(string(commands))
			}
			_, statErr := os.Stat(filepath.Join(destination, "lazydocker"))
			if wantSuccess && statErr != nil {
				t.Fatal(statErr)
			}
			if !wantSuccess && !os.IsNotExist(statErr) {
				t.Fatal("falha não impediu a instalação do LazyDocker")
			}
			if wantSuccess {
				for _, name := range []string{"fd", "bat"} {
					info, err := os.Lstat(filepath.Join(destination, name))
					if err != nil {
						t.Fatal(err)
					}
					if (info.Mode()&os.ModeSymlink != 0) == tc.existing {
						t.Fatalf("comando %s não foi preservado/criado corretamente", name)
					}
				}
				arch := "x86_64"
				if tc.arch == "arm64" {
					arch = "arm64"
				}
				if !strings.Contains(string(commands), "lazydocker_0.25.2_Linux_"+arch+".tar.gz") {
					t.Fatal(string(commands))
				}
			}
		})
	}
}
