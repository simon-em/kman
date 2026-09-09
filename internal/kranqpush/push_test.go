package kranqpush

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

func newBareKranqRepo(t *testing.T, hookBody string) string {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, dir, "init", "-q", "--bare")
	mustRun(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRun(t, dir, "config", "receive.advertisePushOptions", "true")
	hookPath := filepath.Join(dir, "hooks", "post-receive")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"+hookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newSourceRepoWithCommit(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	mustRun(t, src, "init", "-q")
	mustRun(t, src, "config", "user.email", "test@example.com")
	mustRun(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, src, "add", "file.txt")
	mustRun(t, src, "commit", "-q", "-m", "initial")
	return filepath.Join(src, ".git")
}

func TestPushParsesResultFromAGitHook(t *testing.T) {
	kranq := newBareKranqRepo(t, `echo "KRANQ-RESULT id=abc123 status=ok exit=0 result=refs/heads/ok/abc123"`)
	source := newSourceRepoWithCommit(t)

	result, err := Push(context.Background(), source, "HEAD", Options{
		KranqURL: kranq,
		Spec:     []byte("name: a-task\n"),
		Env:      map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if result.ID != "abc123" || result.Status != "ok" || result.ExitCode != 0 {
		t.Errorf("result = %+v", result)
	}
}

func TestPushSendsOptionsTheHookCanRead(t *testing.T) {
	kranq := newBareKranqRepo(t, `
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  echo "$val"
  i=$((i+1))
done
echo "KRANQ-RESULT id=abc123 status=ok exit=0"
`)
	source := newSourceRepoWithCommit(t)

	result, err := Push(context.Background(), source, "HEAD", Options{
		KranqURL: kranq,
		Spec:     []byte("name: a-task\n"),
		Repo:     "dx",
		Branch:   "main",
		Env:      map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !result.Found {
		t.Fatal("expected a result")
	}
}

func TestUpdateMirrorPushesTheRef(t *testing.T) {
	kranq := newBareKranqRepo(t, "")
	source := newSourceRepoWithCommit(t)

	if err := UpdateMirror(context.Background(), source, "HEAD", kranq, "abc123def456"); err != nil {
		t.Fatalf("UpdateMirror: %v", err)
	}
	refs := mustRun(t, kranq, "--git-dir="+kranq, "for-each-ref", "refs/heads/mirror")
	if !strings.Contains(refs, "refs/heads/mirror/abc123def456") {
		t.Errorf("refs = %q, want the mirror ref", refs)
	}
}

func TestPushWithNoKranqURL(t *testing.T) {
	source := newSourceRepoWithCommit(t)
	_, err := Push(context.Background(), source, "HEAD", Options{Spec: []byte("name: a-task\n")})
	if err == nil {
		t.Fatal("expected an error when no kranq URL is given")
	}
}
