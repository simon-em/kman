package gitcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Slug(remoteURL string) string {
	sum := sha256.Sum256([]byte(remoteURL))
	return hex.EncodeToString(sum[:])[:16]
}

func Dir(home, remoteURL string) string {
	return filepath.Join(home, "cache", Slug(remoteURL)+".git")
}

func Sync(ctx context.Context, home, remoteURL, authHeader string) (string, error) {
	dir := Dir(home, remoteURL)
	if _, err := os.Stat(dir); err == nil {
		if err := runGit(ctx, dir, authHeader, "fetch", "--prune", "origin"); err != nil {
			return "", err
		}
		return dir, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := runGit(ctx, "", authHeader, "clone", "--mirror", remoteURL, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func runGit(ctx context.Context, dir, authHeader string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", buildArgs(authHeader, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}

func buildArgs(authHeader string, args ...string) []string {
	if authHeader == "" {
		return args
	}
	full := []string{"-c", "http.extraHeader=Authorization: " + authHeader}
	return append(full, args...)
}
