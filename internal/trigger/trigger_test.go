package trigger

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/meta"
	"github.com/simon-em/kman/internal/reporegistry"
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

func newFakeBitbucketVault(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	home := t.TempDir()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(bitbucket.ClientIDKey, "cid"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(bitbucket.ClientSecretKey, "csecret"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	if err := v.Set(bitbucket.BaseURLKey, srv.URL); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestResolveIntegrationCredentialWithAScopeNeedsNoActingUser(t *testing.T) {
	home := newFakeBitbucketVault(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"SCOPED_AT","expires_in":7200}`))
	})
	value, err := ResolveIntegrationCredential(home, "bitbucket:pullrequest", "")
	if err != nil {
		t.Fatalf("ResolveIntegrationCredential: %v", err)
	}
	if value != "SCOPED_AT" {
		t.Errorf("value = %q, want SCOPED_AT", value)
	}
}

func TestResolveIntegrationCredentialWithAScopeSendsItAsTheRequestedScope(t *testing.T) {
	var gotScope string
	home := newFakeBitbucketVault(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotScope = r.FormValue("scope")
		w.Write([]byte(`{"access_token":"AT","expires_in":7200}`))
	})
	if _, err := ResolveIntegrationCredential(home, "bitbucket:repository:write", ""); err != nil {
		t.Fatal(err)
	}
	if gotScope != "repository:write" {
		t.Errorf("scope = %q, want repository:write (colons in the scope name preserved)", gotScope)
	}
}

func TestResolveIntegrationCredentialWithNoScopeStillNeedsAnActingUser(t *testing.T) {
	home := newFakeBitbucketVault(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call bitbucket when no acting user is given and no scope is requested")
	})
	if _, err := ResolveIntegrationCredential(home, "bitbucket", ""); err == nil {
		t.Fatal("expected an error when no acting user is given and no scope is requested")
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

func newBareKranqRepoCapturingRepoOption(t *testing.T) (dir, repoFile string) {
	t.Helper()
	dir = t.TempDir()
	repoFile = filepath.Join(dir, "repo.txt")
	hook := fmt.Sprintf(`
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    repo=*) printf '%%s' "${val#repo=}" > %s ;;
  esac
  i=$((i+1))
done
echo "KRANQ-RESULT id=x status=ok exit=0"
`, repoFile)
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	if err := os.WriteFile(filepath.Join(dir, "hooks", "post-receive"), []byte("#!/bin/sh\n"+hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, repoFile
}

func TestRunSendsTheResolvedSourceAsTheRepoOptionNotTheFlowsStaticRepo(t *testing.T) {
	home := t.TempDir()
	kranq, repoFile := newBareKranqRepoCapturingRepoOption(t)
	source := newSourceRepo(t)
	sourceURL := "file://" + source
	spec := flow.Spec{
		Name:  "multi-repo-flow",
		Repo:  "https://bitbucket.org/smntlbt/kman-demo.git",
		Steps: []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{
		KranqURL: kranq, Source: sourceURL,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got, _ := os.ReadFile(repoFile); string(got) != sourceURL {
		t.Errorf("repo option = %q, want the actually-pushed source %q, not the flow's static repo:", got, sourceURL)
	}
}

func TestRunFallsBackToTheFlowsStaticRepoWhenSourceIsALocalPath(t *testing.T) {
	home := t.TempDir()
	kranq, repoFile := newBareKranqRepoCapturingRepoOption(t)
	source := newSourceRepo(t)
	spec := flow.Spec{
		Name:  "single-repo-flow",
		Repo:  "https://bitbucket.org/smntlbt/kman-demo.git",
		Steps: []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	_, err := Run(context.Background(), home, spec, nil, Options{
		KranqURL: kranq, Source: source,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got, _ := os.ReadFile(repoFile); string(got) != spec.Repo {
		t.Errorf("repo option = %q, want the flow's static repo %q", got, spec.Repo)
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
    spec=*) printf '%%s' "${val#spec=}" | python3 -c 'import sys,base64,gzip; sys.stdout.buffer.write(gzip.decompress(base64.b64decode(sys.stdin.read())))' > %s ;;
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

func TestRunComposesACustomConfigRepoSkill(t *testing.T) {
	home := t.TempDir()
	kranq, capturedFile := newBareKranqRepoCapturingTaskFile(t)
	source := newSourceRepo(t)

	stored := catalog.StoredEntry{
		Name: "my-skill",
		Kind: catalog.KindDoc,
		Path: ".claude/skills/my-skill/SKILL.md",
		Text: "# my custom skill",
	}
	if err := config.SaveSkill(home, stored, ""); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.GetSkill(home, "my-skill")
	if !ok {
		t.Fatal("expected the just-saved custom skill to be findable")
	}

	spec := flow.Spec{
		Name:   "pr-review",
		Access: flow.Access{Skills: []string{"catalog:my-skill@" + entry.Ref}},
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
	if !strings.Contains(string(rendered), "my-skill/SKILL.md") {
		t.Errorf("rendered task does not mention the custom skill's staged path:\n%s", rendered)
	}
}

func TestSourceAuthHeaderIsEmptyForANonBitbucketHost(t *testing.T) {
	if got := sourceAuthHeader(t.TempDir(), "https://github.com/example/repo.git"); got != "" {
		t.Errorf("sourceAuthHeader = %q, want empty for a non-bitbucket.org host", got)
	}
}

func TestSourceAuthHeaderIsEmptyWhenBitbucketIsNotConfigured(t *testing.T) {
	if got := sourceAuthHeader(t.TempDir(), "https://bitbucket.org/example/repo.git"); got != "" {
		t.Errorf("sourceAuthHeader = %q, want empty when bitbucket isn't configured", got)
	}
}

func TestSourceAuthHeaderMintsARepositoryScopedTokenForBitbucket(t *testing.T) {
	var gotScope string
	home := newFakeBitbucketVault(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotScope = r.FormValue("scope")
		w.Write([]byte(`{"access_token":"SOURCE_AT","expires_in":7200}`))
	})
	got := sourceAuthHeader(home, "https://bitbucket.org/smntlbt/kman-demo.git")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("x-token-auth:SOURCE_AT"))
	if got != want {
		t.Errorf("sourceAuthHeader = %q, want %q (Basic auth with x-token-auth, which is what Bitbucket's git-over-HTTPS endpoint actually requires, confirmed live: a bare Bearer header gets a 401)", got, want)
	}
	if gotScope != "repository" {
		t.Errorf("scope requested = %q, want repository (bitbucket's read scope)", gotScope)
	}
}

func TestResolveRepoRefPassesThroughAnAlreadyRemoteRef(t *testing.T) {
	got, err := ResolveRepoRef(t.TempDir(), "https://bitbucket.org/smntlbt/dx.git")
	if err != nil {
		t.Fatalf("ResolveRepoRef: %v", err)
	}
	if got != "https://bitbucket.org/smntlbt/dx.git" {
		t.Errorf("got = %q", got)
	}
}

func TestResolveRepoRefLooksUpAnAliasInTheRegistry(t *testing.T) {
	home := t.TempDir()
	if err := config.SaveRepo(home, reporegistry.Repo{Name: "dx", URL: "https://bitbucket.org/smntlbt/dx.git"}, ""); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRepoRef(home, "dx")
	if err != nil {
		t.Fatalf("ResolveRepoRef: %v", err)
	}
	if got != "https://bitbucket.org/smntlbt/dx.git" {
		t.Errorf("got = %q", got)
	}
}

func TestResolveRepoRefRejectsAnUnknownAlias(t *testing.T) {
	if _, err := ResolveRepoRef(t.TempDir(), "no-such-repo"); err == nil {
		t.Fatal("expected an error for an unregistered alias")
	}
}

func TestBitbucketEnvDerivesWorkspaceAndSlugFromTheURL(t *testing.T) {
	got := bitbucketEnv("https://bitbucket.org/smntlbt/kman-demo.git")
	if got["BITBUCKET_WORKSPACE"] != "smntlbt" || got["BITBUCKET_REPO_SLUG"] != "kman-demo" {
		t.Errorf("got = %+v", got)
	}
}

func TestBitbucketEnvIsNilForANonBitbucketHost(t *testing.T) {
	if got := bitbucketEnv("https://github.com/example/repo.git"); got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}

func TestAutoRepoEnvLeavesAFlowsOwnExplicitEnvAlone(t *testing.T) {
	spec := flow.Spec{Env: map[string]string{"BITBUCKET_WORKSPACE": "hardcoded"}}
	got := autoRepoEnv(spec, "https://bitbucket.org/smntlbt/kman-demo.git")
	if _, ok := got["BITBUCKET_WORKSPACE"]; ok {
		t.Errorf("got = %+v, want BITBUCKET_WORKSPACE left to the flow's own env:", got)
	}
	if got["BITBUCKET_REPO_SLUG"] != "kman-demo" {
		t.Errorf("got = %+v, want BITBUCKET_REPO_SLUG auto-derived", got)
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
