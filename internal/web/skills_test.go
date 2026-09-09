package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
)

func TestSkillsListShowsCatalogEntries(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/skills")
	if !bodyContains(t, resp, "bitbucket") {
		t.Error("expected the bitbucket catalog entry listed")
	}
}

func TestSkillsListShowsACopyableReference(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/skills")
	if !bodyContains(t, resp, "catalog:bitbucket@") {
		t.Error("expected a copyable catalog: reference")
	}
}

func TestSkillsListMarksBuiltinsAndLinksCustomEntries(t *testing.T) {
	srv, home := newTestServer(t)
	e := catalog.StoredEntry{Name: "my-skill", Kind: catalog.KindDoc, Path: "x", Text: "hi"}
	if err := config.SaveSkill(home, e, ""); err != nil {
		t.Fatal(err)
	}
	body := readBody(t, mustGet(t, srv, "/skills"))
	if !strings.Contains(body, "built-in") {
		t.Error("expected the built-in bitbucket entry marked as built-in")
	}
	if !strings.Contains(body, "/skills/my-skill") {
		t.Error("expected an edit link for the custom entry")
	}
}

func TestSaveSkillThenViewIt(t *testing.T) {
	srv, home := newTestServer(t)

	resp := mustPost(t, srv, "/skills/save", url.Values{
		"is_new":      {"true"},
		"name":        {"my-skill"},
		"description": {"a custom skill"},
		"kind":        {"doc"},
		"path":        {".claude/skills/my-skill/SKILL.md"},
		"text":        {"# hello"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/skills/my-skill" {
		t.Errorf("Location = %q", loc)
	}

	e, err := config.LoadSkill(home, "my-skill")
	if err != nil {
		t.Fatalf("LoadSkill: %v", err)
	}
	if e.Text != "# hello" || e.Description != "a custom skill" {
		t.Errorf("e = %+v", e)
	}

	view := mustGet(t, srv, "/skills/my-skill")
	if !bodyContains(t, view, "# hello") {
		t.Error("expected the skill's content to appear in the edit form")
	}
}

func TestSaveSkillRejectsABuiltinNameCollision(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustPost(t, srv, "/skills/save", url.Values{
		"is_new": {"true"},
		"name":   {"bitbucket"},
		"kind":   {"doc"},
		"path":   {"x"},
		"text":   {"hi"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with the error re-rendered", resp.StatusCode)
	}
	if !bodyContains(t, resp, "error") {
		t.Error("expected an error message in the response")
	}
}

func TestViewMissingSkillIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/skills/no-such-skill")
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestFetchSkillPreviewFillsContentWithoutSaving(t *testing.T) {
	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fetched content"))
	}))
	t.Cleanup(fetchSrv.Close)

	srv, home := newTestServer(t)
	resp := mustPost(t, srv, "/skills/fetch-preview", url.Values{
		"is_new":      {"true"},
		"name":        {"my-skill"},
		"description": {"already typed before fetching"},
		"kind":        {"doc"},
		"path":        {".claude/skills/my-skill/SKILL.md"},
		"fetch_url":   {fetchSrv.URL},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "fetched content") {
		t.Error("expected the fetched content to appear in the form")
	}
	if !strings.Contains(body, "already typed before fetching") {
		t.Error("expected the already-typed description to survive the fetch round trip")
	}
	if _, err := config.LoadSkill(home, "my-skill"); err == nil {
		t.Error("fetch-preview must not save anything")
	}
}
