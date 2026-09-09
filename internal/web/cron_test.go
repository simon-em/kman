package web

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/simon-em/kman/internal/config"
)

func TestCronListEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/cron")
	if !bodyContains(t, resp, "no schedules yet") {
		t.Error("expected an empty-state message")
	}
}

func TestSaveCronEntryThenViewIt(t *testing.T) {
	srv, home := newTestServer(t)

	mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"name: report\nsteps:\n  - name: a\n    run: echo hi\n"},
	})

	resp := mustPost(t, srv, "/cron/save", url.Values{
		"name":     {"nightly"},
		"flow":     {"report"},
		"schedule": {"0 6 * * *"},
		"args":     {"ENV=staging\nFORCE=true"},
		"actor":    {"simon"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	e, err := config.LoadCronEntry(home, "nightly")
	if err != nil {
		t.Fatalf("LoadCronEntry: %v", err)
	}
	if e.Flow != "report" || e.Schedule != "0 6 * * *" {
		t.Errorf("e = %+v", e)
	}
	if e.Args["ENV"] != "staging" || e.Args["FORCE"] != "true" {
		t.Errorf("Args = %+v", e.Args)
	}
	if e.CreatedBy != "simon" {
		t.Errorf("CreatedBy = %q", e.CreatedBy)
	}

	view := mustGet(t, srv, "/cron/nightly")
	if !bodyContains(t, view, "ENV=staging") {
		t.Error("expected the entry's args to appear in the edit form")
	}
}

func TestSaveCronEntryWithAnInvalidScheduleShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	mustPost(t, srv, "/flows/save", url.Values{
		"yaml": {"name: report\nsteps:\n  - name: a\n    run: echo hi\n"},
	})

	resp := mustPost(t, srv, "/cron/save", url.Values{
		"name":     {"nightly"},
		"flow":     {"report"},
		"schedule": {"nonsense"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with the error re-rendered", resp.StatusCode)
	}
	if !bodyContains(t, resp, "error") {
		t.Error("expected an error message in the response")
	}
}

func TestViewMissingCronEntryIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/cron/no-such-entry")
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
