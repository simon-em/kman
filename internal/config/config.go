package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/flow"
)

type Config struct {
	Flows  []flow.Spec
	Access access.Registry
}

func Dir(home string) string {
	return filepath.Join(home, "config")
}

func Load(home, repoURL string) (Config, error) {
	dir := Dir(home)
	if repoURL != "" {
		if err := syncRepo(dir, repoURL); err != nil {
			return Config{}, err
		}
	}
	return loadLocal(dir)
}

func syncRepo(dir, repoURL string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return runGit(dir, "pull", "--ff-only")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	return runGit(filepath.Dir(dir), "clone", repoURL, dir)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}

func gitCommitCmd(dir, message, authorName string) *exec.Cmd {
	cmd := exec.Command("git", "commit", "-q", "-m", message)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorName+"@kman.local",
		"GIT_COMMITTER_NAME=kman",
		"GIT_COMMITTER_EMAIL=kman@kman.local",
	)
	return cmd
}
