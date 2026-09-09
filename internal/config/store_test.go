package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/cron"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/reporegistry"
)

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	return string(out)
}

func initGitConfigRepo(t *testing.T, home string) {
	t.Helper()
	dir := Dir(home)
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func initGitConfigRepoWithOrigin(t *testing.T) (home, originDir string) {
	t.Helper()
	originDir = t.TempDir()
	mustRunGit(t, originDir, "init", "-q", "--bare")

	home = t.TempDir()
	dir := Dir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "init", "-q")
	mustRunGit(t, dir, "remote", "add", "origin", originDir)
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "add", "-A")
	cmd := gitCommitCmd(dir, "initial", "kman")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	mustRunGit(t, dir, "push", "-u", "origin", "HEAD")
	return home, originDir
}

func TestSaveUserRoundTrip(t *testing.T) {
	home := t.TempDir()
	u := access.User{ID: "simon", DisplayName: "Simon"}
	if err := SaveUser(home, u, ""); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	got, err := LoadUser(home, "simon")
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if got.DisplayName != "Simon" {
		t.Errorf("DisplayName = %q, want Simon", got.DisplayName)
	}
}

func TestSaveGroupRoundTrip(t *testing.T) {
	home := t.TempDir()
	g := access.Group{Name: "oncall", Members: []string{"simon"}}
	if err := SaveGroup(home, g, ""); err != nil {
		t.Fatalf("SaveGroup: %v", err)
	}
	got, err := LoadGroup(home, "oncall")
	if err != nil {
		t.Fatalf("LoadGroup: %v", err)
	}
	if len(got.Members) != 1 || got.Members[0] != "simon" {
		t.Errorf("Members = %v", got.Members)
	}
}

func TestSaveFlowRoundTrip(t *testing.T) {
	home := t.TempDir()
	spec := flow.Spec{Name: "deploy", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}
	if err := SaveFlow(home, spec, ""); err != nil {
		t.Fatalf("SaveFlow: %v", err)
	}
	got, err := LoadFlow(home, "deploy")
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if got.Name != "deploy" {
		t.Errorf("Name = %q, want deploy", got.Name)
	}
}

func TestSaveCronEntryRoundTrip(t *testing.T) {
	home := t.TempDir()
	if err := SaveFlow(home, flow.Spec{Name: "report", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}, ""); err != nil {
		t.Fatal(err)
	}
	e := cron.Entry{Name: "nightly", Flow: "report", Schedule: "0 6 * * *", CreatedBy: "simon"}
	if err := SaveCronEntry(home, e, ""); err != nil {
		t.Fatalf("SaveCronEntry: %v", err)
	}
	got, err := LoadCronEntry(home, "nightly")
	if err != nil {
		t.Fatalf("LoadCronEntry: %v", err)
	}
	if got.Flow != "report" || got.Schedule != "0 6 * * *" {
		t.Errorf("got = %+v", got)
	}
}

