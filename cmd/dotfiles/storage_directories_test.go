package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var storageTestNames = []string{"Documents", "Downloads", "Pictures", "Videos", "Music"}

func TestStorageDirectoryLinksAndIdempotence(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[populated], func(t *testing.T) {
			home, root := sdkTestRoots(t)
			if populated {
				for _, name := range storageTestNames {
					sdkTestWrite(t, filepath.Join(home, name, "conflict"), "home "+name)
					sdkTestWrite(t, filepath.Join(root, name, "conflict"), "storage "+name)
					sdkTestWrite(t, filepath.Join(root, name, "unique"), "keep")
					if err := os.Chmod(filepath.Join(home, name), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := prepareStorageDirectories(home, root, sdkTestOps()); err != nil {
				t.Fatal(err)
			}
			for _, name := range storageTestNames {
				source, target := filepath.Join(home, name), filepath.Join(root, name)
				if link, err := os.Readlink(source); err != nil || link != target {
					t.Fatalf("%s should point to %s: %q %v", source, target, link, err)
				}
				info, err := os.Stat(target)
				if err != nil || !info.IsDir() {
					t.Fatalf("missing target %s: %v", target, err)
				}
				if populated {
					if info.Mode().Perm() != 0700 {
						t.Fatalf("permissions changed: %s", target)
					}
					sdkTestContent(t, filepath.Join(source, "conflict"), "home "+name)
					sdkTestContent(t, filepath.Join(target, "unique"), "keep")
					sdkTestContent(t, filepath.Join(testBackupFor(t, source), "conflict"), "home "+name)
					sdkTestContent(t, filepath.Join(testBackupFor(t, target), "conflict"), "storage "+name)
				}
			}
			ops := sdkTestOps()
			ops.copyTree = func(string, string) error { t.Fatal("idempotent migration copied data"); return nil }
			ops.rename = func(string, string) error { t.Fatal("idempotent migration renamed data"); return nil }
			ops.symlink = func(string, string) error { t.Fatal("idempotent migration replaced links"); return nil }
			if err := prepareStorageDirectories(home, root, ops); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStorageRejectsForeignLinkBeforeMigratingAnyDirectory(t *testing.T) {
	home, root := sdkTestRoots(t)
	external := t.TempDir()
	sdkTestWrite(t, filepath.Join(home, "Documents", "file"), "original")
	if err := os.Symlink(external, filepath.Join(home, "Music")); err != nil {
		t.Fatal(err)
	}
	if err := prepareStorageDirectories(home, root, sdkTestOps()); err == nil {
		t.Fatal("foreign link accepted")
	}
	sdkTestContent(t, filepath.Join(home, "Documents", "file"), "original")
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("preflight changed storage: %v %v", entries, err)
	}
}

func TestStorageRollbackPreservesOriginalsAndBackups(t *testing.T) {
	home, root := sdkTestRoots(t)
	for _, name := range storageTestNames {
		sdkTestWrite(t, filepath.Join(home, name, "file"), "home "+name)
		sdkTestWrite(t, filepath.Join(root, name, "file"), "storage "+name)
	}
	failure := errors.New("injected storage link failure")
	ops := sdkTestOps()
	ops.rename = func(from, to string) error {
		if filepath.Base(from) == "link" && to == filepath.Join(home, "Music") {
			return failure
		}
		return os.Rename(from, to)
	}
	if err := prepareStorageDirectories(home, root, ops); !errors.Is(err, failure) {
		t.Fatalf("migration failure lost: %v", err)
	}
	for _, name := range storageTestNames {
		for _, base := range []string{home, root} {
			origin := filepath.Join(base, name)
			value := "home " + name
			if base == root {
				value = "storage " + name
			}
			sdkTestContent(t, filepath.Join(origin, "file"), value)
			sdkTestContent(t, filepath.Join(testBackupFor(t, origin), "file"), value)
		}
	}
}
