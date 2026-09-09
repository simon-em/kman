package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newHomeWithConfigOrigin(t *testing.T) (home, origin string) {
	t.Helper()
	origin = t.TempDir()
	mustRunGit(t, origin, "init", "-q", "--bare")

	home = t.TempDir()
	dir := filepath.Join(home, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "init", "-q")
	mustRunGit(t, dir, "remote", "add", "origin", origin)
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "add", "-A")
	mustRunGit(t, dir, "-c", "user.email=kman@kman.local", "-c", "user.name=kman", "commit", "-q", "-m", "initial")
	mustRunGit(t, dir, "push", "-u", "origin", "HEAD")
	return home, origin
}

func TestUserSetPushesToAConfiguredOrigin(t *testing.T) {
	home, origin := newHomeWithConfigOrigin(t)
	t.Setenv("KMAN_HOME", home)

	if _, stderr, code := run("user", "set", "simon"); code != 0 {
		t.Fatalf("user set exit code = %d, stderr=%s", code, stderr)
	}

	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = origin
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "user simon: saved") {
		t.Errorf("origin log = %q, want the save's commit pushed there", out)
	}
}
