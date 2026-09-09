package slack

import (
	"fmt"

	"github.com/simon-em/kman/internal/integration"
	"github.com/simon-em/kman/internal/vault"
)

const (
	BotTokenKey      = "slack/bot_token"
	SigningSecretKey = "slack/signing_secret"
	BaseURLKey       = "slack/base_url"
)

type Provider struct {
	Client        *Client
	SigningSecret string
}

func FromVault(home string) (*Provider, error) {
	v, err := vault.Open(home)
	if err != nil {
		return nil, err
	}
	botToken, err := v.Get(BotTokenKey)
	if err != nil {
		return nil, fmt.Errorf("slack integration not configured (kman secret set %s <token>): %w", BotTokenKey, err)
	}
	signingSecret, err := v.Get(SigningSecretKey)
	if err != nil {
		return nil, fmt.Errorf("slack integration not configured (kman secret set %s <secret>): %w", SigningSecretKey, err)
	}
	baseURL, _ := v.Get(BaseURLKey)
	return &Provider{Client: New(Config{BotToken: botToken, BaseURL: baseURL}), SigningSecret: signingSecret}, nil
}

func (p *Provider) Name() string { return "slack" }

var (
	_ integration.Integration    = (*Provider)(nil)
	_ integration.TriggerChannel = (*Provider)(nil)
)
