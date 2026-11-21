package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestForceRemoveAllReadOnlyTree(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignora permiss\u00f5es")
	}
	root := t.TempDir()
	mod := filepath.Join(root, "go", "pkg", "mod", "dep@v1")
	sdkTestWrite(t, filepath.Join(mod, "file.go"), "package dep")
	// Replica o cache de m\u00f3dulos do Go: arquivos 0444, diret\u00f3rios 0555.
	if err := os.Chmod(filepath.Join(mod, "file.go"), 0444); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{mod, filepath.Dir(mod), filepath.Dir(filepath.Dir(mod))} {
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "go"), 0700) })
	if err := os.RemoveAll(filepath.Join(root, "go")); err == nil {
		t.Skip("os.RemoveAll j\u00e1 removeu a \u00e1rvore read-only neste ambiente")
	}
	if err := forceRemoveAll(filepath.Join(root, "go")); err != nil {
		t.Fatalf("forceRemoveAll: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "go")); !os.IsNotExist(err) {
		t.Fatalf("\u00e1rvore ainda existe: %v", err)
	}
}

func TestValidateMigrationTreeSkipsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	sdkTestWrite(t, filepath.Join(root, "payload"), "keep")
	sock := filepath.Join(root, "zed-stable.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if err := validateMigrationTree(root); err != nil {
		t.Fatalf("socket rejected: %v", err)
	}
}

func TestRepairMigrationPermissionCommands(t *testing.T) {
	root := t.TempDir()
	var calls [][]string
	err := repairMigrationPermissions(root, 123, 456, func(args ...string) error {
		calls = append(calls, args)
		return nil
	})
	want := [][]string{
		{"sudo", "chown", "-R", "-P", "--no-dereference", "--", "123:456", root},
		{"sudo", "chmod", "-R", "u+rwX", "--", root},
	}
	if err != nil || !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, error = %v", calls, err)
	}
	failure := errors.New("sudo failed")
	calls = nil
	err = repairMigrationPermissions(root, 123, 456, func(args ...string) error {
		calls = append(calls, args)
		return failure
	})
	if !errors.Is(err, failure) || len(calls) != 1 {
		t.Fatalf("did not stop after chown failure: %v, %v", calls, err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := repairMigrationPermissions(link, 123, 456, func(...string) error {
		t.Fatal("must not repair a symlink root")
		return nil
	}); err == nil {
		t.Fatal("accepted symlink root")
	}
}

func TestMigrationRepairsUnreadableTreesBeforeMoving(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses read permissions")
	}
	for _, target := range []bool{false, true} {
		for _, directory := range []bool{false, true} {
			name := "source"
			if target {
				name = "target"
			}
			if directory {
				name += "-directory"
			} else {
				name += "-file"
			}
			t.Run(name, func(t *testing.T) {
				home, root := sdkTestRoots(t)
				tree := filepath.Join(home, ".aur")
				if target {
					tree = filepath.Join(root, "Aur")
				}
				pkg := filepath.Join(tree, "mprocs", "pkg")
				file := filepath.Join(pkg, "payload")
				sdkTestWrite(t, file, "preserved")
				external := filepath.Join(t.TempDir(), "private")
				sdkTestWrite(t, external, "external")
				if err := os.Chmod(external, 0400); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, filepath.Join(tree, "external")); err != nil {
					t.Fatal(err)
				}
				locked := file
				if directory {
					locked = pkg
				}
				if err := os.Chmod(locked, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
				ops := sdkTestOps()
				repairs := 0
				ops.repairPermissions = func(path string) error {
					repairs++
					if path != tree {
						t.Fatalf("unexpected repair: %s", path)
					}
					return repairMigrationPermissions(path, os.Getuid(), os.Getgid(), func(args ...string) error {
						// Fixtures already belong to us. Chown needs root to traverse
						// unreadable directories; exercise real chmod without sudo.
						if args[1] == "chown" {
							return nil
						}
						out, err := exec.Command(args[1], args[2:]...).CombinedOutput()
						if err != nil {
							t.Logf("%s: %s", args, out)
						}
						return err
					})
				}
				if err := prepareHomeDirectoryLinks(home, root, ops, [][2]string{{".aur", "Aur"}}); err != nil {
					t.Fatal(err)
				}
				if repairs != 1 {
					t.Fatalf("repairs = %d; want 1", repairs)
				}
				sdkTestContent(t, filepath.Join(root, "Aur", "mprocs", "pkg", "payload"), "preserved")
				sdkTestContent(t, filepath.Join(sdkTestBackup(t, tree), "mprocs", "pkg", "payload"), "preserved")
				info, err := os.Stat(external)
				if err != nil || info.Mode().Perm() != 0400 {
					t.Fatalf("external permissions changed: %v, %v", info, err)
				}
			})
		}
	}
}

func TestMigrationPermissionRepairFailureStopsBeforeMove(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses read permissions")
	}
	home, root := sdkTestRoots(t)
	pkg := filepath.Join(home, ".aur", "mprocs", "pkg")
	sdkTestWrite(t, filepath.Join(pkg, "payload"), "original")
	if err := os.Chmod(pkg, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pkg, 0700) })
	failure := errors.New("sudo denied")
	ops := sdkTestOps()
	ops.repairPermissions = func(string) error { return failure }
	ops.copyTree = func(string, string) error { t.Fatal("copied before repair"); return nil }
	ops.rename = func(string, string) error { t.Fatal("moved before repair"); return nil }
	if err := prepareHomeDirectoryLinks(home, root, ops, [][2]string{{".aur", "Aur"}}); !errors.Is(err, failure) {
		t.Fatalf("error = %v; want %v", err, failure)
	}
}
