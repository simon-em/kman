package gitcache

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func newOriginWithCommit(t *testing.T, message string) string {
	t.Helper()
	origin := t.TempDir()
	mustRun(t, origin, "init", "-q")
	mustRun(t, origin, "config", "user.email", "test@example.com")
	mustRun(t, origin, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(origin, "file.txt"), []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, origin, "add", "file.txt")
	mustRun(t, origin, "commit", "-q", "-m", message)
	return origin
}

func TestSyncClonesOnFirstCall(t *testing.T) {
	origin := newOriginWithCommit(t, "first")
	home := t.TempDir()

	dir, err := Sync(context.Background(), home, origin)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	log := mustRun(t, dir, "--git-dir="+dir, "log", "--oneline", "-1")
	if !strings.Contains(log, "first") {
		t.Errorf("mirror log = %q, want it to mention the first commit", log)
	}
}

func TestSyncFetchesOnSecondCall(t *testing.T) {
	origin := newOriginWithCommit(t, "first")
	home := t.TempDir()

	if _, err := Sync(context.Background(), home, origin); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	if err := os.WriteFile(filepath.Join(origin, "file2.txt"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, origin, "add", "file2.txt")
	mustRun(t, origin, "commit", "-q", "-m", "second")

	dir, err := Sync(context.Background(), home, origin)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	log := mustRun(t, dir, "--git-dir="+dir, "log", "--oneline", "-2")
	if !strings.Contains(log, "second") {
		t.Errorf("mirror log = %q, want it to mention the second commit after re-sync", log)
	}
}

func TestDirIsStableForTheSameRemote(t *testing.T) {
	home := t.TempDir()
	a := Dir(home, "https://example.com/repo.git")
	b := Dir(home, "https://example.com/repo.git")
	if a != b {
		t.Errorf("Dir is not stable: %q != %q", a, b)
	}
	other := Dir(home, "https://example.com/other.git")
	if a == other {
		t.Error("Dir collided for two different remotes")
	}
}
