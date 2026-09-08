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

func Load(home, repoURL string) (Config, error) {
	dir := filepath.Join(home, "config")
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
