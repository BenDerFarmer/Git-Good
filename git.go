package main

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
)

func createRepo(repoUser, repoName string) error {

	base := "repos"
	dir := filepath.Join(base, repoUser, repoName)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	_, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}

	return nil
}
