package web

import (
	"fmt"
	"net/http"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/integration/slack"
)

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	render(w, "Skills", "skills_list", struct{ Entries []catalog.Entry }{catalog.List()})
}

type integrationRow struct {
	UserID    string
	Connected bool
}

type slackRow struct {
	UserID      string
	SlackUserID string
}

type integrationsData struct {
	Configured bool
	ConfigErr  string
	Rows       []integrationRow
	Actor      string

	SlackConfigured bool
	SlackConfigErr  string
	SlackRows       []slackRow
}

func (s *Server) listIntegrations(w http.ResponseWriter, r *http.Request) {
	ids, err := config.ListUserIDs(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	provider, provErr := bitbucket.FromVault(s.Home)
	data := integrationsData{Configured: provErr == nil, Actor: actorFrom(r)}
	if provErr != nil {
		data.ConfigErr = provErr.Error()
	}
	_, slackErr := slack.FromVault(s.Home)
	data.SlackConfigured = slackErr == nil
	if slackErr != nil {
		data.SlackConfigErr = slackErr.Error()
	}
	for _, id := range ids {
		connected := provErr == nil && provider.Connected(id)
		data.Rows = append(data.Rows, integrationRow{UserID: id, Connected: connected})
		u, err := config.LoadUser(s.Home, id)
		if err == nil {
			data.SlackRows = append(data.SlackRows, slackRow{UserID: id, SlackUserID: u.SlackUserID})
		}
	}
	render(w, "Integrations", "integrations_list", data)
}

func (s *Server) bitbucketConnect(w http.ResponseWriter, r *http.Request) {
	userID := actorFrom(r)
	if userID == "" {
		http.Error(w, `set "Acting as" on any page first, to a known user id`, http.StatusBadRequest)
		return
	}
	if _, err := config.LoadUser(s.Home, userID); err != nil {
		http.Error(w, fmt.Sprintf("no such user %q; create one first at /users/new", userID), http.StatusBadRequest)
		return
	}
	provider, err := bitbucket.FromVault(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	state := s.oauth.create(userID)
	http.Redirect(w, r, provider.OAuth.AuthorizeURL(state), http.StatusSeeOther)
}

func (s *Server) bitbucketCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if reason := q.Get("error"); reason != "" {
		http.Error(w, "bitbucket denied the request: "+reason, http.StatusBadRequest)
		return
	}
	userID, ok := s.oauth.consume(q.Get("state"))
	if !ok {
		http.Error(w, "unknown or expired oauth state; start over from /integrations", http.StatusBadRequest)
		return
	}
	provider, err := bitbucket.FromVault(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	tok, err := provider.OAuth.Exchange(r.Context(), q.Get("code"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := provider.Connect(userID, tok); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/integrations", http.StatusSeeOther)
}
