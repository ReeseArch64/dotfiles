package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func trashTestRoots(t *testing.T) (string, string) {
	t.Helper()
	home, sdk := sdkTestRoots(t)
	local := filepath.Join(sdk, "Local")
	if err := os.MkdirAll(filepath.Join(local, "share"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(local, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}
	return home, filepath.Join(home, ".local", "share")
}

func TestTrashMigrationAndIdempotence(t *testing.T) {
	home, data := trashTestRoots(t)
	source, target := filepath.Join(data, "Trash"), filepath.Join(home, ".trash")
	for _, pair := range [][2]string{{source, "old"}, {target, "existing"}} {
		sdkTestWrite(t, filepath.Join(pair[0], "files", pair[1]), pair[1])
		sdkTestWrite(t, filepath.Join(pair[0], "info", pair[1]+".trashinfo"), "[Trash Info]\nPath=/original/"+pair[1]+"\n")
	}
	if err := prepareTrashDirectory(home, data, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(source); err != nil || link != target {
		t.Fatalf("link = %q, %v", link, err)
	}
	for _, name := range []string{"old", "existing"} {
		sdkTestContent(t, filepath.Join(target, "files", name), name)
		sdkTestContent(t, filepath.Join(target, "info", name+".trashinfo"), "[Trash Info]\nPath=/original/"+name+"\n")
	}
	// Both originals are preserved independently of the merged destination.
	resolved, _ := filepath.EvalSymlinks(data)
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, filepath.Join(resolved, "Trash")), "files", "old"), "old")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, target), "files", "existing"), "existing")
	for _, path := range []string{target, filepath.Join(target, "files"), filepath.Join(target, "info")} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			t.Fatalf("private directory %s: %v, %v", path, info, err)
		}
	}
	ops := sdkTestOps()
	ops.copyTree = func(string, string) error { t.Fatal("repeated migration copied data"); return nil }
	ops.rename = func(string, string) error { t.Fatal("repeated migration renamed data"); return nil }
	ops.symlink = func(string, string) error { t.Fatal("repeated migration replaced link"); return nil }
	if err := prepareTrashDirectory(home, data, ops); err != nil {
		t.Fatal(err)
	}
}

func TestTrashMissingDirectories(t *testing.T) {
	home, data := trashTestRoots(t)
	if err := prepareTrashDirectory(home, data, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"files", "info"} {
		if info, err := os.Stat(filepath.Join(data, "Trash", name)); err != nil || !info.IsDir() {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestTrashRejectsCollisions(t *testing.T) {
	for _, collision := range []string{"payload", "metadata", "orphan", "directory"} {
		t.Run(collision, func(t *testing.T) {
			home, data := trashTestRoots(t)
			source, target := filepath.Join(data, "Trash"), filepath.Join(home, ".trash")
			left, right := "files/item", "files/item"
			switch collision {
			case "metadata":
				left, right = "info/item.trashinfo", "info/item.trashinfo"
			case "orphan":
				right = "info/item.trashinfo"
			case "directory":
				left, right = "files/item/left", "files/item/right"
			}
			sdkTestWrite(t, filepath.Join(source, left), "source")
			sdkTestWrite(t, filepath.Join(target, right), "target")
			if err := prepareTrashDirectory(home, data, sdkTestOps()); err == nil {
				t.Fatal("collision accepted")
			}
			sdkTestContent(t, filepath.Join(source, left), "source")
			sdkTestContent(t, filepath.Join(target, right), "target")
		})
	}
}

func TestTrashRejectsUnsafeLinks(t *testing.T) {
	for _, location := range []string{"target", "source", "files", "info"} {
		t.Run(location, func(t *testing.T) {
			home, data := trashTestRoots(t)
			outside := t.TempDir()
			sdkTestWrite(t, filepath.Join(outside, "untouched"), "external")
			path := filepath.Join(home, ".trash")
			if location != "target" {
				path = filepath.Join(data, "Trash")
				if location != "source" {
					path = filepath.Join(path, location)
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			if err := prepareTrashDirectory(home, data, sdkTestOps()); err == nil {
				t.Fatal("unsafe link accepted")
			}
			sdkTestContent(t, filepath.Join(outside, "untouched"), "external")
		})
	}
}

func TestTrashRollback(t *testing.T) {
	for _, phase := range []string{"copy", "link", "install-target", "backup-source", "install-link"} {
		t.Run(phase, func(t *testing.T) {
			home, data := trashTestRoots(t)
			resolved, _ := filepath.EvalSymlinks(data)
			source, target := filepath.Join(resolved, "Trash"), filepath.Join(home, ".trash")
			sdkTestWrite(t, filepath.Join(source, "files", "old"), "source")
			sdkTestWrite(t, filepath.Join(target, "files", "existing"), "target")
			failure := errors.New("injected trash failure")
			ops := sdkTestOps()
			if phase == "copy" {
				ops.copyTree = func(string, string) error { return failure }
			}
			if phase == "link" {
				ops.symlink = func(string, string) error { return failure }
			}
			ops.rename = func(from, to string) error {
				if (phase == "install-target" && strings.HasPrefix(filepath.Base(from), ".sdk-stage-") && to == target) ||
					(phase == "backup-source" && from == source) ||
					(phase == "install-link" && filepath.Base(from) == "link" && to == source) {
					return failure
				}
				return os.Rename(from, to)
			}
			if err := prepareTrashDirectory(home, data, ops); !errors.Is(err, failure) {
				t.Fatalf("expected failure: %v", err)
			}
			sdkTestContent(t, filepath.Join(source, "files", "old"), "source")
			sdkTestContent(t, filepath.Join(target, "files", "existing"), "target")
			if info, err := os.Lstat(source); err != nil || !info.IsDir() {
				t.Fatalf("original trash not restored: %v", err)
			}
		})
	}
}

// Exercise the actual GIO trash operation across two filesystems, using only
// disposable HOME/XDG_DATA_HOME paths, never the running user's trash.
func TestTrashGIOWithDataOnAnotherFilesystem(t *testing.T) {
	gio, err := exec.LookPath("gio")
	if err != nil {
		t.Skip("gio unavailable")
	}
	home, _ := sdkTestRoots(t)
	data, err := os.MkdirTemp("/dev/shm", "dotfiles-trash-test-*")
	if err != nil {
		t.Skip("second filesystem unavailable")
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	hi, _ := os.Stat(home)
	di, _ := os.Stat(data)
	if hi.Sys().(*syscall.Stat_t).Dev == di.Sys().(*syscall.Stat_t).Dev {
		t.Skip("test requires different filesystems")
	}
	sdkTestWrite(t, filepath.Join(data, "Trash", "files", "previous"), "previous payload")
	sdkTestWrite(t, filepath.Join(data, "Trash", "info", "previous.trashinfo"), "[Trash Info]\nPath=/original/previous\n")
	if err := prepareTrashDirectory(home, data, sdkTestOps()); err != nil {
		t.Fatal(err)
	}
	sdkTestContent(t, filepath.Join(home, ".trash", "files", "previous"), "previous payload")
	file := filepath.Join(home, "discardable")
	sdkTestWrite(t, file, "test payload")
	cmd := exec.Command(gio, "trash", file)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+data, "GIO_USE_VFS=local")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gio trash: %v: %s", err, output)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	sdkTestContent(t, filepath.Join(home, ".trash", "files", "discardable"), "test payload")
	info, err := os.ReadFile(filepath.Join(home, ".trash", "info", "discardable.trashinfo"))
	if err != nil || !strings.Contains(string(info), "Path="+file) {
		t.Fatalf("restore metadata: %s, %v", info, err)
	}
}
