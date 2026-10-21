package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func sdkTestOps() sdkMigrationOps {
	return sdkMigrationOps{copyTree: copySDKTree, rename: os.Rename, symlink: os.Symlink}
}

func sdkTestRoots(t *testing.T) (string, string) {
	t.Helper()
	testBackupStore(t)
	base := t.TempDir()
	home, root := filepath.Join(base, "home"), filepath.Join(base, "sdk")
	for _, path := range []string{home, root} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return home, root
}

func sdkTestWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0640); err != nil {
		t.Fatal(err)
	}
}

func sdkTestContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
}

func sdkTestBackup(t *testing.T, path string) string {
	t.Helper()
	return testBackupFor(t, path)
}

func TestSDKMissingSourcesAndIdempotence(t *testing.T) {
	home, root := sdkTestRoots(t)
	if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}, {".cache", "Cache"}, {"Android", "Android"}, {".gradle", "Gradle"}} {
		target := filepath.Join(root, pair[1])
		link, err := os.Readlink(filepath.Join(home, pair[0]))
		if err != nil || link != target {
			t.Fatalf("%s link = %q, %v", pair[0], link, err)
		}
		if info, err := os.Lstat(target); err != nil || !info.IsDir() {
			t.Fatalf("target directory missing: %s, %v", target, err)
		}
	}
	beforeHome, _ := os.ReadDir(home)
	beforeRoot, _ := os.ReadDir(root)
	ops := sdkTestOps()
	ops.copyTree = func(string, string) error { t.Fatal("idempotent run copied data"); return nil }
	ops.rename = func(string, string) error { t.Fatal("idempotent run renamed data"); return nil }
	ops.symlink = func(string, string) error { t.Fatal("idempotent run replaced links"); return nil }
	if err := prepareSDKDirectories(home, root, ops); err != nil {
		t.Fatal(err)
	}
	afterHome, _ := os.ReadDir(home)
	afterRoot, _ := os.ReadDir(root)
	if !reflect.DeepEqual(beforeHome, afterHome) || !reflect.DeepEqual(beforeRoot, afterRoot) {
		t.Fatal("idempotent run changed directory entries")
	}
}

func TestSDKMigrationPreservesAllSourcesMetadataAndSymlinks(t *testing.T) {
	home, root := sdkTestRoots(t)
	external := t.TempDir()
	sdkTestWrite(t, filepath.Join(external, "untouched"), "external")
	stamp := time.Unix(1700000000, 123456789)
	for _, name := range []string{".aur", ".local", ".cache", "Android", ".gradle"} {
		source := filepath.Join(home, name)
		sdkTestWrite(t, filepath.Join(source, "nested", "file"), name)
		if err := os.Chmod(filepath.Join(source, "nested", "file"), 0751); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(source, 0710); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(source, "outside")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("missing", filepath.Join(source, "dangling")); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(source, "nested", "file"), filepath.Join(source, "hardlink")); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{source, filepath.Join(source, "nested"), filepath.Join(source, "nested", "file")} {
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}, {".cache", "Cache"}, {"Android", "Android"}, {".gradle", "Gradle"}} {
		source, target := filepath.Join(home, pair[0]), filepath.Join(root, pair[1])
		backup := sdkTestBackup(t, source)
		sdkTestContent(t, filepath.Join(target, "nested", "file"), pair[0])
		sdkTestContent(t, filepath.Join(backup, "nested", "file"), pair[0])
		for _, directory := range []string{target, backup} {
			for _, rel := range []string{"", "nested", "nested/file"} {
				info, err := os.Stat(filepath.Join(directory, rel))
				if err != nil {
					t.Fatal(err)
				}
				if !info.ModTime().Equal(stamp) {
					t.Fatalf("timestamp changed at %s/%s: %s", directory, rel, info.ModTime())
				}
				want := os.FileMode(0750)
				if rel == "" {
					want = 0710
				}
				if rel == "nested/file" {
					want = 0751
				}
				if info.Mode().Perm() != want {
					t.Fatalf("mode at %s/%s = %o, want %o", directory, rel, info.Mode().Perm(), want)
				}
			}
			for _, link := range [][2]string{{"outside", external}, {"dangling", "missing"}} {
				got, err := os.Readlink(filepath.Join(directory, link[0]))
				if err != nil || got != link[1] {
					t.Fatalf("symlink changed: %q %v", got, err)
				}
			}
			one, _ := os.Stat(filepath.Join(directory, "nested", "file"))
			two, _ := os.Stat(filepath.Join(directory, "hardlink"))
			if !os.SameFile(one, two) {
				t.Fatalf("hardlink lost in %s", directory)
			}
		}
	}
	sdkTestContent(t, filepath.Join(external, "untouched"), "external")
}

