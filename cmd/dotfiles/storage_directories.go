package main

import "os"

func prepareSystemStorageDirectories() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return prepareStorageDirectories(home, "/mnt/storage", sdkMigrationOps{
		copyTree: copySDKTree,
		rename:   os.Rename,
		symlink:  os.Symlink,
	})
}

func prepareStorageDirectories(home, root string, ops sdkMigrationOps) error {
	return prepareHomeDirectoryLinks(home, root, ops, [][2]string{
		{"Documents", "Documents"},
		{"Downloads", "Downloads"},
		{"Pictures", "Pictures"},
		{"Videos", "Videos"},
		{"Music", "Music"},
	})
}
