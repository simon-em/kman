package bitbucket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/simon-em/kman/internal/vault"
)

func newTestProvider(t *testing.T, refreshHandler http.HandlerFunc) *Provider {
	t.Helper()
	home := t.TempDir()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(refreshHandler)
	t.Cleanup(srv.Close)
	oauth := New(Config{ClientID: "cid", ClientSecret: "csecret", BaseURL: srv.URL})
	return &Provider{OAuth: oauth, Vault: v}
}

func TestScopedCredentialMintsAClientCredentialsToken(t *testing.T) {
	var gotGrantType, gotScope string
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotGrantType = r.FormValue("grant_type")
		gotScope = r.FormValue("scope")
		w.Write([]byte(`{"access_token":"SCOPED_AT","expires_in":7200,"scope":"pullrequest"}`))
	})

	cred, err := p.ScopedCredential(context.Background(), "pullrequest")
	if err != nil {
		t.Fatalf("ScopedCredential: %v", err)
	}
	if gotGrantType != "client_credentials" || gotScope != "pullrequest" {
		t.Errorf("grant_type=%q scope=%q", gotGrantType, gotScope)
	}
	if cred.Value != "SCOPED_AT" {
		t.Errorf("Value = %q, want SCOPED_AT", cred.Value)
	}
}

func TestScopedCredentialNeverTouchesTheVault(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"SCOPED_AT","expires_in":7200}`))
	})
	if _, err := p.ScopedCredential(context.Background(), "pullrequest"); err != nil {
		t.Fatal(err)
	}
	if p.Connected("simon") {
		t.Error("a scoped credential should not register as a connected user")
	}
}

func TestScopedCredentialSurfacesAnError(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_scope"}`))
	})
	if _, err := p.ScopedCredential(context.Background(), "not-a-real-scope"); err == nil {
		t.Fatal("expected an error for an invalid scope")
	}
}

func TestCredentialFailsWhenNotConnected(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call bitbucket for a never-connected user")
	})
	_, err := p.Credential(context.Background(), "simon")
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestConnectedReflectsState(t *testing.T) {
	p := newTestProvider(t, nil)
	if p.Connected("simon") {
		t.Error("Connected = true before Connect")
	}
	if err := p.Connect("simon", Token{AccessToken: "AT", RefreshToken: "RT", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !p.Connected("simon") {
		t.Error("Connected = false after Connect")
	}
}

func TestCredentialReturnsStoredTokenWhenStillValid(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not refresh a still-valid token")
	})
	if err := p.Connect("simon", Token{AccessToken: "AT", RefreshToken: "RT", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cred, err := p.Credential(context.Background(), "simon")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if cred.Value != "AT" {
		t.Errorf("Value = %q, want AT", cred.Value)
	}
}

func TestCredentialRefreshesAnExpiredToken(t *testing.T) {
	refreshed := false
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		refreshed = true
		w.Write([]byte(`{"access_token":"NEW_AT","refresh_token":"NEW_RT","expires_in":3600}`))
	})
	if err := p.Connect("simon", Token{AccessToken: "OLD_AT", RefreshToken: "OLD_RT", ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cred, err := p.Credential(context.Background(), "simon")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if !refreshed {
		t.Error("expected the expired token to trigger a refresh")
	}
	if cred.Value != "NEW_AT" {
		t.Errorf("Value = %q, want NEW_AT", cred.Value)
	}

	again, err := p.Credential(context.Background(), "simon")
	if err != nil {
		t.Fatal(err)
	}
	if again.Value != "NEW_AT" {
		t.Errorf("second Credential call = %q, want the refreshed token cached", again.Value)
	}
}

func TestFromVaultFailsWithoutConfiguredClientCredentials(t *testing.T) {
	home := t.TempDir()
	if _, err := FromVault(home); err == nil {
		t.Fatal("expected an error when no client_id/client_secret is stored")
	}
}

func TestFromVaultSucceedsOnceConfigured(t *testing.T) {
	home := t.TempDir()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ClientIDKey, "cid"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ClientSecretKey, "csecret"); err != nil {
		t.Fatal(err)
	}
	p, err := FromVault(home)
	if err != nil {
		t.Fatalf("FromVault: %v", err)
	}
	if p.OAuth == nil {
		t.Fatal("OAuth was not set up")
	}
}

func TestFromVaultHonorsBaseURLOverride(t *testing.T) {
	home := t.TempDir()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ClientIDKey, "cid"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ClientSecretKey, "csecret"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(BaseURLKey, "https://on-prem.example.com"); err != nil {
		t.Fatal(err)
	}
	p, err := FromVault(home)
	if err != nil {
		t.Fatalf("FromVault: %v", err)
	}
	got := p.OAuth.AuthorizeURL("s")
	if !strings.HasPrefix(got, "https://on-prem.example.com/") {
		t.Errorf("AuthorizeURL = %q, want it to use the overridden base URL", got)
	}
}