func TestSDKMergeBacksUpConflictsAndNeverFollowsDestinationSymlinks(t *testing.T) {
	home, root := sdkTestRoots(t)
	source, target := filepath.Join(home, ".local"), filepath.Join(root, "Local")
	external := t.TempDir()
	sdkTestWrite(t, filepath.Join(external, "file"), "external")
	for name, value := range map[string]string{"nested/conflict": "source", "file-to-dir/child": "source child", "dir-to-file": "source file", "symlink-to-dir/file": "new internal", "replaced-link": "source link conflict"} {
		sdkTestWrite(t, filepath.Join(source, name), value)
	}
	for name, value := range map[string]string{"nested/conflict": "target conflict", "nested/only-target": "retained", "file-to-dir": "old file", "dir-to-file/child": "old directory"} {
		sdkTestWrite(t, filepath.Join(target, name), value)
	}
	for _, name := range []string{"symlink-to-dir", "replaced-link", "only-target-link"} {
		if err := os.Symlink(external, filepath.Join(target, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(external, filepath.Join(source, "dir-replaced-by-source-link")); err != nil {
		t.Fatal(err)
	}
	sdkTestWrite(t, filepath.Join(target, "dir-replaced-by-source-link", "old"), "saved")
	originalHome, _ := os.Stat(source)
	originalTarget, _ := os.Stat(target)
	if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"nested/conflict": "source", "nested/only-target": "retained", "file-to-dir/child": "source child", "dir-to-file": "source file", "symlink-to-dir/file": "new internal", "replaced-link": "source link conflict"} {
		sdkTestContent(t, filepath.Join(target, name), value)
	}
	backup := sdkTestBackup(t, target)
	for name, value := range map[string]string{"nested/conflict": "target conflict", "file-to-dir": "old file", "dir-to-file/child": "old directory", "dir-replaced-by-source-link/old": "saved"} {
		sdkTestContent(t, filepath.Join(backup, name), value)
	}
	for _, name := range []string{"only-target-link", "dir-replaced-by-source-link"} {
		got, err := os.Readlink(filepath.Join(target, name))
		if err != nil || got != external {
			t.Fatalf("symlink %s changed: %q, %v", name, got, err)
		}
	}
	homeBackup := sdkTestBackup(t, source)
	newHome, _ := os.Stat(homeBackup)
	newTarget, _ := os.Stat(backup)
	if !os.SameFile(originalHome, newHome) || !os.SameFile(originalTarget, newTarget) {
		t.Fatal("original directories were copied/deleted instead of retained by rename")
	}
	sdkTestContent(t, filepath.Join(external, "file"), "external")
	entries, _ := os.ReadDir(external)
	if len(entries) != 1 {
		t.Fatal("wrote through a destination symlink")
	}
}

func TestSDKPreflightRejectsUnsafeLastMappingBeforeChanges(t *testing.T) {
	for _, kind := range []string{"home-file", "home-link", "home-relative-link", "target-file", "target-link", "source-fifo", "target-socket"} {
		t.Run(kind, func(t *testing.T) {
			home, root := sdkTestRoots(t)
			sdkTestWrite(t, filepath.Join(home, ".aur", "original"), "home")
			sdkTestWrite(t, filepath.Join(root, "Aur", "original"), "target")
			source, target := filepath.Join(home, ".gradle"), filepath.Join(root, "Gradle")
			var err error
			switch kind {
			case "home-file":
				err = os.WriteFile(source, nil, 0600)
			case "home-link":
				err = os.Symlink(t.TempDir(), source)
			case "home-relative-link":
				err = os.Symlink("elsewhere", source)
			case "target-file":
				err = os.WriteFile(target, nil, 0600)
			case "target-link":
				err = os.Symlink(t.TempDir(), target)
			case "source-fifo":
				err = os.Mkdir(source, 0700)
				if err == nil {
					err = syscall.Mkfifo(filepath.Join(source, "fifo"), 0600)
				}
			case "target-socket":
				err = os.Mkdir(target, 0700)
				if err == nil {
					var listener net.Listener
					listener, err = net.Listen("unix", filepath.Join(target, "socket"))
					if err == nil {
						defer listener.Close()
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			beforeHome, _ := os.ReadDir(home)
			beforeRoot, _ := os.ReadDir(root)
			ops := sdkTestOps()
			ops.copyTree = func(string, string) error { t.Fatal("copied before preflight completed"); return nil }
			ops.rename = func(string, string) error { t.Fatal("renamed before preflight completed"); return nil }
			if err := prepareSDKDirectories(home, root, ops); err == nil {
				t.Fatal("unsafe mapping accepted")
			}
			afterHome, _ := os.ReadDir(home)
			afterRoot, _ := os.ReadDir(root)
			if !reflect.DeepEqual(beforeHome, afterHome) || !reflect.DeepEqual(beforeRoot, afterRoot) {
				t.Fatal("preflight modified directories")
			}
			sdkTestContent(t, filepath.Join(home, ".aur", "original"), "home")
			sdkTestContent(t, filepath.Join(root, "Aur", "original"), "target")
		})
	}
}

func TestSDKCorrectRelativeAndDanglingLinks(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "dangling", true: "existing"}[existing], func(t *testing.T) {
			home, root := sdkTestRoots(t)
			if existing {
				sdkTestWrite(t, filepath.Join(root, "Aur", "file"), "retained")
			}
			if err := os.Symlink("../sdk/Aur", filepath.Join(home, ".aur")); err != nil {
				t.Fatal(err)
			}
			if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
				t.Fatal(err)
			}
			link, err := os.Readlink(filepath.Join(home, ".aur"))
			if err != nil || link != "../sdk/Aur" {
				t.Fatalf("correct relative link replaced: %q %v", link, err)
			}
			if info, err := os.Stat(filepath.Join(home, ".aur")); err != nil || !info.IsDir() {
				t.Fatalf("target not created: %v", err)
			}
		})
	}
}

func TestSDKEmptyOriginalsRemainAsBackups(t *testing.T) {
	home, root := sdkTestRoots(t)
	for _, path := range []string{filepath.Join(home, ".aur"), filepath.Join(root, "Aur")} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(home, ".aur"), filepath.Join(root, "Aur")} {
		backup := sdkTestBackup(t, path)
		if info, err := os.Stat(backup); err != nil || !info.IsDir() {
			t.Fatalf("empty original backup removed: %v", err)
		}
	}
}

