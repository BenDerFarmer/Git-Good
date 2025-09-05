package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
)

func createRepo(repoUser, repoName string) error {

	dir := filepath.Join(reposDir, repoUser, repoName)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	_, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}

	return nil
}

func isVaildRepoName() bool {
	return true
}
func isVaildRepoUser() bool {
	return true
}

func isVaildRepoPath(path string) bool {
	return len(path) > 0
}

func isVaildCommand(command string) bool {
	return command == "git-receive-pack" || command == "git-upload-pack"
}

func sanitizeRepoArg(arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	arg = strings.Trim(arg, `'"`)
	if arg == "" {
		return "", fmt.Errorf("empty repo argument")
	}
	if filepath.IsAbs(arg) {
		return "", fmt.Errorf("absolute paths not allowed")
	}
	clean := path.Clean("/" + arg)
	clean = strings.TrimPrefix(clean, "/")
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("parent traversal not allowed")
	}
	return clean, nil
}
