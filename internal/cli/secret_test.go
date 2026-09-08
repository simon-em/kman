package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSecretSetListRemoveRoundTrip(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	if _, _, code := run("secret", "set", "bitbucket/deploy-key", "s3cr3t"); code != 0 {
		t.Fatalf("secret set exit code = %d", code)
	}

	stdout, _, code := run("secret", "ls")
	if code != 0 {
		t.Fatalf("secret ls exit code = %d", code)
	}
	if !strings.Contains(stdout, "bitbucket/deploy-key") {
		t.Errorf("secret ls = %q, want it to list bitbucket/deploy-key", stdout)
	}
	if strings.Contains(stdout, "s3cr3t") {
		t.Errorf("secret ls printed a value: %q", stdout)
	}

	if _, _, code := run("secret", "rm", "bitbucket/deploy-key"); code != 0 {
		t.Fatalf("secret rm exit code = %d", code)
	}

	stdout, _, _ = run("secret", "ls")
	if strings.Contains(stdout, "bitbucket/deploy-key") {
		t.Errorf("secret ls after rm = %q, want it gone", stdout)
	}
}

func TestSecretRemoveMissingIsAnError(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	_, stderr, code := run("secret", "rm", "never-existed")
	if code == 0 {
		t.Fatal("expected a non-zero exit code")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestSecretSetReadsValueFromStdinWhenOmitted(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := dispatchWithStdin(t, Env{Stdout: &stdout, Stderr: &stderr}, []string{"secret", "set", "from-stdin"}, "piped-value\n")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}

	out, _, _ := run("secret", "ls")
	if !strings.Contains(out, "from-stdin") {
		t.Errorf("secret ls = %q, want from-stdin listed", out)
	}
}
