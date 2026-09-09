package web

import (
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

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

func TestSaveUserPushesToAConfiguredOriginNoWarning(t *testing.T) {
	home, origin := newHomeWithConfigOrigin(t)
	srv := httptest.NewServer(New(home).Handler())
	t.Cleanup(srv.Close)

	resp := mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}})
	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "push_warning") {
		t.Errorf("Location = %q, want no push_warning on a successful push", loc)
	}

	log := exec.Command("git", "log", "--oneline")
	log.Dir = origin
	out, err := log.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "user simon: saved") {
		t.Errorf("origin log = %q, want the save's commit pushed there", out)
	}
}

func TestSaveUserShowsAWarningWhenPushFails(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "init", "-q")
	mustRunGit(t, dir, "remote", "add", "origin", "/no/such/path/on/disk.git")
	srv := httptest.NewServer(New(home).Handler())
	t.Cleanup(srv.Close)

	resp := mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}})
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "push_warning") {
		t.Fatalf("Location = %q, want a push_warning for an unreachable remote", loc)
	}

	view := mustGet(t, srv, loc)
	if !bodyContains(t, view, "could not push") {
		t.Error("expected the warning banner to render on the redirected-to page")
	}
}
