package main

func prepareStorageDirectories(home, root string, ops sdkMigrationOps) error {
	return prepareHomeDirectoryLinks(home, root, ops, [][2]string{
		{"Documents", "Documents"},
		{"Downloads", "Downloads"},
		{"Pictures", "Pictures"},
		{"Videos", "Videos"},
		{"Music", "Music"},
	})
}
