package slack

import (
	"testing"

	"github.com/simon-em/kman/internal/flow"
)

func TestInjectAskRelayAddsFileAndMCPServer(t *testing.T) {
	spec := flow.Spec{
		Name: "ask-flow",
		Steps: []flow.Step{
			{Name: "a", Claude: "do something"},
		},
	}
	out := InjectAskRelay(spec, "kman-run-1", "C1", "T1")

	if len(out.Files) != 1 || out.Files[0].Path != askRelayPath {
		t.Fatalf("Files = %+v", out.Files)
	}
	srv, ok := out.Steps[0].MCPServers["kman-ask"]
	if !ok {
		t.Fatal("expected a kman-ask mcp server on the claude step")
	}
	if srv.Command != "python3" || len(srv.Args) != 1 || srv.Args[0] != askRelayPath {
		t.Errorf("mcp server = %+v", srv)
	}
	if srv.Env["KMAN_RUN_ID"] != "kman-run-1" || srv.Env["KMAN_FLOW_ID"] != "ask-flow" ||
		srv.Env["KMAN_SLACK_CHANNEL"] != "C1" || srv.Env["KMAN_SLACK_THREAD_TS"] != "T1" {
		t.Errorf("mcp server env = %+v", srv.Env)
	}
}

func TestInjectAskRelayDisablesBuiltinAskUserQuestion(t *testing.T) {
	spec := flow.Spec{
		Name:  "ask-flow",
		Steps: []flow.Step{{Name: "a", Claude: "do something"}},
	}
	out := InjectAskRelay(spec, "id", "C1", "")
	if !containsString(out.Steps[0].DisallowedTools, "AskUserQuestion") {
		t.Errorf("DisallowedTools = %v, want it to include AskUserQuestion", out.Steps[0].DisallowedTools)
	}
}

func TestInjectAskRelayOnlyTouchesClaudeSteps(t *testing.T) {
	spec := flow.Spec{
		Name: "mixed",
		Steps: []flow.Step{
			{Name: "a", Run: "echo hi"},
			{Name: "b", Claude: "do something"},
		},
	}
	out := InjectAskRelay(spec, "id", "C1", "")
	if len(out.Steps[0].MCPServers) != 0 {
		t.Errorf("run step should not get an mcp server: %+v", out.Steps[0])
	}
	if len(out.Steps[1].MCPServers) != 1 {
		t.Errorf("claude step should get an mcp server: %+v", out.Steps[1])
	}
}

func TestInjectAskRelayDoesNotMutateTheOriginalSpec(t *testing.T) {
	spec := flow.Spec{
		Name:  "ask-flow",
		Steps: []flow.Step{{Name: "a", Claude: "do something"}},
	}
	InjectAskRelay(spec, "id", "C1", "")
	if len(spec.Files) != 0 {
		t.Errorf("original spec was mutated: Files = %+v", spec.Files)
	}
	if len(spec.Steps[0].MCPServers) != 0 {
		t.Errorf("original spec's step was mutated: %+v", spec.Steps[0])
	}
}
