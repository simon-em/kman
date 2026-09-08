package bitbucket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fakeBitbucket(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *OAuth) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oauth := New(Config{ClientID: "cid", ClientSecret: "csecret", BaseURL: srv.URL})
	return srv, oauth
}

func TestAuthorizeURL(t *testing.T) {
	oauth := New(Config{ClientID: "cid", BaseURL: "https://bitbucket.org"})
	got := oauth.AuthorizeURL("state123")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "bitbucket.org" || u.Path != "/site/oauth2/authorize" {
		t.Errorf("AuthorizeURL = %q", got)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "state123" || q.Get("response_type") != "code" {
		t.Errorf("query = %v", q)
	}
}

func TestAuthorizeURLDefaultsBaseURL(t *testing.T) {
	oauth := New(Config{ClientID: "cid"})
	if !strings.HasPrefix(oauth.AuthorizeURL("s"), "https://bitbucket.org/site/oauth2/authorize") {
		t.Errorf("AuthorizeURL = %q", oauth.AuthorizeURL("s"))
	}
}

func TestExchangeSendsBasicAuthAndCode(t *testing.T) {
	var gotUser, gotPass string
	var gotGrantType, gotCode string
	_, oauth := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotGrantType = r.FormValue("grant_type")
		gotCode = r.FormValue("code")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","expires_in":3600}`))
	})

	tok, err := oauth.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotUser != "cid" || gotPass != "csecret" {
		t.Errorf("basic auth = %q/%q", gotUser, gotPass)
	}
	if gotGrantType != "authorization_code" || gotCode != "the-code" {
		t.Errorf("grant_type=%q code=%q", gotGrantType, gotCode)
	}
	if tok.AccessToken != "AT" || tok.RefreshToken != "RT" {
		t.Errorf("tok = %+v", tok)
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("ExpiresAt was not set")
	}
}

func TestRefreshSendsRefreshToken(t *testing.T) {
	var gotGrantType, gotRefreshToken string
	_, oauth := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotGrantType = r.FormValue("grant_type")
		gotRefreshToken = r.FormValue("refresh_token")
		w.Write([]byte(`{"access_token":"NEW_AT","refresh_token":"NEW_RT","expires_in":7200}`))
	})

	tok, err := oauth.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotGrantType != "refresh_token" || gotRefreshToken != "old-refresh" {
		t.Errorf("grant_type=%q refresh_token=%q", gotGrantType, gotRefreshToken)
	}
	if tok.AccessToken != "NEW_AT" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestExchangeSurfacesAnErrorResponse(t *testing.T) {
	_, oauth := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	})
	if _, err := oauth.Exchange(context.Background(), "bad-code"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestExchangeRejectsNonJSONResponse(t *testing.T) {
	_, oauth := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})
	if _, err := oauth.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("expected an error for a non-json response")
	}
}
