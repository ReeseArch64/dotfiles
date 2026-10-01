package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPGImportPathsFromEnvAcceptsMountedDirectory(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "run", "media", "usb")
	if err := os.MkdirAll(media, 0755); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(media, "public.asc")
	private := filepath.Join(media, "private.ASC")
	for _, path := range []string{public, private} {
		if err := os.WriteFile(path, []byte("key"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := "GPG_PUBLIC_IMPORT='" + public + "'\nGPG_PRIVATE_IMPORT=\"" + private + "\"\n"
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(env), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := gpgImportPathsFromEnv(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if paths.public != public || paths.private != private {
		t.Fatalf("caminhos inesperados: %#v", paths)
	}
}

func TestGPGImportPathsFromEnvRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "public.asc")
	if err := os.WriteFile(valid, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"variável ausente", "GPG_PUBLIC_IMPORT=" + valid + "\n", "GPG_PRIVATE_IMPORT"},
		{"mesmo arquivo", "GPG_PUBLIC_IMPORT=" + valid + "\nGPG_PRIVATE_IMPORT=" + valid + "\n", "arquivos diferentes"},
		{"extensão inválida", "GPG_PUBLIC_IMPORT=" + valid + "\nGPG_PRIVATE_IMPORT=" + filepath.Join(dir, "private.key") + "\n", ".asc"},
		{"arquivo ausente", "GPG_PUBLIC_IMPORT=" + valid + "\nGPG_PRIVATE_IMPORT=" + filepath.Join(dir, "private.asc") + "\n", "acessar chave privada"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPath := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(envPath, []byte(tt.env), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := gpgImportPathsFromEnv(envPath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("esperava erro contendo %q, recebeu %v", tt.want, err)
			}
		})
	}
}

func TestGPGImportStepsInstallPackageBeforeImports(t *testing.T) {
	install := step{label: "Instalar gnupg"}
	steps := gpgImportSteps("/dotfiles", gpgImportPaths{"/media/public.asc", "/media/private.asc"}, errors.New("parar"), []step{install})
	want := []string{"Validar Git e arquivos .asc", "Instalar gnupg", "Importar chave pública", "Importar chave privada", "Configurar user.signingkey"}
	if len(steps) != len(want) {
		t.Fatalf("quantidade de passos: recebeu %d, esperava %d", len(steps), len(want))
	}
	for i, label := range want {
		if steps[i].label != label {
			t.Fatalf("passo %d: recebeu %q, esperava %q", i, steps[i].label, label)
		}
	}
	for i, path := range []string{"/media/public.asc", "/media/private.asc"} {
		args := steps[i+2].cmd().Args
		got := args[len(args)-3:]
		wantArgs := []string{"gpg", "--import", path}
		for j := range wantArgs {
			if got[j] != wantArgs[j] {
				t.Fatalf("argumento %d da importação %d: recebeu %q, esperava %q", j, i, got[j], wantArgs[j])
			}
		}
	}
	if steps[len(steps)-1].run == nil {
		t.Fatal("configuração de user.signingkey deve ser um passo nativo")
	}
}

func TestParseGPGSecretFingerprintUsesPrimarySecretKey(t *testing.T) {
	primary := "0123456789ABCDEF0123456789ABCDEF01234567"
	subkey := "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
	output := strings.Join([]string{
		"sec:-:255:22:89ABCDEF01234567:0:0:::::::",
		"fpr:::::::::" + primary + ":",
		"uid:::::::::Test User:",
		"ssb:-:255:18:0123456789ABCDEF:0:0:::::::",
		"fpr:::::::::" + subkey + ":",
	}, "\n")

	got, err := parseGPGSecretFingerprint(output)
	if err != nil {
		t.Fatal(err)
	}
	if got != primary {
		t.Fatalf("fingerprint inesperado: %s", got)
	}
}

func TestParseGPGSecretFingerprintRejectsPublicKey(t *testing.T) {
	output := "pub:-:255:22:89ABCDEF01234567:0:0:::::::\nfpr:::::::::0123456789ABCDEF0123456789ABCDEF01234567:"
	if _, err := parseGPGSecretFingerprint(output); err == nil {
		t.Fatal("esperava erro para chave sem material privado")
	}
}

func TestSigningKeyFromPrivateFile(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg não está disponível")
	}
	gnupgHome := filepath.Join(t.TempDir(), "gnupg")
	if err := os.Mkdir(gnupgHome, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", gnupgHome)
	defer exec.Command("gpgconf", "--kill", "gpg-agent").Run()
	identity := "Dotfiles Test <dotfiles-test@example.invalid>"
	generate := exec.Command("gpg", "--batch", "--passphrase", "", "--quick-generate-key", identity, "ed25519", "sign", "1d")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("gerar chave temporária: %s: %v", output, err)
	}
	privateKey := filepath.Join(t.TempDir(), "private.asc")
	exported, err := exec.Command("gpg", "--batch", "--armor", "--export-secret-keys", identity).Output()
	if err != nil {
		t.Fatalf("exportar chave temporária: %v", err)
	}
	if err := os.WriteFile(privateKey, exported, 0600); err != nil {
		t.Fatal(err)
	}

	fingerprint, err := signingKeyFromPrivateFile(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !validGPGFingerprint(fingerprint) {
		t.Fatalf("fingerprint inválido: %q", fingerprint)
	}
}

func TestDevelopmentMenuPlacesGPGAfterGit(t *testing.T) {
	model := newModel("/dotfiles")
	model.screen = screenDevelopment
	items := model.items()
	for i := range items {
		if items[i].title == "Git" {
			if i+1 >= len(items) || items[i+1].title != "GPG" {
				t.Fatal("GPG deve aparecer imediatamente após Git")
			}
			return
		}
	}
	t.Fatal("Git ausente no menu Desenvolvimento")
}

func TestGitStatusReadyForGPG(t *testing.T) {
	ready := gitStatus{envOK: true, links: []gitLink{{ok: true}, {ok: true}}}
	if !ready.readyForGPG() {
		t.Fatal("esperava Git pronto")
	}
	ready.missing = []string{"git"}
	if ready.readyForGPG() {
		t.Fatal("Git com pacote ausente não pode liberar GPG")
	}
	ready.missing = nil
	ready.links[0].ok = false
	if ready.readyForGPG() {
		t.Fatal("Git com link ausente não pode liberar GPG")
	}
}
