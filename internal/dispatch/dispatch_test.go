package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFakeClaude(t *testing.T, response string) string {
	t.Helper()
	binDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "claude-args.log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf 'ARG>>>%s<<<\\n' \"$a\" >> \"$FAKE_CLAUDE_LOG\"; done\n" +
		"printf '%s' \"$FAKE_CLAUDE_RESPONSE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_LOG", logFile)
	t.Setenv("FAKE_CLAUDE_RESPONSE", response)
	return logFile
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fake claude was never invoked: %v", err)
	}
	return string(data)
}

const canned = `{"is_error":false,"result":"{}","structured_output":{"flow":"create-feature","repo":"kman-demo","task":"add a TEST.md file saying hi","clarify":""}}`

func TestRouteParsesTheStructuredOutput(t *testing.T) {
	newFakeClaude(t, canned)

	flows := []FlowCandidate{{Name: "create-feature", Description: "opens a small PR"}}
	result, err := Route(context.Background(), "add a TEST.md file to kman-demo saying hi", "", flows, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Flow != "create-feature" || result.Repo != "kman-demo" || result.Task != "add a TEST.md file saying hi" || result.Clarify != "" {
		t.Errorf("result = %+v", result)
	}
}

func TestRoutePassesCandidatesInTheSystemPrompt(t *testing.T) {
	logFile := newFakeClaude(t, canned)

	flows := []FlowCandidate{{Name: "create-feature", Description: "opens a small PR"}}
	repos := []RepoCandidate{{Name: "kman-demo"}, {Name: "dx"}}
	if _, err := Route(context.Background(), "add a file", "", flows, repos); err != nil {
		t.Fatalf("Route: %v", err)
	}

	logged := readLog(t, logFile)
	for _, want := range []string{"create-feature: opens a small PR", "kman-demo", "dx"} {
		if !strings.Contains(logged, want) {
			t.Errorf("logged args = %q, want it to contain %q", logged, want)
		}
	}
}

func TestRouteDisablesAllTools(t *testing.T) {
	logFile := newFakeClaude(t, canned)

	if _, err := Route(context.Background(), "add a file", "", []FlowCandidate{{Name: "a"}}, nil); err != nil {
		t.Fatalf("Route: %v", err)
	}

	logged := readLog(t, logFile)
	if !strings.Contains(logged, "ARG>>>--allowedTools<<<\nARG>>><<<") {
		t.Errorf("logged args = %q, want --allowedTools followed by an empty argument", logged)
	}
	if !strings.Contains(logged, "ARG>>>--strict-mcp-config<<<") {
		t.Errorf("logged args = %q, want --strict-mcp-config", logged)
	}
}

func TestRouteDefaultsToHaikuModel(t *testing.T) {
	logFile := newFakeClaude(t, canned)

	if _, err := Route(context.Background(), "add a file", "", []FlowCandidate{{Name: "a"}}, nil); err != nil {
		t.Fatalf("Route: %v", err)
	}

	logged := readLog(t, logFile)
	if !strings.Contains(logged, "ARG>>>--model<<<\nARG>>>haiku<<<") {
		t.Errorf("logged args = %q, want the default model haiku", logged)
	}
}

func TestRouteUsesAGivenModelOverTheDefault(t *testing.T) {
	logFile := newFakeClaude(t, canned)

	if _, err := Route(context.Background(), "add a file", "sonnet", []FlowCandidate{{Name: "a"}}, nil); err != nil {
		t.Fatalf("Route: %v", err)
	}

	logged := readLog(t, logFile)
	if !strings.Contains(logged, "ARG>>>--model<<<\nARG>>>sonnet<<<") {
		t.Errorf("logged args = %q, want the overridden model sonnet", logged)
	}
}

func TestRouteWithNoFlowsSkipsTheClaudeCallEntirely(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "unused.log")
	t.Setenv("FAKE_CLAUDE_LOG", logFile)

	result, err := Route(context.Background(), "anything", "", nil, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Flow != "" || result.Clarify == "" {
		t.Errorf("result = %+v, want an empty flow and a clarify message", result)
	}
	if _, err := os.Stat(logFile); err == nil {
		t.Error("claude should never have been invoked when there are no candidate flows")
	}
}

func TestRouteSurfacesAClarifyingQuestion(t *testing.T) {
	newFakeClaude(t, `{"is_error":false,"result":"{}","structured_output":{"flow":"","repo":"","task":"","clarify":"which repo did you mean?"}}`)

	result, err := Route(context.Background(), "do the thing", "", []FlowCandidate{{Name: "a"}}, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Flow != "" || result.Clarify != "which repo did you mean?" {
		t.Errorf("result = %+v", result)
	}
}

func TestRouteSurfacesAClaudeError(t *testing.T) {
	newFakeClaude(t, `{"is_error":true,"result":"budget exceeded"}`)

	_, err := Route(context.Background(), "do the thing", "", []FlowCandidate{{Name: "a"}}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "budget exceeded") {
		t.Errorf("err = %v, want it to mention the underlying reason", err)
	}
}
