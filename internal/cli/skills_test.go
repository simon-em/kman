package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSkillsList(t *testing.T) {
	stdout, _, code := run("skills", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "catalog:bitbucket@") {
		t.Errorf("skills ls = %q, want it to list the bitbucket catalog entry", stdout)
	}
}

func TestSkillsSetFromStdinThenList(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := dispatchWithStdin(t, Env{Stdout: &stdout, Stderr: &stderr},
		[]string{"skills", "set", "my-skill", "--kind", "doc", "--path", ".claude/skills/my-skill/SKILL.md", "--description", "a test skill"},
		"hello from a skill\n")
	if code != 0 {
		t.Fatalf("skills set exit code = %d, stderr=%s", code, stderr.String())
	}

	out, _, lsCode := run("skills", "ls")
	if lsCode != 0 {
		t.Fatalf("skills ls exit code = %d", lsCode)
	}
	if !strings.Contains(out, "catalog:my-skill@") || !strings.Contains(out, "a test skill") {
		t.Errorf("skills ls = %q", out)
	}
}

func TestSkillsSetRejectsABuiltinNameCollision(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := dispatchWithStdin(t, Env{Stdout: &stdout, Stderr: &stderr},
		[]string{"skills", "set", "bitbucket", "--kind", "doc", "--path", "x"}, "hi\n")
	if code == 0 {
		t.Fatalf("expected an error, stderr=%s", stderr.String())
	}
}

func TestSkillsRemove(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	dispatchWithStdin(t, Env{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
		[]string{"skills", "set", "my-skill", "--kind", "doc", "--path", "x"}, "hi\n")

	if _, stderr, code := run("skills", "rm", "my-skill"); code != 0 {
		t.Fatalf("skills rm exit code = %d, stderr=%s", code, stderr)
	}
	out, _, _ := run("skills", "ls")
	if strings.Contains(out, "my-skill") {
		t.Errorf("skills ls = %q, want my-skill gone", out)
	}
}

func TestSkillsRemoveRejectsAnUnknownName(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())

	_, stderr, code := run("skills", "rm", "no-such-skill")
	if code == 0 {
		t.Fatalf("expected an error, stderr=%s", stderr)
	}
}
