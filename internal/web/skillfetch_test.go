package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchSkillContentFetchesTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from the fetched skill"))
	}))
	t.Cleanup(srv.Close)

	got, err := fetchSkillContent(srv.URL)
	if err != nil {
		t.Fatalf("fetchSkillContent: %v", err)
	}
	if got != "hello from the fetched skill" {
		t.Errorf("got = %q", got)
	}
}

func TestFetchSkillContentSurfacesANon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchSkillContent(srv.URL); err == nil {
		t.Fatal("expected an error for a 404")
	}
}

func TestFetchSkillContentRejectsAnOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxFetchBytes+1))
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchSkillContent(srv.URL); err == nil {
		t.Fatal("expected an error for an oversized response")
	}
}

func TestNormalizeFetchURLRewritesAGithubBlobURL(t *testing.T) {
	got := normalizeFetchURL("https://github.com/owner/repo/blob/main/path/to/SKILL.md")
	want := "https://raw.githubusercontent.com/owner/repo/main/path/to/SKILL.md"
	if got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestNormalizeFetchURLLeavesOtherURLsAlone(t *testing.T) {
	raw := "https://raw.githubusercontent.com/owner/repo/main/SKILL.md"
	if got := normalizeFetchURL(raw); got != raw {
		t.Errorf("got = %q, want it unchanged", got)
	}
}

func TestNormalizeFetchURLLeavesNonGithubURLsAlone(t *testing.T) {
	raw := "https://example.com/blob/main/x"
	if got := normalizeFetchURL(raw); got != raw {
		t.Errorf("got = %q, want it unchanged", got)
	}
}
