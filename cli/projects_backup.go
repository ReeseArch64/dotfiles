package main

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

func findProjectRepositories(root string) ([]string, error) {
	if err := sdkRealDirectoryPath(root); err != nil {
		return nil, err
	}
	var repositories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == ".git" && (entry.IsDir() || entry.Type().IsRegular()) {
			repositories = append(repositories, filepath.Dir(path))
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mapear projetos em %s: %w", root, err)
	}
	slices.Sort(repositories)
	return repositories, nil
}

var projectBackupRemotes = [...]struct{ remote, service string }{
	{"origin", "GitHub"}, {"ogle", "GitLab"}, {"backup", "Gitea"},
}

func backupProjectRepositories(root string, commands sshCommands) (int, error) {
	repositories, err := findProjectRepositories(root)
	if err != nil {
		return 0, err
	}
	if len(repositories) == 0 {
		return 0, fmt.Errorf("nenhum projeto Git encontrado em %s", root)
	}
	var failures []error
	completed := 0
	for _, repository := range repositories {
		if err := backupProjectRepository(repository, commands); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", repository, err))
			continue
		}
		completed++
	}
	if len(failures) > 0 {
		return completed, fmt.Errorf("backup concluído em %d/%d projetos: %w", completed, len(repositories), errors.Join(failures...))
	}
	return completed, nil
}

func backupProjectRepository(repository string, commands sshCommands) error {
	output, err := commands.capture("", "git", "-C", repository, "remote")
	if err != nil {
		return fmt.Errorf("listar remotos: %w", err)
	}
	remotes := strings.Fields(output)
	var failures []error
	for _, target := range projectBackupRemotes {
		if !slices.Contains(remotes, target.remote) {
			failures = append(failures, fmt.Errorf("remoto %s não configurado", target.remote))
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	for i, target := range projectBackupRemotes {
		if err := commands.interactive("git", "-C", repository, "push", target.remote); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", target.service, err))
			if i == 0 {
				break
			}
		}
	}
	return errors.Join(failures...)
}
