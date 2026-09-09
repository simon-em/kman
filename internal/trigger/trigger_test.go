package trigger

import (
	"context"
	"errors"
	"testing"

	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/vault"
)

func TestMergeEnvCombinesSources(t *testing.T) {
	merged, err := MergeEnv(map[string]string{"A": "1"}, map[string]string{"B": "2"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged["A"] != "1" || merged["B"] != "2" {
		t.Errorf("merged = %+v", merged)
	}
}

func TestMergeEnvRejectsACollision(t *testing.T) {
	_, err := MergeEnv(map[string]string{"A": "1"}, map[string]string{"A": "2"})
	if err == nil {
		t.Fatal("expected an error for a name set by two sources")
	}
}

func TestParseAssignmentsRejectsAMissingEquals(t *testing.T) {
	if _, err := ParseAssignments([]string{"NOTKV"}); err == nil {
		t.Fatal("expected an error for an argument with no =")
	}
}

func TestResolveIntegrationCredentialRequiresAnActingUser(t *testing.T) {
	if _, err := ResolveIntegrationCredential(t.TempDir(), "bitbucket", ""); err == nil {
		t.Fatal("expected an error when no acting user is given")
	}
}

func TestResolveIntegrationCredentialRejectsAnUnknownIntegration(t *testing.T) {
	if _, err := ResolveIntegrationCredential(t.TempDir(), "not-a-real-integration", "simon"); err == nil {
		t.Fatal("expected an error for an unknown integration")
	}
}

func TestRunFailsAtTheArgsStageForAMissingRequiredArg(t *testing.T) {
	home := t.TempDir()
	spec := flow.Spec{
		Name: "needs-arg",
		Args: map[string]flow.Arg{"X": {Required: true}},
		Steps: []flow.Step{
			{Name: "a", Run: "echo hi"},
		},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: "unused"})
	var te *Error
	if !errors.As(err, &te) || te.Stage != StageArgs {
		t.Fatalf("err = %v, want a StageArgs *Error", err)
	}
}

func TestRunFailsAtTheCredentialsStageForAMissingSecret(t *testing.T) {
	home := t.TempDir()
	if _, err := vault.Open(home); err != nil {
		t.Fatal(err)
	}
	spec := flow.Spec{
		Name:        "needs-secret",
		Credentials: map[string]string{"TOKEN": "does/not/exist"},
		Access:      flow.Access{Credentials: []string{"does/not/exist"}},
		Steps:       []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: "unused"})
	var te *Error
	if !errors.As(err, &te) || te.Stage != StageCredentials {
		t.Fatalf("err = %v, want a StageCredentials *Error", err)
	}
}

func TestLooksLikeRemote(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/repo.git": true,
		"git@example.com:org/repo.git": true,
		"file:///tmp/repo.git":         true,
		"/local/path":                  false,
		"":                             false,
	}
	for in, want := range cases {
		if got := LooksLikeRemote(in); got != want {
			t.Errorf("LooksLikeRemote(%q) = %v, want %v", in, got, want)
		}
	}
}
