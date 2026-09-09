package slack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type Envelope struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Event     Event  `json:"event"`
}

type Event struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
}

func ParseEnvelope(body []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return Envelope{}, fmt.Errorf("slack event: not json: %w", err)
	}
	return e, nil
}

func (e Event) ThreadAnchor() string {
	if e.ThreadTS != "" {
		return e.ThreadTS
	}
	return e.TS
}

var mentionPattern = regexp.MustCompile(`^\s*<@[A-Z0-9]+>\s*`)

func ParseCommand(text string) (flowName string, args []string, err error) {
	text = mentionPattern.ReplaceAllString(text, "")
	fields := strings.Fields(text)
	if len(fields) == 0 || fields[0] != "run" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]"`)
	}
	if len(fields) < 2 {
		return "", nil, fmt.Errorf("no flow name given")
	}
	return fields[1], fields[2:], nil
}
