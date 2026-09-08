package config

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/flow"
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
