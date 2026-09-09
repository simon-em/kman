package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Config struct {
	BotToken string
	BaseURL  string
}

type Client struct {
	cfg Config
}

func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://slack.com/api"
	}
	return &Client{cfg: cfg}
}

func (c *Client) BotToken() string { return c.cfg.BotToken }

type postMessageRequest struct {
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	TS    string `json:"ts"`
}

func (c *Client) PostMessage(ctx context.Context, channel, threadTS, text string) (string, error) {
	body, err := json.Marshal(postMessageRequest{Channel: channel, Text: text, ThreadTS: threadTS})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out apiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("slack chat.postMessage: response is not json: %w", err)
	}
	if !out.OK {
		return "", fmt.Errorf("slack chat.postMessage: %s", out.Error)
	}
	return out.TS, nil
}