func TestSDKPreflightRejectsSymlinkAncestorsAndOverlappingRoots(t *testing.T) {
	home, root := sdkTestRoots(t)
	alias := filepath.Join(filepath.Dir(home), "alias")
	if err := os.Symlink(filepath.Dir(home), alias); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][2]string{{filepath.Join(alias, "home"), root}, {home, filepath.Join(alias, "sdk")}, {home, home}, {filepath.Dir(home), root}} {
		if err := prepareSDKDirectories(paths[0], paths[1], sdkTestOps()); err == nil {
			t.Fatalf("unsafe roots accepted: %v", paths)
		}
	}
	for _, directory := range []string{home, root} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("unsafe preflight modified %s: %v, %v", directory, entries, err)
		}
	}
}

func TestSDKRollbackRetainsCorrectDanglingLinkAndMissingSources(t *testing.T) {
	home, root := sdkTestRoots(t)
	if err := os.Symlink("../sdk/Aur", filepath.Join(home, ".aur")); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("later activation failure")
	ops := sdkTestOps()
	ops.rename = func(from, to string) error {
		if filepath.Base(from) == "link" && to == filepath.Join(home, ".gradle") {
			return failure
		}
		return os.Rename(from, to)
	}
	if err := prepareSDKDirectories(home, root, ops); !errors.Is(err, failure) {
		t.Fatalf("failure lost: %v", err)
	}
	link, err := os.Readlink(filepath.Join(home, ".aur"))
	if err != nil || link != "../sdk/Aur" {
		t.Fatalf("original dangling link lost: %q %v", link, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 1 {
		t.Fatalf("unexpected HOME changes after rollback: %v", entries)
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("unexpected targets after rollback: %v", entries)
	}
}

func TestSDKRollbackFailureRetainsOriginalAtReportedBackup(t *testing.T) {
	home, root := sdkTestRoots(t)
	sdkTestWrite(t, filepath.Join(home, ".aur", "file"), "original")
	sdkTestWrite(t, filepath.Join(root, "Aur", "file"), "old target")
	activationFailure, restoreFailure := errors.New("activation failure"), errors.New("restore failure")
	ops := sdkTestOps()
	ops.rename = func(from, to string) error {
		if to == filepath.Join(home, ".aur") {
			if filepath.Base(from) == "link" {
				return activationFailure
			}
			if filepath.Base(from) == "original" {
				return restoreFailure
			}
		}
		return os.Rename(from, to)
	}
	err := prepareSDKDirectories(home, root, ops)
	if !errors.Is(err, activationFailure) || !errors.Is(err, restoreFailure) {
		t.Fatalf("failed rollback not reported: %v", err)
	}
	backup := sdkTestBackup(t, filepath.Join(home, ".aur"))
	if !strings.Contains(err.Error(), backup) {
		t.Fatalf("recovery path absent in %v", err)
	}
	sdkTestContent(t, filepath.Join(backup, "file"), "original")
	sdkTestContent(t, filepath.Join(root, "Aur", "file"), "old target")
}

func TestSDKCrossFilesystemMigration(t *testing.T) {
	home, _ := sdkTestRoots(t)
	root, err := os.MkdirTemp("/dev/shm", "dotfiles-sdk-test-")
	if err != nil {
		t.Skipf("second temporary filesystem unavailable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	var homeStat, rootStat syscall.Stat_t
	if err := syscall.Stat(home, &homeStat); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Stat(root, &rootStat); err != nil {
		t.Fatal(err)
	}
	if homeStat.Dev == rootStat.Dev {
		t.Skip("temporary paths are on the same filesystem")
	}
	sdkTestWrite(t, filepath.Join(home, ".local", "nested", "file"), "cross filesystem")
	sdkTestWrite(t, filepath.Join(root, "Local", "target-only"), "retained")
	if err := prepareSDKDirectories(home, root, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	sdkTestContent(t, filepath.Join(root, "Local", "nested", "file"), "cross filesystem")
	sdkTestContent(t, filepath.Join(root, "Local", "target-only"), "retained")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, filepath.Join(home, ".local")), "nested", "file"), "cross filesystem")
}

func TestSDKFailuresPreserveOriginalsAndRestoreEarlierActivations(t *testing.T) {
	for _, phase := range []string{"copy", "symlink", "move-target", "install-target", "move-home", "activate-link", "later-link"} {
		t.Run(phase, func(t *testing.T) {
			home, root := sdkTestRoots(t)
			for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}} {
				sdkTestWrite(t, filepath.Join(home, pair[0], "file"), "home "+pair[0])
				sdkTestWrite(t, filepath.Join(root, pair[1], "file"), "target "+pair[1])
			}
			beforeHome, _ := os.ReadDir(home)
			beforeRoot, _ := os.ReadDir(root)
			failure := errors.New("injected failure: " + phase)
			ops := sdkTestOps()
			failed := false
			ops.copyTree = func(from, to string) error {
				if err := copySDKTree(from, to); err != nil {
					return err
				}
				if phase == "copy" && from == filepath.Join(home, ".local") {
					failed = true
					return failure
				}
				return nil
			}
			ops.symlink = func(from, to string) error {
				if phase == "symlink" {
					failed = true
					return failure
				}
				return os.Symlink(from, to)
			}
			ops.rename = func(from, to string) error {
				match := false
				switch phase {
				case "move-target":
					match = from == filepath.Join(root, "Aur")
				case "install-target":
					match = strings.HasPrefix(filepath.Base(from), ".sdk-stage-") && to == filepath.Join(root, "Aur")
				case "move-home":
					match = from == filepath.Join(home, ".aur")
				case "activate-link":
					match = filepath.Base(from) == "link" && to == filepath.Join(home, ".aur")
				case "later-link":
					match = filepath.Base(from) == "link" && to == filepath.Join(home, ".local")
				}
				if match && !failed {
					failed = true
					return failure
				}
				return os.Rename(from, to)
			}
			if err := prepareSDKDirectories(home, root, ops); !errors.Is(err, failure) {
				t.Fatalf("failure not returned: %v", err)
			}
			if !failed {
				t.Fatal("fault not reached")
			}
			for _, pair := range [][2]string{{".aur", "Aur"}, {".local", "Local"}} {
				sdkTestContent(t, filepath.Join(home, pair[0], "file"), "home "+pair[0])
				sdkTestContent(t, filepath.Join(root, pair[1], "file"), "target "+pair[1])
				if info, _ := os.Lstat(filepath.Join(home, pair[0])); !info.IsDir() {
					t.Fatal("original HOME directory not restored")
				}
			}
			afterHome, _ := os.ReadDir(home)
			afterRoot, _ := os.ReadDir(root)
			if !reflect.DeepEqual(beforeHome, afterHome) || !reflect.DeepEqual(beforeRoot, afterRoot) {
				t.Fatalf("rollback left changes: HOME %v, SDK %v", afterHome, afterRoot)
			}
		})
	}
}