func TestRemoveCronEntry(t *testing.T) {
	home := t.TempDir()
	e := cron.Entry{Name: "nightly", Flow: "report", Schedule: "0 6 * * *"}
	if err := SaveCronEntry(home, e, ""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCronEntry(home, "nightly", ""); err != nil {
		t.Fatalf("RemoveCronEntry: %v", err)
	}
	if _, err := LoadCronEntry(home, "nightly"); !os.IsNotExist(err) {
		t.Errorf("LoadCronEntry after remove: %v, want IsNotExist", err)
	}
}

func TestSaveRepoRoundTrip(t *testing.T) {
	home := t.TempDir()
	r := reporegistry.Repo{Name: "dx", URL: "https://bitbucket.org/smntlbt/dx.git"}
	if err := SaveRepo(home, r, ""); err != nil {
		t.Fatalf("SaveRepo: %v", err)
	}
	got, err := LoadRepo(home, "dx")
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if got.URL != "https://bitbucket.org/smntlbt/dx.git" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestRemoveRepo(t *testing.T) {
	home := t.TempDir()
	r := reporegistry.Repo{Name: "dx", URL: "https://bitbucket.org/smntlbt/dx.git"}
	if err := SaveRepo(home, r, ""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRepo(home, "dx", ""); err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}
	if _, err := LoadRepo(home, "dx"); !os.IsNotExist(err) {
		t.Errorf("LoadRepo after remove: %v, want IsNotExist", err)
	}
}

func TestListRepos(t *testing.T) {
	home := t.TempDir()
	if err := SaveRepo(home, reporegistry.Repo{Name: "dx", URL: "https://bitbucket.org/smntlbt/dx.git"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveRepo(home, reporegistry.Repo{Name: "kman-demo", URL: "https://bitbucket.org/smntlbt/kman-demo.git"}, ""); err != nil {
		t.Fatal(err)
	}
	repos, err := ListRepos(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].Name != "dx" || repos[1].Name != "kman-demo" {
		t.Errorf("repos = %+v, want sorted [dx kman-demo]", repos)
	}
}

func TestListNames(t *testing.T) {
	home := t.TempDir()
	if err := SaveFlow(home, flow.Spec{Name: "b", Steps: []flow.Step{{Name: "s", Run: "x"}}}, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveFlow(home, flow.Spec{Name: "a", Steps: []flow.Step{{Name: "s", Run: "x"}}}, ""); err != nil {
		t.Fatal(err)
	}
	names, err := ListFlowNames(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("names = %v, want sorted [a b]", names)
	}
}

func TestSaveDoesNotCommitWithoutAGitRepo(t *testing.T) {
	home := t.TempDir()
	if err := SaveUser(home, access.User{ID: "simon"}, ""); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	got, err := LoadUser(home, "simon")
	if err != nil || got.ID != "simon" {
		t.Fatalf("LoadUser: %v, %+v", err, got)
	}
}

func TestSaveCommitsWhenConfigIsAGitRepo(t *testing.T) {
	home := t.TempDir()
	initGitConfigRepo(t, home)

	if err := SaveUser(home, access.User{ID: "simon"}, "alex"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	log := gitLog(t, Dir(home))
	if !strings.Contains(log, "user simon: saved") {
		t.Errorf("git log = %q, want a commit for the save", log)
	}
}

func TestSaveCommitAttributesTheAuthor(t *testing.T) {
	home := t.TempDir()
	initGitConfigRepo(t, home)

	if err := SaveUser(home, access.User{ID: "simon"}, "alex"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	cmd := exec.Command("git", "log", "-1", "--format=%an <%ae>")
	cmd.Dir = Dir(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "alex") {
		t.Errorf("author = %q, want it to mention alex", out)
	}
}

func TestSaveTwiceProducesTwoCommits(t *testing.T) {
	home := t.TempDir()
	initGitConfigRepo(t, home)

	if err := SaveUser(home, access.User{ID: "simon", DisplayName: "Simon"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveUser(home, access.User{ID: "simon", DisplayName: "Simon Talbot"}, ""); err != nil {
		t.Fatal(err)
	}
	log := gitLog(t, Dir(home))
	if len(strings.Split(strings.TrimSpace(log), "\n")) != 2 {
		t.Errorf("git log = %q, want exactly two commits", log)
	}
}

func TestSaveWithNoChangeIsANoOpCommit(t *testing.T) {
	home := t.TempDir()
	initGitConfigRepo(t, home)

	u := access.User{ID: "simon", DisplayName: "Simon"}
	if err := SaveUser(home, u, ""); err != nil {
		t.Fatal(err)
	}
	if err := SaveUser(home, u, ""); err != nil {
		t.Fatalf("second identical SaveUser should not error: %v", err)
	}
	log := gitLog(t, Dir(home))
	if len(strings.Split(strings.TrimSpace(log), "\n")) != 1 {
		t.Errorf("git log = %q, want exactly one commit (the second save changed nothing)", log)
	}
}

func TestPushIfConfiguredIsANoOpWithNoGitRepo(t *testing.T) {
	home := t.TempDir()
	pushed, err := PushIfConfigured(home)
	if err != nil || pushed {
		t.Errorf("pushed=%v err=%v, want false, nil", pushed, err)
	}
}

func TestPushIfConfiguredIsANoOpWithNoRemote(t *testing.T) {
	home := t.TempDir()
	initGitConfigRepo(t, home)
	if err := SaveUser(home, access.User{ID: "simon"}, ""); err != nil {
		t.Fatal(err)
	}
	pushed, err := PushIfConfigured(home)
	if err != nil || pushed {
		t.Errorf("pushed=%v err=%v, want false, nil", pushed, err)
	}
}

func TestPushIfConfiguredPushesWhenARemoteExists(t *testing.T) {
	home, origin := initGitConfigRepoWithOrigin(t)
	if err := SaveUser(home, access.User{ID: "simon"}, ""); err != nil {
		t.Fatal(err)
	}

	pushed, err := PushIfConfigured(home)
	if err != nil {
		t.Fatalf("PushIfConfigured: %v", err)
	}
	if !pushed {
		t.Fatal("want pushed = true")
	}
	log := gitLog(t, origin)
	if !strings.Contains(log, "user simon: saved") {
		t.Errorf("origin log = %q, want the save's commit", log)
	}
}

func TestPushIfConfiguredPullsRemoteChangesFirst(t *testing.T) {
	home, origin := initGitConfigRepoWithOrigin(t)

	other := t.TempDir()
	mustRunGit(t, filepath.Dir(other), "clone", "-q", origin, other)
	if err := os.WriteFile(filepath.Join(other, "from-other.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, other, "add", "-A")
	otherCommit := gitCommitCmd(other, "someone else's change", "kman")
	if out, err := otherCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	mustRunGit(t, other, "push")

	if err := SaveUser(home, access.User{ID: "simon"}, ""); err != nil {
		t.Fatal(err)
	}
	pushed, err := PushIfConfigured(home)
	if err != nil {
		t.Fatalf("PushIfConfigured: %v", err)
	}
	if !pushed {
		t.Fatal("want pushed = true")
	}

	log := gitLog(t, origin)
	if !strings.Contains(log, "someone else's change") || !strings.Contains(log, "user simon: saved") {
		t.Errorf("origin log = %q, want both commits present", log)
	}
	if _, err := os.Stat(filepath.Join(Dir(home), "from-other.txt")); err != nil {
		t.Errorf("local config dir should have pulled the other change too: %v", err)
	}
}

func TestPushIfConfiguredSurfacesAnErrorWhenTheRemoteIsUnreachable(t *testing.T) {
	home := t.TempDir()
	dir := Dir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, dir, "init", "-q")
	mustRunGit(t, dir, "remote", "add", "origin", "/no/such/path/on/disk.git")
	if err := SaveUser(home, access.User{ID: "simon"}, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := PushIfConfigured(home); err == nil {
		t.Fatal("expected an error for an unreachable remote")
	}
}

func TestSaveSkillRoundTrip(t *testing.T) {
	home := t.TempDir()
	e := catalog.StoredEntry{Name: "my-skill", Kind: catalog.KindDoc, Path: ".claude/skills/my-skill/SKILL.md", Text: "hello"}
	if err := SaveSkill(home, e, ""); err != nil {
		t.Fatalf("SaveSkill: %v", err)
	}
	got, err := LoadSkill(home, "my-skill")
	if err != nil {
		t.Fatalf("LoadSkill: %v", err)
	}
	if got.Text != "hello" {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestRemoveSkill(t *testing.T) {
	home := t.TempDir()
	e := catalog.StoredEntry{Name: "my-skill", Kind: catalog.KindDoc, Path: "x", Text: "hi"}
	if err := SaveSkill(home, e, ""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSkill(home, "my-skill", ""); err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if _, err := LoadSkill(home, "my-skill"); !os.IsNotExist(err) {
		t.Errorf("LoadSkill after remove: %v, want IsNotExist", err)
	}
}

func TestListSkillsIncludesBuiltinsAndCustom(t *testing.T) {
	home := t.TempDir()
	e := catalog.StoredEntry{Name: "my-skill", Kind: catalog.KindDoc, Path: "x", Text: "hi"}
	if err := SaveSkill(home, e, ""); err != nil {
		t.Fatal(err)
	}
	entries, err := ListSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["bitbucket"] {
		t.Error("expected the built-in bitbucket entry to be included")
	}
	if !names["my-skill"] {
		t.Error("expected the custom my-skill entry to be included")
	}
}

func TestGetSkillFindsBuiltinAndCustom(t *testing.T) {
	home := t.TempDir()
	e := catalog.StoredEntry{Name: "my-skill", Kind: catalog.KindDoc, Path: "x", Text: "hi"}
	if err := SaveSkill(home, e, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetSkill(home, "bitbucket"); !ok {
		t.Error("expected GetSkill to find the built-in bitbucket entry")
	}
	if _, ok := GetSkill(home, "my-skill"); !ok {
		t.Error("expected GetSkill to find the custom my-skill entry")
	}
	if _, ok := GetSkill(home, "no-such-skill"); ok {
		t.Error("expected GetSkill to report false for an unknown entry")
	}
}

func TestSkillLookupMatchesGetSkill(t *testing.T) {
	home := t.TempDir()
	lookup := SkillLookup(home)
	e, ok := lookup("bitbucket")
	if !ok || e.Name != "bitbucket" {
		t.Errorf("lookup(bitbucket) = %+v, %v", e, ok)
	}
}
