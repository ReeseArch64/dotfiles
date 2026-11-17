package main

import (
	"os"
	"path/filepath"
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

func TestLaptopAppearanceMenuWorksWithoutSSH(t *testing.T) {
	root, home := prepareLaptopAppearanceTest(t)
	t.Setenv("HOME", home)
	m := newModelForDevice(root, deviceLaptop)
	it := m.items()[2]
	if it.title != "Aparência" || it.disabled || it.job == nil {
		t.Fatal("ação de aparência indisponível")
	}
	for _, step := range it.job().steps {
		if step.cmd != nil || step.run == nil {
			t.Fatal("aparência deve usar somente código Go sem sudo")
		}
		if _, err := step.run(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".face")); err != nil {
		t.Fatal(err)
	}
}
