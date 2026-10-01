package main

import (
	"errors"
	"os"
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
	want := []string{"Validar Git e arquivos .asc", "Instalar gnupg", "Importar chave pública", "Importar chave privada"}
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
