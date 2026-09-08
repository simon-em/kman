package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/config"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	home := t.TempDir()
	srv := httptest.NewServer(New(home).Handler())
	t.Cleanup(srv.Close)
	return srv, home
}

func mustGet(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustPost(t *testing.T, srv *httptest.Server, path string, form url.Values) *http.Response {
	t.Helper()
	client := srv.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.PostForm(srv.URL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func bodyContains(t *testing.T, resp *http.Response, needle string) bool {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(data), needle)
}

func TestIndexServesLinks(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !bodyContains(t, resp, "/flows") {
		t.Error("expected the index page to link to /flows")
	}
}

func TestFlowsListEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/flows")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !bodyContains(t, resp, "no flows yet") {
		t.Error("expected an empty-state message")
	}
}

func TestSaveFlowThenViewIt(t *testing.T) {
	srv, home := newTestServer(t)

	resp := mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"name: deploy-review\nsteps:\n  - name: a\n    run: echo hi\n"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/flows/deploy-review" {
		t.Errorf("Location = %q", loc)
	}

	spec, err := config.LoadFlow(home, "deploy-review")
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if spec.Name != "deploy-review" {
		t.Errorf("Name = %q", spec.Name)
	}

	view := mustGet(t, srv, "/flows/deploy-review")
	if !bodyContains(t, view, "run: echo hi") {
		t.Error("expected the flow's YAML to appear in the edit form")
	}
}

func TestSaveFlowWithInvalidYAMLShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"steps: []\n"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with the error re-rendered", resp.StatusCode)
	}
	if !bodyContains(t, resp, "error") {
		t.Error("expected an error message in the response")
	}
}

func TestViewMissingFlowIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/flows/no-such-flow")
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSaveUserThenGrantAFlow(t *testing.T) {
	srv, home := newTestServer(t)

	mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"name: deploy-review\nsteps:\n  - name: a\n    run: echo hi\n"},
	})

	resp := mustPost(t, srv, "/users/save", url.Values{
		"id":           {"simon"},
		"display_name": {"Simon"},
		"slack_id":     {"U123"},
		"flows":        {"deploy-review"},
		"actor":        {"simon"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	u, err := config.LoadUser(home, "simon")
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if len(u.Flows) != 1 || u.Flows[0] != "deploy-review" {
		t.Errorf("Flows = %v", u.Flows)
	}
	if u.SlackUserID != "U123" {
		t.Errorf("SlackUserID = %q", u.SlackUserID)
	}
}

func TestSaveUserWithNoIDShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustPost(t, srv, "/users/save", url.Values{"display_name": {"No ID"}})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with the error re-rendered", resp.StatusCode)
	}
	if !bodyContains(t, resp, "error") {
		t.Error("expected an error message in the response")
	}
}

func TestSaveGroupWithMembersAndGrant(t *testing.T) {
	srv, home := newTestServer(t)

	mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"name: run-tests\nsteps:\n  - name: a\n    run: echo hi\n"},
	})
	mustPost(t, srv, "/users/save", url.Values{"id": {"alex"}})

	resp := mustPost(t, srv, "/groups/save", url.Values{
		"name":    {"oncall"},
		"members": {"alex"},
		"flows":   {"run-tests"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	g, err := config.LoadGroup(home, "oncall")
	if err != nil {
		t.Fatalf("LoadGroup: %v", err)
	}
	if len(g.Members) != 1 || g.Members[0] != "alex" {
		t.Errorf("Members = %v", g.Members)
	}
	if len(g.Flows) != 1 || g.Flows[0] != "run-tests" {
		t.Errorf("Flows = %v", g.Flows)
	}
}

func TestActorCookieIsSetAndReflectedBack(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}, "actor": {"alex"}})
	var actorCookieValue string
	for _, c := range resp.Cookies() {
		if c.Name == "kman_actor" {
			actorCookieValue = c.Value
		}
	}
	if actorCookieValue != "alex" {
		t.Errorf("kman_actor cookie = %q, want alex", actorCookieValue)
	}
}

func TestSaveCommitsWhenConfigDirIsAGitRepo(t *testing.T) {
	home := t.TempDir()
	dir := config.Dir(home)
	initCmd := exec.Command("git", "init", "-q", dir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	srv := httptest.NewServer(New(home).Handler())
	t.Cleanup(srv.Close)

	mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}, "actor": {"alex"}})

	log := exec.Command("git", "log", "-1", "--format=%an %s")
	log.Dir = dir
	out, err := log.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "alex") || !strings.Contains(string(out), "simon") {
		t.Errorf("git log = %q, want it to mention alex and simon", out)
	}
}
