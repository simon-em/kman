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

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/dispatch"
	"github.com/simon-em/kman/internal/kranqpush"
	"github.com/simon-em/kman/internal/trigger"
)

type Handler struct {
	Home          string
	KranqURL      string
	MetaURL       string
	DefaultFlow   string
	Dispatch      bool
	DispatchModel string
}

func NewHandler(home, kranqURL, metaURL, defaultFlow string) *Handler {
	return &Handler{Home: home, KranqURL: kranqURL, MetaURL: metaURL, DefaultFlow: defaultFlow}
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

	flowName, provided, err := h.route(ctx, cfg, user, event, reply)
	if err != nil {
		reply("%s", err.Error())
		return
	}
	if flowName == "" {
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

	source := spec.Repo
	if repoRef := provided["REPO"]; repoRef != "" {
		resolved, err := trigger.ResolveRepoRef(h.Home, repoRef)
		if err != nil {
			reply("%v", err)
			return
		}
		source = resolved
	}
	if !trigger.LooksLikeRemote(source) {
		reply("flow %q has no repo: set repo: on the flow, or name one from `kman repo ls` with REPO=", flowName)
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
		Source:   source,
		AsUser:   user.ID,
		MetaURL:  h.MetaURL,
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

func (h *Handler) route(ctx context.Context, cfg config.Config, user access.User, event Event, reply func(string, ...any)) (flowName string, provided map[string]string, err error) {
	stripped := strippedText(event.Text)
	if stripped == "" {
		return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]" or describe what you want done`)
	}

	if name, args, isRun, rerr := parseExplicitRun(stripped); isRun {
		if rerr != nil {
			return "", nil, rerr
		}
		provided, err := trigger.ParseAssignments(args)
		if err != nil {
			return "", nil, err
		}
		return name, provided, nil
	}

	if !h.Dispatch {
		if h.DefaultFlow == "" {
			return "", nil, fmt.Errorf(`say "run <flow> [NAME=VALUE...]"; no default flow is configured for free-form requests`)
		}
		return h.DefaultFlow, map[string]string{"TASK": stripped}, nil
	}

	candidates, err := h.candidateFlows(cfg, user.ID)
	if err != nil {
		return "", nil, err
	}
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("no flows are granted to you; ask an admin to `kman grant user/%s <flow>`", user.ID)
	}
	repoRows, err := config.ListRepos(h.Home)
	if err != nil {
		return "", nil, err
	}
	repoCandidates := make([]dispatch.RepoCandidate, len(repoRows))
	for i, r := range repoRows {
		repoCandidates[i] = dispatch.RepoCandidate{Name: r.Name}
	}

	result, err := dispatch.Route(ctx, stripped, h.DispatchModel, candidates, repoCandidates)
	if err != nil {
		return "", nil, err
	}
	if result.Flow == "" {
		if result.Clarify != "" {
			reply("%s", result.Clarify)
		} else {
			reply(`not sure what you meant; try "run <flow> [NAME=VALUE...]"`)
		}
		return "", nil, nil
	}
	provided = map[string]string{"TASK": result.Task}
	if result.Repo != "" {
		provided["REPO"] = result.Repo
	}
	return result.Flow, provided, nil
}

func (h *Handler) candidateFlows(cfg config.Config, userID string) ([]dispatch.FlowCandidate, error) {
	names, err := config.ListFlowNames(h.Home)
	if err != nil {
		return nil, err
	}
	var candidates []dispatch.FlowCandidate
	for _, name := range names {
		if !cfg.Access.CanTrigger(userID, name) {
			continue
		}
		spec, err := config.LoadFlow(h.Home, name)
		if err != nil {
			continue
		}
		candidates = append(candidates, dispatch.FlowCandidate{Name: name, Description: spec.Description})
	}
	return candidates, nil
}

func newRunID() string {
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return fmt.Sprintf("kman-%s", hex.EncodeToString(nonce[:]))
	}
	return fmt.Sprintf("kman-%d", time.Now().UnixNano())
}
