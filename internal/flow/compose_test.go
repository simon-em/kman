package flow

import "testing"

func TestWithFileAddsAFile(t *testing.T) {
	s := Spec{Name: "a"}
	out := s.WithFile(File{Path: "/x", Content: "aGk="})
	if len(out.Files) != 1 || out.Files[0].Path != "/x" {
		t.Errorf("Files = %+v", out.Files)
	}
	if len(s.Files) != 0 {
		t.Errorf("original spec was mutated: Files = %+v", s.Files)
	}
}

func TestWithFileIsIdempotentByPath(t *testing.T) {
	s := Spec{Name: "a"}
	out := s.WithFile(File{Path: "/x", Content: "aGk="})
	out = out.WithFile(File{Path: "/x", Content: "d29ybGQ="})
	if len(out.Files) != 1 || out.Files[0].Content != "aGk=" {
		t.Errorf("Files = %+v, want the first write to win", out.Files)
	}
}

func TestWithMCPServerOnlyTouchesClaudeSteps(t *testing.T) {
	s := Spec{
		Name: "a",
		Steps: []Step{
			{Name: "run-step", Run: "echo hi"},
			{Name: "claude-step", Claude: "do it"},
		},
	}
	out := s.WithMCPServer("x", MCPServer{Command: "y"})
	if len(out.Steps[0].MCPServers) != 0 {
		t.Errorf("run step got an mcp server: %+v", out.Steps[0])
	}
	if srv, ok := out.Steps[1].MCPServers["x"]; !ok || srv.Command != "y" {
		t.Errorf("claude step MCPServers = %+v", out.Steps[1].MCPServers)
	}
	if len(s.Steps[1].MCPServers) != 0 {
		t.Errorf("original spec was mutated: %+v", s.Steps[1])
	}
}

func TestWithDisallowedToolAppendsOnce(t *testing.T) {
	s := Spec{
		Name:  "a",
		Steps: []Step{{Name: "a", Claude: "do it", DisallowedTools: []string{"Bash"}}},
	}
	out := s.WithDisallowedTool("AskUserQuestion")
	out = out.WithDisallowedTool("AskUserQuestion")
	want := []string{"Bash", "AskUserQuestion"}
	got := out.Steps[0].DisallowedTools
	if len(got) != len(want) {
		t.Fatalf("DisallowedTools = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DisallowedTools = %v, want %v", got, want)
		}
	}
}
