package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
}

type OAuth struct {
	cfg Config
}

func New(cfg Config) *OAuth {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://bitbucket.org"
	}
	return &OAuth{cfg: cfg}
}

type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func (o *OAuth) AuthorizeURL(state string) string {
	v := url.Values{}
	v.Set("client_id", o.cfg.ClientID)
	v.Set("response_type", "code")
	v.Set("state", state)
	return o.cfg.BaseURL + "/site/oauth2/authorize?" + v.Encode()
}

func (o *OAuth) Exchange(ctx context.Context, code string) (Token, error) {
	return o.request(ctx, url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	})
}

func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	return o.request(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (o *OAuth) request(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.cfg.BaseURL+"/site/oauth2/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(o.cfg.ClientID, o.cfg.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("bitbucket oauth: %s: %s", resp.Status, body)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("bitbucket oauth: response is not json: %w", err)
	}
	return Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}
