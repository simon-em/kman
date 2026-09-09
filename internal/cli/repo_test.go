package cli

import (
	"strings"
	"testing"
)

func TestRepoSetAndList(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	if _, _, code := run("repo", "set", "dx", "https://bitbucket.org/smntlbt/dx.git"); code != 0 {
		t.Fatalf("repo set exit code = %d", code)
	}
	stdout, _, code := run("repo", "ls")
	if code != 0 {
		t.Fatalf("repo ls exit code = %d", code)
	}
	if !strings.Contains(stdout, "dx") || !strings.Contains(stdout, "https://bitbucket.org/smntlbt/dx.git") {
		t.Errorf("repo ls = %q, want dx and its url listed", stdout)
	}
}

func TestRepoSetRejectsAnEmptyURL(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	_, stderr, code := run("repo", "set", "dx", "")
	if code == 0 {
		t.Fatalf("expected a non-zero exit code, stderr = %q", stderr)
	}
}

func TestRepoRemove(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	run("repo", "set", "dx", "https://bitbucket.org/smntlbt/dx.git")
	if _, _, code := run("repo", "rm", "dx"); code != 0 {
		t.Fatalf("repo rm exit code = %d", code)
	}
	stdout, _, code := run("repo", "ls")
	if code != 0 || strings.Contains(stdout, "dx") {
		t.Errorf("repo ls after rm = %q, want dx gone", stdout)
	}
}

func TestRepoRemoveRejectsAnUnknownName(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	_, stderr, code := run("repo", "rm", "nobody-set-this")
	if code == 0 {
		t.Fatalf("expected a non-zero exit code, stderr = %q", stderr)
	}
}
