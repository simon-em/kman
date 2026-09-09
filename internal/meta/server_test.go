package meta

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/flow"
)

func TestCreateCronRequiresABearerToken(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(NewServer(home).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/meta/cron", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCreateCronRejectsATokenNotGrantedCronCreate(t *testing.T) {
	home := t.TempDir()
	s := NewServer(home)
	tok, err := s.Store.Mint("deploy-review", "simon", []string{"something.else"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/meta/cron", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCreateCronCreatesAnEntryScopedToTheTokenSFlow(t *testing.T) {
	home := t.TempDir()
	if err := config.SaveFlow(home, flow.Spec{
		Name:   "deploy-review",
		Access: flow.Access{Meta: []string{"cron.create"}},
		Steps:  []flow.Step{{Name: "a", Run: "echo hi"}},
	}, ""); err != nil {
		t.Fatal(err)
	}
	s := NewServer(home)
	tok, err := s.Store.Mint("deploy-review", "simon", []string{"cron.create"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(createCronRequest{Name: "nightly", Schedule: "0 6 * * *"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/meta/cron", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	entry, err := config.LoadCronEntry(home, "nightly")
	if err != nil {
		t.Fatalf("LoadCronEntry: %v", err)
	}
	if entry.Flow != "deploy-review" || entry.CreatedBy != "simon" {
		t.Errorf("entry = %+v", entry)
	}
}

func TestCreateCronIgnoresAFlowNameInTheRequestBody(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"deploy-review", "some-other-flow"} {
		if err := config.SaveFlow(home, flow.Spec{Name: name, Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}, ""); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(home)
	tok, err := s.Store.Mint("deploy-review", "simon", []string{"cron.create"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	body := []byte(`{"name":"nightly","schedule":"0 6 * * *","flow":"some-other-flow"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/meta/cron", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	if _, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	}

	entry, err := config.LoadCronEntry(home, "nightly")
	if err != nil {
		t.Fatalf("LoadCronEntry: %v", err)
	}
	if entry.Flow != "deploy-review" {
		t.Errorf("Flow = %q, want deploy-review (the token's own flow, not the request body's)", entry.Flow)
	}
}
