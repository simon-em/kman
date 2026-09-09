package slack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/kranqpush"
	"github.com/simon-em/kman/internal/trigger"
)

type Handler struct {
	Home     string
	KranqURL string
}

func NewHandler(home, kranqURL string) *Handler {
	return &Handler{Home: home, KranqURL: kranqURL}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	provider, err := FromVault(h.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	if !VerifySignature(provider.SigningSecret, r.Header.Get("X-Slack-Request-Timestamp"), string(body), r.Header.Get("X-Slack-Signature")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	env, err := ParseEnvelope(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if env.Type == "url_verification" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(env.Challenge))
		return
	}
	if env.Event.BotID != "" || env.Event.Type != "app_mention" {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	go h.handle(provider, env.Event)
}

func (h *Handler) handle(provider *Provider, event Event) {
	ctx := context.Background()
	anchor := event.ThreadAnchor()
	reply := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if _, err := provider.Client.PostMessage(ctx, event.Channel, anchor, msg); err != nil {
			fmt.Fprintf(os.Stderr, "kman: slack: posting reply: %v\n", err)
		}
	}

	flowName, args, err := ParseCommand(event.Text)
	if err != nil {
		reply("%s", err.Error())
		return
	}

	cfg, err := config.Load(h.Home, "")
	if err != nil {
		reply("kman config error: %v", err)
		return
	}
	user, ok := cfg.Access.UserBySlackID(event.User)
	if !ok {
		reply("no kman user is linked to your Slack account; ask an admin to set slack_user_id")
		return
	}
	if !cfg.Access.CanTrigger(user.ID, flowName) {
		reply("%s is not authorized to trigger %q", user.ID, flowName)
		return
	}

	spec, err := config.LoadFlow(h.Home, flowName)
	if err != nil {
		reply("no such flow %q", flowName)
		return
	}
	if !trigger.LooksLikeRemote(spec.Repo) {
		reply("flow %q has no repo: set repo: to a remote URL for Slack-triggered runs", flowName)
		return
	}

	provided, err := trigger.ParseAssignments(args)
	if err != nil {
		reply("%v", err)
		return
	}

	extraEnv := map[string]string{}
	if spec.Access.HasMeta(AskMeta) {
		spec = InjectAskRelay(spec, newRunID(), event.Channel, anchor)
		extraEnv["SLACK_BOT_TOKEN"] = provider.Client.BotToken()
	}

	reply("running %s...", flowName)
	result, err := trigger.Run(ctx, h.Home, spec, provided, trigger.Options{
		KranqURL: h.KranqURL,
		Source:   spec.Repo,
		AsUser:   user.ID,
		ExtraEnv: extraEnv,
	})
	if err != nil {
		reply("%s: %v", flowName, err)
		return
	}
	if result.Status == kranqpush.StatusRefused {
		reply("%s: refused, exit=%d", flowName, result.ExitCode)
		return
	}
	reply("%s: id=%s status=%s exit=%d", flowName, result.ID, result.Status, result.ExitCode)
}

func newRunID() string {
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return fmt.Sprintf("kman-%s", hex.EncodeToString(nonce[:]))
	}
	return fmt.Sprintf("kman-%d", time.Now().UnixNano())
}
