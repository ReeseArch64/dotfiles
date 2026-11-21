package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func prepareLaptopAppearanceTest(t *testing.T) (string, string) {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	for path, contents := range map[string]string{
		assetPath(root, "face.jpg"):                 "face-image",
		assetPath(root, "wallpapers", "laptop.jpg"): "laptop-wallpaper",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, home
}

func TestInstallLaptopAppearanceCreatesDirectoriesAndRegularFiles(t *testing.T) {
	root, home := prepareLaptopAppearanceTest(t)
	for range 2 {
		if err := installLaptopAppearance(root, home); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]string{
		filepath.Join(home, ".face"): "face-image",
		filepath.Join(home, ".config", "omarchy", "backgrounds", "dracula", "laptop.jpg"): "laptop-wallpaper",
	} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("destino não é arquivo regular: %s, %v", path, err)
		}
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("conteúdo incorreto: %s, %v", path, err)
		}
	}
	for _, name := range []string{".wallpapers", ".ssh", "mnt"} {
		if _, err := os.Lstat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("diretório não solicitado: %s", name)
		}
	}
}

func TestInstallLaptopAppearancePreservesOtherBackgroundsAndSymlinkTarget(t *testing.T) {
	root, home := prepareLaptopAppearanceTest(t)
	target := filepath.Join(home, "old-face.jpg")
	if err := os.WriteFile(target, []byte("old-face"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".face")); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(home, ".config", "omarchy", "backgrounds", "dracula")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"other.jpg", "laptop.jpg"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := installLaptopAppearance(root, home); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{target: "old-face", filepath.Join(directory, "other.jpg"): "old", filepath.Join(directory, "laptop.jpg"): "laptop-wallpaper"} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("arquivo inesperadamente alterado: %s", path)
		}
	}
	info, err := os.Lstat(filepath.Join(home, ".face"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal(".face deveria ser uma cópia regular")
	}
}

func TestInstallLaptopAppearanceValidatesSourcesBeforeReplacingFace(t *testing.T) {
	for _, missing := range []string{"face.jpg", "wallpapers/laptop.jpg"} {
		t.Run(missing, func(t *testing.T) {
			root, home := prepareLaptopAppearanceTest(t)
			if err := os.WriteFile(filepath.Join(home, ".face"), []byte("preserve"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(assetPath(root, missing)); err != nil {
				t.Fatal(err)
			}
			if err := installLaptopAppearance(root, home); err == nil {
				t.Fatal("origem ausente aceita")
			}
			contents, _ := os.ReadFile(filepath.Join(home, ".face"))
			if string(contents) != "preserve" {
				t.Fatal("foto anterior alterada sem todas as origens")
			}
		})
	}
}

func TestLaptopAppearanceStopsIfThemeInstallFails(t *testing.T) {
	root, home := prepareLaptopAppearanceTest(t)
	t.Setenv("HOME", home)
	j := laptopAppearanceJob(root)
	run := newJobRun(j)
	if next := run.handle(stepDoneMsg{err: errors.New("tema não instalado")}); next != nil || !run.failed || !run.done || run.cur != 0 {
		t.Fatal("falha do instalador não interrompeu as cópias")
	}
	for _, path := range []string{filepath.Join(home, ".face"), filepath.Join(home, ".config", "omarchy", "backgrounds", "dracula", "laptop.jpg")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("arquivo copiado após erro: %s", path)
		}
	}
}

func TestLaptopAppearanceMenuWorksWithoutSSH(t *testing.T) {
	root, home := prepareLaptopAppearanceTest(t)
	t.Setenv("HOME", home)
	m := newModelForDevice(root, deviceLaptop)
	it := m.items()[2]
	if it.title != "Aparência" || it.disabled || it.job == nil {
		t.Fatal("ação de aparência indisponível")
	}
	bin := t.TempDir()
	log := filepath.Join(home, "theme-command.log")
	t.Setenv("THEME_COMMAND_LOG", log)
	writeTestExecutable(t, bin, "omarchy-theme-install", `printf '%s\n' "$@" > "$THEME_COMMAND_LOG"`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	steps := it.job().steps
	if len(steps) != 2 || steps[0].cmd == nil || steps[1].run == nil {
		t.Fatal("tema deve ser instalado antes das cópias")
	}
	cmd := steps[0].cmd()
	if !slices.Equal(cmd.Args, []string{"omarchy-theme-install", "https://github.com/catlee/omarchy-dracula-theme"}) {
		t.Fatalf("comando de instalação inesperado: %v", cmd.Args)
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".face")); !os.IsNotExist(err) {
		t.Fatal("foto copiada antes do tema")
	}
	if _, err := steps[1].run(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(log)
	if err != nil || string(contents) != "https://github.com/catlee/omarchy-dracula-theme\n" {
		t.Fatal("instalador não recebeu a URL correta")
	}
	if _, err := os.Stat(filepath.Join(home, ".face")); err != nil {
		t.Fatal(err)
	}
}
