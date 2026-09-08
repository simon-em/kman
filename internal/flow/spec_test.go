package flow

import (
	"os"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	data, err := os.ReadFile("testdata/flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "deploy-review" {
		t.Errorf("Name = %q, want deploy-review", s.Name)
	}
	if len(s.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(s.Steps))
	}
	if arg, ok := s.Args["REPO_BRANCH"]; !ok || !arg.Required {
		t.Errorf("REPO_BRANCH arg = %+v, want required", arg)
	}
}

func TestParseRejectsNoName(t *testing.T) {
	_, err := Parse([]byte("steps:\n  - name: a\n    run: echo hi\n"))
	if err == nil {
		t.Fatal("expected an error for a missing name")
	}
}

func TestParseRejectsNoSteps(t *testing.T) {
	_, err := Parse([]byte("name: empty\n"))
	if err == nil {
		t.Fatal("expected an error for a flow with no steps")
	}
}

func TestParseRejectsRunAndClaude(t *testing.T) {
	data := []byte("name: bad\nsteps:\n  - name: a\n    run: echo hi\n    claude: do it\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected an error for a step with both run and claude")
	}
}

func TestParseRejectsBadPermissionMode(t *testing.T) {
	data := []byte("name: bad\nsteps:\n  - name: a\n    claude: go\n    permission_mode: yolo\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected an error for an invalid permission_mode")
	}
}

func TestParseRejectsMCPOnRunStep(t *testing.T) {
	data := []byte("name: bad\nsteps:\n  - name: a\n    run: echo hi\n    mcp_servers:\n      x:\n        command: y\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected an error for mcp_servers on a run step")
	}
}

func TestParseRejectsRequiredWithDefault(t *testing.T) {
	data := []byte("name: bad\nargs:\n  X:\n    required: true\n    default: y\nsteps:\n  - name: a\n    run: echo hi\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected an error for an arg that is both required and defaulted")
	}
}

func TestRenderProjectsToKranqShape(t *testing.T) {
	data, err := os.ReadFile("testdata/flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"name: deploy-review", "run: go test ./...", "permission_mode: bypassPermissions"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "args:") {
		t.Errorf("rendered output should not contain kman's args: block, kranq doesn't know it:\n%s", out)
	}
}
