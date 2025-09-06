package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

var allowed = regexp.MustCompile(`^[a-z0-9_-]{1,100}$`)
var consec = regexp.MustCompile(`__|--|_-|-_`)

func isValidRepoNameOrUser(name string) bool {
	if !allowed.MatchString(name) {
		return false
	}
	if consec.MatchString(name) {
		return false
	}
	if name[0] == '-' || name[0] == '_' || name[len(name)-1] == '-' || name[len(name)-1] == '_' {
		return false
	}
	return true
}

func isValidRepoPath(path string) bool {

	split := strings.Split(path, "/")

	if len(split) != 2 {
		return false
	}

	if !isValidRepoNameOrUser(split[0]) {
		return false
	}

	return isValidRepoNameOrUser(split[1])
}

func isValidCommand(command string) bool {
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
