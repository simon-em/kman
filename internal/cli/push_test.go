package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/simon-em/kman/internal/vault"
)

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func newBareKranqRepoWithHook(t *testing.T, hookBody string) string {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	hookPath := filepath.Join(dir, "hooks", "post-receive")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"+hookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newSourceRepo(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	mustRunGit(t, src, "init", "-q")
	mustRunGit(t, src, "config", "user.email", "test@example.com")
	mustRunGit(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, src, "add", "file.txt")
	mustRunGit(t, src, "commit", "-q", "-m", "initial")
	return src
}

func TestPushForwardsAResolvedCredentialAsAPushOption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("bitbucket/deploy-key", "s3cr3t-token"); err != nil {
		t.Fatal(err)
	}

	kranq := newBareKranqRepoWithHook(t, `
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
found=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    env.BITBUCKET_TOKEN=s3cr3t-token) found=1 ;;
  esac
  i=$((i+1))
done
if [ "$found" = "1" ]; then
  echo "KRANQ-RESULT id=cred-test status=ok exit=0"
else
  echo "KRANQ-RESULT id=cred-test status=ok exit=1"
fi
`)
	source := newSourceRepo(t)

	flowPath := filepath.Join(t.TempDir(), "flow.yaml")
	flowYAML := "name: needs-a-credential\ncredentials:\n  BITBUCKET_TOKEN: bitbucket/deploy-key\naccess:\n  credentials: [bitbucket/deploy-key]\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := dispatch(Env{Stdout: &stdout, Stderr: &stderr}, []string{
		"push", flowPath, "--kranq-url", kranq, "--source", source,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestPushFailsWhenACredentialIsMissingFromTheVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=x status=ok exit=0"`)
	source := newSourceRepo(t)

	flowPath := filepath.Join(t.TempDir(), "flow.yaml")
	flowYAML := "name: needs-a-credential\ncredentials:\n  BITBUCKET_TOKEN: bitbucket/does-not-exist\naccess:\n  credentials: [bitbucket/does-not-exist]\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := dispatch(Env{Stdout: &stdout, Stderr: &stderr}, []string{
		"push", flowPath, "--kranq-url", kranq, "--source", source,
	})
	if code == 0 {
		t.Fatal("expected a non-zero exit code for a missing credential")
	}
	if stderr.String() == "" {
		t.Error("expected an error message on stderr")
	}
}
