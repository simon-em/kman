package cli

import (
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
