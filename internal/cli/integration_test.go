package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/vault"
)

func setUpBitbucketIntegration(t *testing.T, home, userID string) {
	t.Helper()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(bitbucket.ClientIDKey, "cid"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(bitbucket.ClientSecretKey, "csecret"); err != nil {
		t.Fatal(err)
	}
	provider := &bitbucket.Provider{OAuth: bitbucket.New(bitbucket.Config{ClientID: "cid", ClientSecret: "csecret"}), Vault: v}
	if err := provider.Connect(userID, bitbucket.Token{
		AccessToken:  "live-bb-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPushResolvesIntegrationCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	setUpBitbucketIntegration(t, home, "simon")

	kranq := newBareKranqRepoWithHook(t, `
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
found=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    env.BITBUCKET_TOKEN=live-bb-token) found=1 ;;
  esac
  i=$((i+1))
done
if [ "$found" = "1" ]; then
  echo "KRANQ-RESULT id=int-test status=ok exit=0"
else
  echo "KRANQ-RESULT id=int-test status=ok exit=1"
fi
`)
	source := newSourceRepo(t)

	flowPath := filepath.Join(t.TempDir(), "flow.yaml")
	flowYAML := "name: needs-bitbucket\ncredentials:\n  BITBUCKET_TOKEN: integration:bitbucket\naccess:\n  credentials: [integration:bitbucket]\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := dispatch(Env{Stdout: &stdout, Stderr: &stderr}, []string{
		"push", flowPath, "--kranq-url", kranq, "--source", source, "--as", "simon",
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestPushIntegrationCredentialRequiresActingUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	t.Setenv("KMAN_ACTOR", "")
	setUpBitbucketIntegration(t, home, "simon")

	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=x status=ok exit=0"`)
	source := newSourceRepo(t)

	flowPath := filepath.Join(t.TempDir(), "flow.yaml")
	flowYAML := "name: needs-bitbucket\ncredentials:\n  BITBUCKET_TOKEN: integration:bitbucket\naccess:\n  credentials: [integration:bitbucket]\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := run("push", flowPath, "--kranq-url", kranq, "--source", source)
	if code == 0 {
		t.Fatal("expected an error with no acting user")
	}
	if !strings.Contains(stderr, "acting user") {
		t.Errorf("stderr = %q, want it to mention the missing acting user", stderr)
	}
}

func TestPushRejectsUnknownIntegration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=x status=ok exit=0"`)
	source := newSourceRepo(t)

	flowPath := filepath.Join(t.TempDir(), "flow.yaml")
	flowYAML := "name: bad\ncredentials:\n  TOKEN: integration:not-a-real-integration\naccess:\n  credentials: [integration:not-a-real-integration]\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := run("push", flowPath, "--kranq-url", kranq, "--source", source, "--as", "simon")
	if code == 0 {
		t.Fatal("expected an error for an unknown integration")
	}
	if !strings.Contains(stderr, "unknown integration") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestIntegrationBitbucketStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	setUpBitbucketIntegration(t, home, "simon")
	run("user", "set", "alex")

	stdout, _, code := run("integration", "bitbucket", "status", "simon", "alex")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "simon: connected") {
		t.Errorf("stdout = %q, want simon connected", stdout)
	}
	if !strings.Contains(stdout, "alex: not connected") {
		t.Errorf("stdout = %q, want alex not connected", stdout)
	}
}

func TestIntegrationBitbucketStatusNotConfigured(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())
	_, stderr, code := run("integration", "bitbucket", "status", "simon")
	if code == 0 {
		t.Fatal("expected an error when bitbucket is not configured")
	}
	if stderr == "" {
		t.Error("expected an error message")
	}
}
