package web

import "testing"

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
