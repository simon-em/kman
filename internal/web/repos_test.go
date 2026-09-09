package web

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/simon-em/kman/internal/config"
)

func TestReposListEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/repos")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !bodyContains(t, resp, "no repos registered") {
		t.Error("expected an empty-state message")
	}
}

func TestSaveRepoThenViewIt(t *testing.T) {
	srv, home := newTestServer(t)

	resp := mustPost(t, srv, "/repos/save", url.Values{
		"is_new": {"true"},
		"name":   {"dx"},
		"url":    {"https://bitbucket.org/smntlbt/dx.git"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/repos/dx" {
		t.Errorf("Location = %q", loc)
	}

	repo, err := config.LoadRepo(home, "dx")
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if repo.URL != "https://bitbucket.org/smntlbt/dx.git" {
		t.Errorf("URL = %q", repo.URL)
	}

	view := mustGet(t, srv, "/repos/dx")
	if !bodyContains(t, view, "https://bitbucket.org/smntlbt/dx.git") {
		t.Error("expected the repo's URL to appear in the edit form")
	}
}

func TestSaveRepoWithNoURLShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustPost(t, srv, "/repos/save", url.Values{"is_new": {"true"}, "name": {"dx"}})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with the error re-rendered", resp.StatusCode)
	}
	if !bodyContains(t, resp, "error") {
		t.Error("expected an error message in the response")
	}
}

func TestViewMissingRepoIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/repos/no-such-repo")
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
