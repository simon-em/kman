package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

func VerifySignature(signingSecret, timestamp, body, signature string) bool {
	if signingSecret == "" || timestamp == "" || signature == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if abs(time.Now().Unix()-ts) > 5*60 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
