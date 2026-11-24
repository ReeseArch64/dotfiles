package main

type consoleAction func(profile, sshConsole) error

func consoleActionForArgs(args []string) consoleAction {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "--homelab":
		return func(p profile, c sshConsole) error { return runHomelabAction(p, args[1:], c) }
	case "--network":
		return func(p profile, c sshConsole) error { return runNetworkAction(p, args[1:], c) }
	}
	if len(args) == 2 {
		var run func(profile, string, sshConsole) error
		switch args[0] {
		case "--packages":
			run = runPackageAction
		case "--terminal-config":
			run = runTerminalConfig
		case "--mise-login":
			run = runMiseLogin
		}
		if run != nil {
			return func(p profile, c sshConsole) error { return run(p, args[1], c) }
		}
	}
	if len(args) != 1 {
		return nil
	}
	switch args[0] {
	case "--cursor":
		return func(p profile, c sshConsole) error { return setupCursor(p, c.output, c.commands) }
	case "--wallpapers":
		return func(p profile, c sshConsole) error { return setupWallpapers(p, c.output, c.commands) }
	case "--setup-niri":
		return func(p profile, c sshConsole) error { return setupNiri(p, c.output, c.commands) }
	case "--noctalia-requirements":
		return showNoctaliaRequirements
	case "--setup-noctalia":
		return func(p profile, c sshConsole) error { return setupNoctalia(p, c.output, c.commands) }
	case "--sdk-links":
		return runSDKMigration
	case "--storage-links":
		return runStorageMigration
	case "--setup-workspaces":
		return runWorkspacesSetup
	case "--setup-git":
		return runGitSetup
	case "--setup-mise":
		return runMiseSetup
	case "--setup-pi-agent":
		return runPiAgentSetup
	case "--remove-preinstalled":
		return runPreinstalledCleanup
	case "--setup-docker":
		return runDockerSetup
	case "--setup-distrobox":
		return runDistroboxSetup
	case "--apply-docker":
		return func(p profile, _ sshConsole) error { return runPrivilegedDocker(p) }
	case "--docker-backup":
		return func(p profile, _ sshConsole) error { return writeDockerBackup(p) }
	case "--sshd-backup":
		return func(p profile, _ sshConsole) error { return writeSSHDBackup(p) }
	}
	return nil
}
