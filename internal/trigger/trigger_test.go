package trigger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/meta"
	"github.com/simon-em/kman/internal/vault"
)

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func newBareKranqRepoWithHook(t *testing.T, hookBody string) string {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	hookPath := filepath.Join(dir, "hooks", "post-receive")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"+hookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newSourceRepo(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	mustRunGit(t, src, "init", "-q")
	mustRunGit(t, src, "config", "user.email", "test@example.com")
	mustRunGit(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, src, "add", "file.txt")
	mustRunGit(t, src, "commit", "-q", "-m", "initial")
	return src
}

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

func TestRunFailsAtTheMetaStageWhenNoActingUserIsGiven(t *testing.T) {
	home := t.TempDir()
	spec := flow.Spec{
		Name:   "wants-meta",
		Access: flow.Access{Meta: []string{"cron.create"}},
		Steps:  []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: "unused", MetaURL: "http://meta.example"})
	var te *Error
	if !errors.As(err, &te) || te.Stage != StageMeta {
		t.Fatalf("err = %v, want a StageMeta *Error", err)
	}
}

func TestRunWithNoMetaURLConfiguredSkipsMintingSilently(t *testing.T) {
	home := t.TempDir()
	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=x status=ok exit=0"`)
	source := newSourceRepo(t)
	spec := flow.Spec{
		Name:   "wants-meta",
		Access: flow.Access{Meta: []string{"cron.create"}},
		Steps:  []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: kranq, Source: source, AsUser: "simon"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func newBareKranqRepoCapturingMetaEnv(t *testing.T) (dir, tokenFile, urlFile string) {
	t.Helper()
	dir = t.TempDir()
	tokenFile = filepath.Join(dir, "meta_token.txt")
	urlFile = filepath.Join(dir, "meta_url.txt")
	hook := fmt.Sprintf(`
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    env.KMAN_META_TOKEN=*) printf '%%s' "${val#env.KMAN_META_TOKEN=}" > %s ;;
    env.KMAN_META_URL=*) printf '%%s' "${val#env.KMAN_META_URL=}" > %s ;;
  esac
  i=$((i+1))
done
echo "KRANQ-RESULT id=x status=ok exit=0"
`, tokenFile, urlFile)
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	if err := os.WriteFile(filepath.Join(dir, "hooks", "post-receive"), []byte("#!/bin/sh\n"+hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, tokenFile, urlFile
}

func TestRunMintsAndInjectsAMetaTokenWhenConfigured(t *testing.T) {
	home := t.TempDir()
	kranq, tokenFile, urlFile := newBareKranqRepoCapturingMetaEnv(t)
	source := newSourceRepo(t)
	spec := flow.Spec{
		Name:   "wants-meta",
		Access: flow.Access{Meta: []string{"cron.create"}},
		Steps:  []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{
		KranqURL: kranq, Source: source, AsUser: "simon", MetaURL: "http://meta.example",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("hook did not see KMAN_META_TOKEN: %v", err)
	}
	urlBytes, err := os.ReadFile(urlFile)
	if err != nil || string(urlBytes) != "http://meta.example" {
		t.Fatalf("KMAN_META_URL = %q, %v", urlBytes, err)
	}

	tok, err := meta.Open(home).Validate(string(tokenBytes), "cron.create")
	if err != nil {
		t.Fatalf("the injected token should validate against the meta store: %v", err)
	}
	if tok.FlowName != "wants-meta" || tok.UserID != "simon" {
		t.Errorf("tok = %+v", tok)
	}
}

func TestRunFailsAtTheSkillsStageForAnUnknownCatalogEntry(t *testing.T) {
	home := t.TempDir()
	spec := flow.Spec{
		Name:   "wants-a-skill",
		Access: flow.Access{Skills: []string{"catalog:no-such-skill@abc123"}},
		Steps:  []flow.Step{{Name: "a", Claude: "do it"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: "unused"})
	var te *Error
	if !errors.As(err, &te) || te.Stage != StageSkills {
		t.Fatalf("err = %v, want a StageSkills *Error", err)
	}
}

func newBareKranqRepoCapturingTaskFile(t *testing.T) (dir, capturedFile string) {
	t.Helper()
	dir = t.TempDir()
	capturedFile = filepath.Join(dir, "captured_task.yaml")
	hook := fmt.Sprintf(`
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    task_file=*) cat "${val#task_file=}" > %s ;;
  esac
  i=$((i+1))
done
echo "KRANQ-RESULT id=x status=ok exit=0"
`, capturedFile)
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	if err := os.WriteFile(filepath.Join(dir, "hooks", "post-receive"), []byte("#!/bin/sh\n"+hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, capturedFile
}

func TestRunComposesADocSkillIntoTheRenderedTask(t *testing.T) {
	home := t.TempDir()
	kranq, capturedFile := newBareKranqRepoCapturingTaskFile(t)
	source := newSourceRepo(t)

	entry, ok := catalog.Get("bitbucket")
	if !ok {
		t.Fatal("expected a bitbucket catalog entry")
	}
	spec := flow.Spec{
		Name:   "pr-review",
		Access: flow.Access{Skills: []string{"catalog:bitbucket@" + entry.Ref}},
		Steps:  []flow.Step{{Name: "a", Claude: "review the PR"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{KranqURL: kranq, Source: source})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	rendered, err := os.ReadFile(capturedFile)
	if err != nil {
		t.Fatalf("hook did not capture the task file: %v", err)
	}
	if !strings.Contains(string(rendered), entry.Path) {
		t.Errorf("rendered task does not mention the staged skill path %q:\n%s", entry.Path, rendered)
	}
	if strings.Contains(string(rendered), "mcp_servers:") {
		t.Errorf("a doc skill should not add an mcp_servers block:\n%s", rendered)
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
