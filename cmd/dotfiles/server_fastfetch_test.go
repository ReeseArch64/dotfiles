package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestServerFastfetchAndFaceMenu(t *testing.T) {
	for _, device := range []deviceProfile{deviceServer, deviceHomeProd, deviceDesktop, deviceNotebook} {
		m := newModelForDevice("/repo", device)
		found := false
		for _, it := range m.items() {
			if it.title == "Fastfetch e foto de perfil" {
				found = true
				if it.job == nil {
					t.Fatal("ação sem job")
				}
			}
		}
		if found != (device == deviceServer || device == deviceHomeProd) {
			t.Fatalf("Fastfetch para servidor no perfil incorreto: %s", device)
		}
	}
}

func TestServerFastfetchAndFaceAppliesUserConfiguration(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		home := t.TempDir()
		root := filepath.Dir(filepath.Dir(setupDir))
		installed := false
		configured := serverFastfetchJobFor(root, setupDir, home, []step{nativeStep("Instalar Fastfetch", func() error {
			installed = true
			return nil
		})})
		want := []string{"Validar configuração Fastfetch e imagem de perfil", "Instalar Fastfetch", "Copiar configuração para ~/.config/fastfetch", "Criar symlink ~/.face"}
		if got := jobStepLabels(configured); !slices.Equal(got, want) {
			t.Fatalf("passos inesperados: %v", got)
		}
		for range 2 {
			for _, current := range configured.steps {
				if _, err := current.run(); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !installed {
			t.Fatal("instalação ignorada")
		}
		source := filepath.Join(setupDir, "fastfetch", "config.json")
		if configPath(setupDir, "fastfetch", "config.json") != source {
			t.Fatal("servidor não usa configuração própria")
		}
		if !regularFileMatches(source, filepath.Join(home, ".config", "fastfetch", "config.json")) {
			t.Fatal("configuração do Fastfetch não foi copiada como arquivo regular")
		}
		if target, err := os.Readlink(filepath.Join(home, ".face")); err != nil || target != assetPath(root, "face.jpg") {
			t.Fatalf("foto de perfil incorreta: %s, %v", target, err)
		}
	}, deviceServer, deviceHomeProd)
}

func TestServerFastfetchPreflightRejectsMissingSources(t *testing.T) {
	for _, missing := range []string{"config", "face"} {
		t.Run(missing, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			setupDir := setupDirectory(root, deviceHomeProd)
			if missing != "config" {
				writeSharedConfigTestFile(t, filepath.Join(setupDir, "fastfetch", "config.json"), "{}")
			}
			if missing != "face" {
				writeSharedConfigTestFile(t, assetPath(root, "face.jpg"), "image")
			}
			j := serverFastfetchJobFor(root, setupDir, home, nil)
			if _, err := j.steps[0].run(); err == nil {
				t.Fatal("origem ausente deveria impedir instalação")
			}
			if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
				t.Fatal("validação alterou o diretório do usuário")
			}
		})
	}
}

func TestServerFastfetchConfigsAreHeadlessAndDetectOS(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		path := configPath(setupDir, "fastfetch", "config.json")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var config struct{ Modules []string }
		if err := json.Unmarshal(content, &config); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(config.Modules, "os") || !slices.Contains(config.Modules, "uptime") {
			t.Fatal("configuração deve detectar o sistema e informar uptime")
		}
		for _, module := range []string{"display", "wm", "icons", "font", "cursor", "battery", "publicip"} {
			if slices.Contains(config.Modules, module) {
				t.Fatalf("módulo inadequado para servidor: %s", module)
			}
		}
		if _, err := exec.LookPath("fastfetch"); err == nil {
			if output, err := exec.Command("fastfetch", "--config", path, "--format", "json").CombinedOutput(); err != nil {
				t.Fatalf("Fastfetch rejeitou configuração: %s: %v", output, err)
			}
		}
	}, deviceServer, deviceHomeProd)
}

