package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/simon-em/kman/internal/integration"
	"github.com/simon-em/kman/internal/vault"
)

const (
	ClientIDKey     = "bitbucket/oauth/client_id"
	ClientSecretKey = "bitbucket/oauth/client_secret"
	BaseURLKey      = "bitbucket/oauth/base_url"
)

var ErrNotConnected = errors.New("bitbucket: user has not connected an account")

type Provider struct {
	OAuth *OAuth
	Vault *vault.Vault
}

func FromVault(home string) (*Provider, error) {
	v, err := vault.Open(home)
	if err != nil {
		return nil, err
	}
	clientID, err := v.Get(ClientIDKey)
	if err != nil {
		return nil, fmt.Errorf("bitbucket integration not configured (kman secret set %s <id>): %w", ClientIDKey, err)
	}
	clientSecret, err := v.Get(ClientSecretKey)
	if err != nil {
		return nil, fmt.Errorf("bitbucket integration not configured (kman secret set %s <secret>): %w", ClientSecretKey, err)
	}
	baseURL, _ := v.Get(BaseURLKey)
	return &Provider{OAuth: New(Config{ClientID: clientID, ClientSecret: clientSecret, BaseURL: baseURL}), Vault: v}, nil
}

func (p *Provider) Name() string { return "bitbucket" }

func (p *Provider) Credential(ctx context.Context, userID string) (integration.Credential, error) {
	tok, err := p.load(userID)
	if err != nil {
		return integration.Credential{}, err
	}
	if time.Now().Before(tok.ExpiresAt.Add(-1 * time.Minute)) {
		return integration.Credential{Value: tok.AccessToken, ExpiresAt: tok.ExpiresAt}, nil
	}
	refreshed, err := p.OAuth.Refresh(ctx, tok.RefreshToken)
	if err != nil {
		return integration.Credential{}, fmt.Errorf("refreshing bitbucket token for %s: %w", userID, err)
	}
	if err := p.save(userID, refreshed); err != nil {
		return integration.Credential{}, err
	}
	return integration.Credential{Value: refreshed.AccessToken, ExpiresAt: refreshed.ExpiresAt}, nil
}

func (p *Provider) Connect(userID string, tok Token) error {
	return p.save(userID, tok)
}

func (p *Provider) Connected(userID string) bool {
	_, err := p.load(userID)
	return err == nil
}

func (p *Provider) load(userID string) (Token, error) {
	access, err := p.Vault.Get(accessKey(userID))
	if errors.Is(err, vault.ErrNotFound) {
		return Token{}, ErrNotConnected
	}
	if err != nil {
		return Token{}, err
	}
	refresh, err := p.Vault.Get(refreshKey(userID))
	if errors.Is(err, vault.ErrNotFound) {
		return Token{}, ErrNotConnected
	}
	if err != nil {
		return Token{}, err
	}
	expiresRaw, err := p.Vault.Get(expiresKey(userID))
	if err != nil {
		return Token{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresRaw)
	if err != nil {
		return Token{}, fmt.Errorf("stored expiry for %s is corrupt: %w", userID, err)
	}
	return Token{AccessToken: access, RefreshToken: refresh, ExpiresAt: expiresAt}, nil
}

func (p *Provider) save(userID string, tok Token) error {
	if err := p.Vault.Set(accessKey(userID), tok.AccessToken); err != nil {
		return err
	}
	if err := p.Vault.Set(refreshKey(userID), tok.RefreshToken); err != nil {
		return err
	}
	return p.Vault.Set(expiresKey(userID), tok.ExpiresAt.Format(time.RFC3339))
}

func accessKey(userID string) string  { return "bitbucket/oauth/" + userID + "/access_token" }
func refreshKey(userID string) string { return "bitbucket/oauth/" + userID + "/refresh_token" }
func expiresKey(userID string) string { return "bitbucket/oauth/" + userID + "/expires_at" }
