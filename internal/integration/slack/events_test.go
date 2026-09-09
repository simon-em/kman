package slack

import "testing"

func TestParseEnvelopeURLVerification(t *testing.T) {
	env, err := ParseEnvelope([]byte(`{"type":"url_verification","challenge":"abc123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "url_verification" || env.Challenge != "abc123" {
		t.Errorf("env = %+v", env)
	}
}

func TestParseEnvelopeAppMention(t *testing.T) {
	data := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U1","text":"<@UBOT> run deploy ENV=staging","ts":"1.1","thread_ts":"0.1"}}`
	env, err := ParseEnvelope([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if env.Event.Type != "app_mention" || env.Event.Channel != "C1" || env.Event.User != "U1" {
		t.Errorf("event = %+v", env.Event)
	}
}

func TestParseEnvelopeRejectsNonJSON(t *testing.T) {
	if _, err := ParseEnvelope([]byte("not json")); err == nil {
		t.Fatal("expected an error for non-json input")
	}
}

func TestThreadAnchorPrefersThreadTS(t *testing.T) {
	e := Event{TS: "1.1", ThreadTS: "0.1"}
	if e.ThreadAnchor() != "0.1" {
		t.Errorf("ThreadAnchor() = %q, want 0.1", e.ThreadAnchor())
	}
}

func TestThreadAnchorFallsBackToTS(t *testing.T) {
	e := Event{TS: "1.1"}
	if e.ThreadAnchor() != "1.1" {
		t.Errorf("ThreadAnchor() = %q, want 1.1", e.ThreadAnchor())
	}
}

func TestParseCommandStripsMentionAndParsesArgs(t *testing.T) {
	flowName, args, err := ParseCommand("<@U0BOT123> run deploy-review ENV=staging FORCE=true")
	if err != nil {
		t.Fatal(err)
	}
	if flowName != "deploy-review" {
		t.Errorf("flowName = %q", flowName)
	}
	if len(args) != 2 || args[0] != "ENV=staging" || args[1] != "FORCE=true" {
		t.Errorf("args = %v", args)
	}
}

func TestParseCommandRejectsAnythingButRun(t *testing.T) {
	if _, _, err := ParseCommand("<@U0BOT123> status"); err == nil {
		t.Fatal("expected an error for a non-run command")
	}
}

func TestParseCommandRejectsRunWithNoFlowName(t *testing.T) {
	if _, _, err := ParseCommand("<@U0BOT123> run"); err == nil {
		t.Fatal("expected an error for run with no flow name")
	}
}
