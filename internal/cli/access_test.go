package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/config"
)

func writeFlow(t *testing.T, home, name, content string) {
	t.Helper()
	path := filepath.Join(home, "config", "flows", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUserSetAndList(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	if _, _, code := run("user", "set", "simon", "--display-name", "Simon", "--slack-id", "U123"); code != 0 {
		t.Fatalf("user set exit code = %d", code)
	}
	stdout, _, code := run("user", "ls")
	if code != 0 {
		t.Fatalf("user ls exit code = %d", code)
	}
	if !strings.Contains(stdout, "simon") {
		t.Errorf("user ls = %q, want simon listed", stdout)
	}
}

func TestGroupSetAndMembership(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	if _, _, code := run("user", "set", "simon"); code != 0 {
		t.Fatalf("user set exit code = %d", code)
	}
	if _, _, code := run("group", "set", "oncall"); code != 0 {
		t.Fatalf("group set exit code = %d", code)
	}
	if _, _, code := run("group", "add-member", "oncall", "simon"); code != 0 {
		t.Fatalf("group add-member exit code = %d", code)
	}
	stdout, _, code := run("group", "ls")
	if code != 0 || !strings.Contains(stdout, "oncall") {
		t.Fatalf("group ls = %q, code = %d", stdout, code)
	}
}

func TestGroupAddMemberRejectsUnknownUser(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	run("group", "set", "oncall")
	_, stderr, code := run("group", "add-member", "oncall", "nobody")
	if code == 0 {
		t.Fatal("expected an error adding an unknown user to a group")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestGrantAndRevoke(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	writeFlow(t, home, "deploy-review", "name: deploy-review\nsteps:\n  - name: a\n    run: echo hi\n")
	run("user", "set", "simon")

	if _, stderr, code := run("grant", "user/simon", "deploy-review"); code != 0 {
		t.Fatalf("grant exit code = %d, stderr=%s", code, stderr)
	}

	u, err := config.LoadUser(kmanHome(), "simon")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(u.Flows, "deploy-review") {
		t.Errorf("Flows = %v, want deploy-review granted", u.Flows)
	}

	if _, stderr, code := run("revoke", "user/simon", "deploy-review"); code != 0 {
		t.Fatalf("revoke exit code = %d, stderr=%s", code, stderr)
	}
	u, err = config.LoadUser(kmanHome(), "simon")
	if err != nil {
		t.Fatal(err)
	}
	if contains(u.Flows, "deploy-review") {
		t.Errorf("Flows = %v, want deploy-review revoked", u.Flows)
	}
}

func TestGrantToGroup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	writeFlow(t, home, "run-tests", "name: run-tests\nsteps:\n  - name: a\n    run: echo hi\n")
	run("group", "set", "oncall")

	if _, stderr, code := run("grant", "group/oncall", "run-tests"); code != 0 {
		t.Fatalf("grant exit code = %d, stderr=%s", code, stderr)
	}
	g, err := config.LoadGroup(kmanHome(), "oncall")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(g.Flows, "run-tests") {
		t.Errorf("Flows = %v, want run-tests granted", g.Flows)
	}
}

func TestGrantRejectsUnknownFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	run("user", "set", "simon")

	_, stderr, code := run("grant", "user/simon", "no-such-flow")
	if code == 0 {
		t.Fatal("expected an error granting an unknown flow")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestGrantRejectsMalformedPrincipal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	writeFlow(t, home, "a", "name: a\nsteps:\n  - name: s\n    run: echo hi\n")

	_, stderr, code := run("grant", "simon", "a")
	if code == 0 {
		t.Fatal("expected an error for a principal with no user/ or group/ prefix")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func contains(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
