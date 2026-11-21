package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func secondBackupFilesystem(t *testing.T, source string) string {
	t.Helper()
	root, err := os.MkdirTemp("/dev/shm", "dotfiles-backup-test-")
	if err != nil {
		t.Skipf("second temporary filesystem unavailable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	var a, b syscall.Stat_t
	if err := syscall.Stat(source, &a); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Stat(root, &b); err != nil {
		t.Fatal(err)
	}
	if a.Dev == b.Dev {
		t.Skip("temporary paths are on the same filesystem")
	}
	return root
}

func TestBackupGenerationsNeverOverwriteAndKeepOrigin(t *testing.T) {
	root := testBackupStore(t)
	at := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.FixedZone("other", 3600))
	origin := filepath.Join(t.TempDir(), "config")
	var previous string
	for _, value := range []string{"first", "second"} {
		sdkTestWrite(t, origin, value)
		backup, err := backupExistingPath(origin, at)
		if err != nil {
			t.Fatal(err)
		}
		if backup == previous {
			t.Fatal("backup generation reused")
		}
		sdkTestContent(t, backup, value)
		if previous != "" {
			sdkTestContent(t, previous, "first")
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(backup), "origin.json"))
		if err != nil {
			t.Fatal(err)
		}
		var metadata struct {
			Origin  string `json:"origin"`
			Created string `json:"created_utc"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Origin != origin || metadata.Created != at.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("incorrect origin metadata: %s", data)
		}
		for path := filepath.Dir(backup); path != filepath.Dir(root); path = filepath.Dir(path) {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("non-private backup container %s: %v %v", path, info, err)
			}
		}
		previous = backup
	}
	paths, err := testBackupPaths(root, "files", origin)
	if err != nil || len(paths) != 2 {
		t.Fatalf("generations: %v %v", paths, err)
	}
}

func TestBackupStoreRefusesUnsafeRootsWithoutFallback(t *testing.T) {
	for _, kind := range []string{"relative", "empty", "public", "symlink", "symlink-ancestor", "overlap"} {
		t.Run(kind, func(t *testing.T) {
			root := testBackupStore(t)
			source := filepath.Join(t.TempDir(), "original")
			sdkTestWrite(t, filepath.Join(source, "file"), "original")
			switch kind {
			case "relative":
				root = "relative"
			case "empty":
				root = ""
			case "public":
				if err := os.Chmod(root, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink", "symlink-ancestor":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				root = alias
				if kind == "symlink-ancestor" {
					root = filepath.Join(alias, "not-created")
				}
			case "overlap":
				root = filepath.Join(source, "backups")
			}
			t.Setenv("DOTFILES_BACKUP_DIR", root)
			if _, err := backupExistingPath(source, time.Now()); err == nil {
				t.Fatal("unsafe store accepted")
			}
			sdkTestContent(t, filepath.Join(source, "file"), "original")
			adjacent, _ := filepath.Glob(source + ".backup-*")
			if len(adjacent) != 0 {
				t.Fatalf("backup fell back outside store: %v", adjacent)
			}
			if kind == "symlink-ancestor" {
				if _, err := os.Stat(root); !os.IsNotExist(err) {
					t.Fatalf("wrote through symlink: %v", err)
				}
			}
		})
	}
}

func TestCrossDeviceBackupAndRollbackPreserveMetadataAndLinks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "original")
	sdkTestWrite(t, filepath.Join(source, "nested", "file"), "old")
	file := filepath.Join(source, "nested", "file")
	if err := os.Chmod(file, 0751); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(source, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(source, "dangling")); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1700000000, 123456789)
	for _, path := range []string{file, filepath.Join(source, "nested"), source} {
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	root := secondBackupFilesystem(t, source)
	t.Setenv("DOTFILES_BACKUP_DIR", root)
	backup, err := backupExistingPath(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("original not removed after complete copy: %v", err)
	}
	for _, tree := range []string{backup, source} {
		if tree == source {
			if err := restoreLocalBackup(backup, source); err != nil {
				t.Fatal(err)
			}
		}
		sdkTestContent(t, filepath.Join(tree, "nested", "file"), "old")
		a, err := os.Stat(filepath.Join(tree, "nested", "file"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.Stat(filepath.Join(tree, "hardlink"))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(a, b) || a.Mode().Perm() != 0751 || !a.ModTime().Equal(stamp) {
			t.Fatalf("metadata/hardlinks lost in %s", tree)
		}
		for _, path := range []string{tree, filepath.Join(tree, "nested")} {
			info, err := os.Stat(path)
			if err != nil || !info.ModTime().Equal(stamp) {
				t.Fatalf("directory timestamp lost: %s %v", path, err)
			}
		}
		link, err := os.Readlink(filepath.Join(tree, "dangling"))
		if err != nil || link != "missing" {
			t.Fatalf("symlink lost: %q %v", link, err)
		}
	}
	sdkTestContent(t, filepath.Join(backup, "nested", "file"), "old")
}

func TestCrossDeviceCopyFailureKeepsOriginal(t *testing.T) {
	root := testBackupStore(t)
	source := filepath.Join(t.TempDir(), "original")
	sdkTestWrite(t, filepath.Join(source, "file"), "original")
	backup, err := reserveBackupAt(root, "files", source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected incomplete copy")
	ops := sdkTestOps()
	ops.rename = func(string, string) error { return syscall.EXDEV }
	ops.copyTree = func(from, to string) error {
		if err := copySDKTree(from, to); err != nil {
			return err
		}
		return failure
	}
	if err := moveToBackup(source, backup, ops); !errors.Is(err, failure) {
		t.Fatalf("copy error lost: %v", err)
	}
	sdkTestContent(t, filepath.Join(source, "file"), "original")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatalf("partial backup exposed: %v", err)
	}
}

func TestBackupRollbackFailureKeepsCentralOriginal(t *testing.T) {
	for _, phase := range []string{"copy", "activate"} {
		t.Run(phase, func(t *testing.T) {
			testBackupStore(t)
			destination := filepath.Join(t.TempDir(), "destination")
			sdkTestWrite(t, destination, "old")
			backup, err := backupExistingPath(destination, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			sdkTestWrite(t, destination, "new")
			failure := errors.New("injected rollback " + phase)
			ops := sdkTestOps()
			if phase == "copy" {
				ops.copyTree = func(string, string) error { return failure }
			} else {
				ops.rename = func(string, string) error { return failure }
			}
			err = restoreBackup(backup, destination, ops)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), backup) {
				t.Fatalf("recovery path/error lost: %v", err)
			}
			sdkTestContent(t, backup, "old")
			if phase == "copy" {
				sdkTestContent(t, destination, "new")
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".backup-restore-*"))
			if len(stages) != 0 {
				t.Fatalf("rollback staging retained: %v", stages)
			}
		})
	}
}

func TestSDKCrossDeviceBackupFailureAndRollback(t *testing.T) {
	for _, phase := range []string{"backup-copy", "later-link", "rollback-copy", "rollback-activate"} {
		t.Run(phase, func(t *testing.T) {
			home, sdk := sdkTestRoots(t)
			store := secondBackupFilesystem(t, home)
			t.Setenv("DOTFILES_BACKUP_DIR", store)
			for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}} {
				sdkTestWrite(t, filepath.Join(home, pair[0], "file"), "home "+pair[0])
				sdkTestWrite(t, filepath.Join(sdk, pair[1], "file"), "sdk "+pair[1])
			}
			failure := errors.New("injected " + phase)
			ops := sdkTestOps()
			ops.copyTree = func(from, to string) error {
				if phase == "rollback-copy" && sdkPathWithin(from, store) {
					return failure
				}
				if err := copySDKTree(from, to); err != nil {
					return err
				}
				if phase == "backup-copy" && from == filepath.Join(home, ".local") && sdkPathWithin(to, store) {
					return failure
				}
				return nil
			}
			ops.rename = func(from, to string) error {
				if filepath.Base(from) == "link" && to == filepath.Join(home, ".local") && phase != "backup-copy" {
					return failure
				}
				if phase == "rollback-activate" && filepath.Base(from) == "original" && !sdkPathWithin(from, store) && to == filepath.Join(home, ".aur") {
					return failure
				}
				return os.Rename(from, to)
			}
			err := prepareSDKDirectories(home, sdk, ops)
			if !errors.Is(err, failure) {
				t.Fatalf("failure lost: %v", err)
			}
			for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}} {
				for _, origin := range []string{filepath.Join(home, pair[0]), filepath.Join(sdk, pair[1])} {
					value := "home " + pair[0]
					if sdkPathWithin(origin, sdk) {
						value = "sdk " + pair[1]
					}
					paths, e := testBackupPaths(store, "files", origin)
					if e != nil {
						t.Fatal(e)
					}
					if phase == "backup-copy" && origin == filepath.Join(home, ".local") {
						sdkTestContent(t, filepath.Join(origin, "file"), value)
					} else {
						if len(paths) != 1 {
							t.Fatalf("last good backup missing for %s: %v", origin, paths)
						}
						sdkTestContent(t, filepath.Join(paths[0], "file"), value)
					}
					if phase == "backup-copy" || phase == "later-link" {
						sdkTestContent(t, filepath.Join(origin, "file"), value)
					}
				}
			}
			for _, directory := range []string{home, sdk} {
				entries, _ := os.ReadDir(directory)
				for _, entry := range entries {
					if strings.Contains(entry.Name(), "backup") || strings.HasPrefix(entry.Name(), ".sdk-") {
						t.Fatalf("retained temporary/adjacent backup: %s", entry.Name())
					}
				}
			}
		})
	}
}