func TestServerFastfetchRendersFaceWithoutTerminalPixelDimensions(t *testing.T) {
	fastfetch, err := exec.LookPath("fastfetch")
	if err != nil {
		t.Skip("fastfetch não disponível")
	}
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		bin, home := t.TempDir(), t.TempDir()
		log := filepath.Join(home, "chafa.log")
		t.Setenv("HOME", home)
		t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
		t.Setenv("CHAFA_LOG", log)
		t.Setenv("TERM", "dumb")
		t.Setenv("SSH_CLIENT", "192.168.1.104 12345 22")
		writeTestExecutable(t, bin, "chafa", `printf '%s\n' "$@" > "$CHAFA_LOG"
printf 'FACE_RENDERED\n'`)
		output, err := exec.Command(fastfetch, "--config", configPath(setupDir, "fastfetch", "config.json"), "--structure", "title", "--pipe", "false", "--show-errors").CombinedOutput()
		if err != nil || !strings.Contains(string(output), "FACE_RENDERED") || strings.Contains(string(output), "Logo:") {
			t.Fatalf("logo não renderizado sem dimensões em pixels: %v: %s", err, output)
		}
		args, err := os.ReadFile(log)
		want := "--format\nsymbols\n--symbols\nblock\n--colors\nfull\n--size\n30x15\n--\n" + filepath.Join(home, ".face") + "\n"
		if err != nil || string(args) != want {
			t.Fatalf("Chafa não recebeu ~/.face: %v: %q", err, args)
		}
	}, deviceServer, deviceHomeProd)
}

func TestServerFastfetchInstallerIsVerifiedAndUbuntuOnly(t *testing.T) {
	for _, tc := range []struct{ name, osID, arch, fail string }{
		{"amd64", "ubuntu", "amd64", ""},
		{"arm64", "ubuntu", "arm64", ""},
		{"wrong OS", "debian", "amd64", ""},
		{"wrong architecture", "ubuntu", "riscv64", ""},
		{"APT failure", "ubuntu", "amd64", "apt"},
		{"download failure", "ubuntu", "amd64", "curl"},
		{"checksum mismatch", "ubuntu", "amd64", "checksum"},
		{"package failure", "ubuntu", "amd64", "package"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, work := t.TempDir(), t.TempDir()
			log := filepath.Join(work, "commands")
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			t.Setenv("COMMAND_LOG", log)
			t.Setenv("TEST_ARCH", tc.arch)
			t.Setenv("TEST_FAIL", tc.fail)
			writeTestExecutable(t, bin, "dpkg", `printf '%s\n' "$TEST_ARCH"`)
			writeTestExecutable(t, bin, "apt-get", `echo "apt-get $*" >> "$COMMAND_LOG"
[ "$TEST_FAIL" != apt ] || exit 1
case "$*" in *fastfetch-linux*) [ "$TEST_FAIL" != package ] ;; esac`)
			writeTestExecutable(t, bin, "curl", `echo "curl $*" >> "$COMMAND_LOG"
[ "$TEST_FAIL" != curl ] || exit 1
for last; do :; done
if [ "$TEST_FAIL" = checksum ]; then printf 'corrupt' > "$last"; else printf 'fixture package' > "$last"; fi`)
			writeTestExecutable(t, bin, "fastfetch", `echo "fastfetch $*" >> "$COMMAND_LOG"`)
			osRelease := filepath.Join(work, "os-release")
			writeSharedConfigTestFile(t, osRelease, "ID="+tc.osID+"\n")
			// Substitute only privileged OS detection and the fixture's expected hash.
			// APT and downloads are mocks; checksum verification uses real sha256sum.
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture package")))
			script := strings.NewReplacer(
				"[[ $EUID == 0 ]]", "true",
				"/etc/os-release", osRelease,
				"06e9a1be9a863dc815f8a31a6b1abec7b29967be30084ffcc29c1bca072d801b", hash,
				"25053651486d97b8731dd6bc59e725944e39adcdcd903741334c1c3282a7281d", hash,
			).Replace(serverFastfetchInstallScript)
			output, err := exec.Command("bash", "-c", script).CombinedOutput()
			wantSuccess := tc.osID == "ubuntu" && tc.arch != "riscv64" && tc.fail == ""
			if (err == nil) != wantSuccess {
				t.Fatalf("resultado inesperado: %s: %v", output, err)
			}
			commands, _ := os.ReadFile(log)
			if tc.osID != "ubuntu" || tc.arch == "riscv64" {
				if len(commands) != 0 {
					t.Fatalf("plataforma incompatível executou instalação: %s", commands)
				}
				return
			}
			installedPackage := strings.Contains(string(commands), "apt-get install --yes /")
			if tc.fail != "package" && installedPackage != wantSuccess {
				t.Fatalf("pacote instalado sem download verificado: %s", commands)
			}
			if wantSuccess {
				arch := "amd64"
				if tc.arch == "arm64" {
					arch = "aarch64"
				}
				if !strings.Contains(string(commands), "/2.70.0/fastfetch-linux-"+arch+".deb") || !strings.Contains(string(commands), "fastfetch --version") {
					t.Fatalf("pacote ou verificação incorretos: %s", commands)
				}
			}
		})
	}
}
