package flow

import "testing"

func TestResolveArgsProvidedWins(t *testing.T) {
	s := Spec{Args: map[string]Arg{"ENVIRONMENT": {Default: "staging"}}}
	resolved, err := ResolveArgs(s, map[string]string{"ENVIRONMENT": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved["ENVIRONMENT"] != "prod" {
		t.Errorf("ENVIRONMENT = %q, want prod", resolved["ENVIRONMENT"])
	}
}

func TestResolveArgsFallsBackToDefault(t *testing.T) {
	s := Spec{Args: map[string]Arg{"ENVIRONMENT": {Default: "staging"}}}
	resolved, err := ResolveArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["ENVIRONMENT"] != "staging" {
		t.Errorf("ENVIRONMENT = %q, want staging", resolved["ENVIRONMENT"])
	}
}

func TestResolveArgsMissingRequired(t *testing.T) {
	s := Spec{Args: map[string]Arg{"REPO_BRANCH": {Required: true}}}
	if _, err := ResolveArgs(s, nil); err == nil {
		t.Fatal("expected an error for a missing required arg")
	}
}

func TestResolveArgsRequiredProvided(t *testing.T) {
	s := Spec{Args: map[string]Arg{"REPO_BRANCH": {Required: true}}}
	resolved, err := ResolveArgs(s, map[string]string{"REPO_BRANCH": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved["REPO_BRANCH"] != "main" {
		t.Errorf("REPO_BRANCH = %q, want main", resolved["REPO_BRANCH"])
	}
}

func TestResolveArgsOptionalNoDefaultOmittedWhenUnset(t *testing.T) {
	s := Spec{Args: map[string]Arg{"DRY_RUN": {}}}
	resolved, err := ResolveArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resolved["DRY_RUN"]; ok {
		t.Errorf("DRY_RUN should be omitted, got %q", resolved["DRY_RUN"])
	}
}
