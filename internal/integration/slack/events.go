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

func ParseCommand(text, defaultFlow string) (flowName string, args []string, err error) {
	stripped := strings.TrimSpace(mentionPattern.ReplaceAllString(text, ""))
	if stripped == "" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]" or describe what you want done`)
	}
	fields := strings.Fields(stripped)
	if fields[0] == "run" {
		if len(fields) < 2 {
			return "", nil, fmt.Errorf("no flow name given")
		}
		return fields[1], fields[2:], nil
	}
	if defaultFlow == "" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]"; no default flow is configured for free-form requests`)
	}
	return defaultFlow, []string{"TASK=" + stripped}, nil
}
