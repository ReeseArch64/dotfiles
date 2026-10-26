package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRestorePointValidatesBtrfsFilesystemBeforeSnapshot(t *testing.T) {
	for _, scenario := range []struct {
		name, source, target string
		allowed              bool
	}{
		{"same-btrfs", "btrfs uuid-a", "btrfs uuid-a", true},
		{"different-btrfs", "btrfs uuid-a", "btrfs uuid-b", false},
		{"non-btrfs-backups", "btrfs uuid-a", "ext4 uuid-a", false},
		{"non-btrfs-root", "ext4 uuid-a", "btrfs uuid-a", false},
		{"unknown-uuid", "btrfs", "btrfs", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := testBackupStore(t)
			bin := t.TempDir()
			log := filepath.Join(t.TempDir(), "snapshots.log")
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			t.Setenv("SOURCE_FS", scenario.source)
			t.Setenv("TARGET_FS", scenario.target)
			t.Setenv("SNAPSHOT_LOG", log)
			writeTestExecutable(t, bin, "findmnt", `if [ "$5" = / ]; then printf '%s\n' "$SOURCE_FS"; else printf '%s\n' "$TARGET_FS"; fi`)
			writeTestExecutable(t, bin, "btrfs", `case "$1 $2" in
    'subvolume show') exit 0 ;;
    'subvolume snapshot') printf '%s\n' "$*" >> "$SNAPSHOT_LOG"; mkdir -- "$4" ;;
    *) exit 80 ;;
   esac`)
			destination := restorePointPath(time.Now())
			args := restorePointSnapshotArgs(destination)
			out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
			if scenario.allowed {
				if err != nil {
					t.Fatalf("same filesystem refused: %v %s", err, out)
				}
				if info, err := os.Stat(destination); err != nil || !info.IsDir() {
					t.Fatalf("snapshot not created: %v", err)
				}
				metadata, err := os.ReadFile(filepath.Join(filepath.Dir(destination), "origin.json"))
				if err != nil || !strings.Contains(string(metadata), `"origin":"/"`) {
					t.Fatalf("snapshot origin lost: %s %v", metadata, err)
				}
				first, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err == nil {
					t.Fatalf("reused snapshot generation: %s", out)
				}
				second, _ := os.ReadFile(log)
				if string(first) != string(second) {
					t.Fatal("snapshot generation overwritten")
				}
			} else {
				if err == nil || !strings.Contains(string(out), "mesmo filesystem Btrfs") || !strings.Contains(string(out), store) {
					t.Fatalf("missing clear filesystem constraint: %v %s", err, out)
				}
				if _, err := os.Stat(log); !os.IsNotExist(err) {
					t.Fatal("snapshot command ran despite mismatch")
				}
				entries, err := os.ReadDir(store)
				if err != nil || len(entries) != 0 {
					t.Fatalf("mismatch created backup data: %v %v", entries, err)
				}
			}
		})
	}
}

func TestBackupExportsRetainEveryGeneration(t *testing.T) {
	for _, category := range []string{"browser", "ssh", "gpg"} {
		t.Run(category, func(t *testing.T) {
			store := testBackupStore(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			proc := t.TempDir()
			origin := home
			var payloadName string
			for _, value := range []string{"first", "second"} {
				switch category {
				case "browser":
					for _, root := range zenBackupRoots {
						sdkTestWrite(t, filepath.Join(home, root, "file"), value)
					}
					if err := backupBrowserAt(context.Background(), home, store, proc); err != nil {
						t.Fatal(err)
					}
					payloadName = "zen-backup.tar"
				case "ssh":
					origin = filepath.Join(home, ".ssh")
					names := requiredSSHIdentityFiles(deviceDesktop)
					for _, name := range names {
						sdkTestWrite(t, filepath.Join(origin, name), value)
					}
					if err := backupSSHKeys(origin, store, names); err != nil {
						t.Fatal(err)
					}
					payloadName = names[0]
				case "gpg":
					origin = filepath.Join(home, ".gnupg")
					if err := backupGPGKeys(context.Background(), store, func(context.Context, bool) ([]byte, error) { return []byte(value), nil }); err != nil {
						t.Fatal(err)
					}
					payloadName = "minha_chave_privada.asc"
				}
			}
			paths, err := testBackupPaths(store, category, origin)
			if err != nil || len(paths) != 2 {
				t.Fatalf("generations overwritten: %v %v", paths, err)
			}
			values := map[string]bool{}
			for _, path := range paths {
				var data []byte
				if category == "browser" {
					data, err = exec.Command("tar", "-xOf", filepath.Join(path, payloadName), ".config/zen/file").Output()
				} else {
					data, err = os.ReadFile(filepath.Join(path, payloadName))
				}
				if err != nil {
					t.Fatal(err)
				}
				values[string(data)] = true
			}
			if !values["first"] || !values["second"] {
				t.Fatalf("old/new contents not retained: %v", values)
			}
		})
	}
}

func TestZenRestorePermanentlyPreservesReplacedDataCentrally(t *testing.T) {
	testBackupStore(t)
	dotfiles, home := t.TempDir(), t.TempDir()
	writeZenTestArchive(t, filepath.Join(dotfiles, "zen-backup.tar"), validZenTestEntries())
	if err := extractZenBackup(dotfiles); err != nil {
		t.Fatal(err)
	}
	for _, root := range zenBackupRoots {
		sdkTestWrite(t, filepath.Join(home, root, "old"), "original "+root)
	}
	if err := restoreZenBackup(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	for _, root := range zenBackupRoots {
		origin := filepath.Join(home, root)
		sdkTestContent(t, filepath.Join(testBackupFor(t, origin), "old"), "original "+root)
		adjacent, _ := filepath.Glob(filepath.Join(filepath.Dir(origin), ".zen-previous-*"))
		if len(adjacent) > 0 {
			t.Fatalf("retained adjacent backup: %v", adjacent)
		}
	}
}

func TestDNSBackupStoreDoesNotChangeRootOwnershipOrMode(t *testing.T) {
	f := prepareDNS(t, "[Resolve]\nDNSStubListener=yes\n", "stub")
	before, err := os.Stat(f.backups)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := f.run(t); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	after, err := os.Stat(f.backups)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode() != after.Mode() || !os.SameFile(before, after) || before.Sys().(*syscall.Stat_t).Uid != after.Sys().(*syscall.Stat_t).Uid || before.Sys().(*syscall.Stat_t).Gid != after.Sys().(*syscall.Stat_t).Gid {
		t.Fatal("DNS script changed shared root")
	}
	for _, origin := range []string{f.config, f.resolver} {
		backup := testBackupAt(t, f.backups, "dns", origin)
		for path := filepath.Dir(backup); path != f.backups; path = filepath.Dir(path) {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("non-private root-run backup category: %s %v", path, err)
			}
		}
		if _, err := os.Lstat(origin + ".dotfiles-backup"); !os.IsNotExist(err) {
			t.Fatalf("adjacent DNS backup retained: %v", err)
		}
	}
}
