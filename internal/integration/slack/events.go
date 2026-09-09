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

func strippedText(text string) string {
	return strings.TrimSpace(mentionPattern.ReplaceAllString(text, ""))
}

func parseExplicitRun(stripped string) (flowName string, args []string, isRun bool, err error) {
	fields := strings.Fields(stripped)
	if len(fields) == 0 || fields[0] != "run" {
		return "", nil, false, nil
	}
	if len(fields) < 2 {
		return "", nil, true, fmt.Errorf("no flow name given")
	}
	return fields[1], fields[2:], true, nil
}

func ParseCommand(text, defaultFlow string) (flowName string, args []string, err error) {
	stripped := strippedText(text)
	if stripped == "" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]" or describe what you want done`)
	}
	if name, a, isRun, rerr := parseExplicitRun(stripped); isRun {
		return name, a, rerr
	}
	if defaultFlow == "" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]"; no default flow is configured for free-form requests`)
	}
	return defaultFlow, []string{"TASK=" + stripped}, nil
}
