package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostMessageSendsAuthAndBody(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"ok":true,"ts":"123.456"}`))
	}))
	t.Cleanup(srv.Close)

	c := New(Config{BotToken: "xoxb-token", BaseURL: srv.URL})
	ts, err := c.PostMessage(context.Background(), "C1", "T1", "hello")
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if ts != "123.456" {
		t.Errorf("ts = %q", ts)
	}
	if gotAuth != "Bearer xoxb-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody["channel"] != "C1" || gotBody["text"] != "hello" || gotBody["thread_ts"] != "T1" {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestPostMessageSurfacesAnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	t.Cleanup(srv.Close)

	c := New(Config{BotToken: "t", BaseURL: srv.URL})
	if _, err := c.PostMessage(context.Background(), "C1", "", "hello"); err == nil {
		t.Fatal("expected an error for ok:false")
	}
}
