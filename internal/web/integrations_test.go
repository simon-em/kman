package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/vault"
)

func configureBitbucket(t *testing.T, home, baseURL string) {
	t.Helper()
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
	if baseURL != "" {
		if err := v.Set(bitbucket.BaseURLKey, baseURL); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntegrationsPageShowsNotConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/integrations")
	if !bodyContains(t, resp, "Not configured") {
		t.Error("expected a not-configured message")
	}
}

func TestIntegrationsPageListsUsersOnceConfigured(t *testing.T) {
	srv, home := newTestServer(t)
	configureBitbucket(t, home, "")
	mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}})

	resp := mustGet(t, srv, "/integrations")
	body := readBody(t, resp)
	if !strings.Contains(body, "simon") || !strings.Contains(body, "not connected") {
		t.Errorf("expected simon listed as not connected:\n%s", body)
	}
}

func TestBitbucketConnectRequiresActor(t *testing.T) {
	srv, home := newTestServer(t)
	configureBitbucket(t, home, "")

	resp := mustGet(t, srv, "/integrations/bitbucket/connect")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 with no actor set", resp.StatusCode)
	}
}

func TestBitbucketConnectRedirectsToAuthorize(t *testing.T) {
	srv, home := newTestServer(t)
	configureBitbucket(t, home, "")
	mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}, "actor": {"simon"}})

	client := srv.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/integrations/bitbucket/connect", nil)
	req.AddCookie(&http.Cookie{Name: actorCookie, Value: "simon"})
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "bitbucket.org/site/oauth2/authorize") || !strings.Contains(loc, "client_id=cid") {
		t.Errorf("Location = %q", loc)
	}
}

func TestBitbucketCallbackExchangesCodeAndStoresToken(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","expires_in":3600}`))
	}))
	t.Cleanup(fake.Close)

	srv, home := newTestServer(t)
	configureBitbucket(t, home, fake.URL)
	mustPost(t, srv, "/users/save", url.Values{"id": {"simon"}, "actor": {"simon"}})

	provider, err := bitbucket.FromVault(home)
	if err != nil {
		t.Fatal(err)
	}
	state := "test-state"

	client := srv.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	connectReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/integrations/bitbucket/connect", nil)
	connectReq.AddCookie(&http.Cookie{Name: actorCookie, Value: "simon"})
	connectResp, err := client.Do(connectReq)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := url.Parse(connectResp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state = loc.Query().Get("state")

	callback := srv.URL + "/integrations/bitbucket/callback?code=abc&state=" + state
	resp, err := client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if !provider.Connected("simon") {
		t.Error("expected simon to be connected after the callback")
	}
}

func TestBitbucketCallbackRejectsUnknownState(t *testing.T) {
	srv, home := newTestServer(t)
	configureBitbucket(t, home, "")

	resp := mustGet(t, srv, "/integrations/bitbucket/callback?code=abc&state=bogus")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestBitbucketCallbackSurfacesDenial(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := mustGet(t, srv, "/integrations/bitbucket/callback?error=access_denied")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !bodyContains(t, resp, "denied") {
		t.Error("expected the denial reason in the response")
	}
}
