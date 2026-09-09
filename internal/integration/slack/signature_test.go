package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func sign(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureAcceptsAValidSignature(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	body := `{"type":"event_callback"}`
	sig := sign("shhh", ts, body)
	if !VerifySignature("shhh", ts, body, sig) {
		t.Error("expected a valid signature to verify")
	}
}

func TestVerifySignatureRejectsATamperedBody(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := sign("shhh", ts, "original")
	if VerifySignature("shhh", ts, "tampered", sig) {
		t.Error("expected a tampered body to fail verification")
	}
}

func TestVerifySignatureRejectsAnOldTimestamp(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	body := "x"
	sig := sign("shhh", ts, body)
	if VerifySignature("shhh", ts, body, sig) {
		t.Error("expected an hour-old timestamp to fail verification")
	}
}

func TestVerifySignatureRejectsAWrongSecret(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	body := "x"
	sig := sign("shhh", ts, body)
	if VerifySignature("different", ts, body, sig) {
		t.Error("expected a signature made with a different secret to fail verification")
	}
}
