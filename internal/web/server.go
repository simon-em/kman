package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/cron"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/integration/slack"
	"github.com/simon-em/kman/internal/trigger"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*.html
var templateFS embed.FS

var pageTmpls = template.Must(template.ParseFS(templateFS, "templates/*.html"))

const layoutHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}} - kman</title>
<style>
body{font-family:sans-serif;max-width:800px;margin:2em auto;padding:0 1em;color:#222}
textarea{width:100%;font-family:monospace}
.error{color:#b00;font-weight:bold}
fieldset{margin:1em 0}
nav{margin-bottom:2em}
</style></head>
<body>
<nav><a href="/">kman</a> | <a href="/flows">flows</a> | <a href="/users">users</a> | <a href="/groups">groups</a> | <a href="/cron">cron</a> | <a href="/skills">skills</a> | <a href="/integrations">integrations</a></nav>
{{.Body}}
</body></html>`

var layoutTmpl = template.Must(template.New("layout").Parse(layoutHTML))

const actorCookie = "kman_actor"

type Server struct {
	Home  string
	oauth *oauthState
}

func New(home string) *Server {
	return &Server{Home: home, oauth: newOAuthState()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /flows", s.listFlows)
	mux.HandleFunc("GET /flows/new", s.newFlow)
	mux.HandleFunc("GET /flows/{name}", s.viewFlow)
	mux.HandleFunc("POST /flows/save", s.saveFlow)
	mux.HandleFunc("GET /users", s.listUsers)
	mux.HandleFunc("GET /users/new", s.newUser)
	mux.HandleFunc("GET /users/{id}", s.viewUser)
	mux.HandleFunc("POST /users/save", s.saveUser)
	mux.HandleFunc("GET /groups", s.listGroups)
	mux.HandleFunc("GET /groups/new", s.newGroup)
	mux.HandleFunc("GET /groups/{name}", s.viewGroup)
	mux.HandleFunc("POST /groups/save", s.saveGroup)
	mux.HandleFunc("GET /cron", s.listCron)
	mux.HandleFunc("GET /cron/new", s.newCron)
	mux.HandleFunc("GET /cron/{name}", s.viewCron)
	mux.HandleFunc("POST /cron/save", s.saveCron)
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("GET /integrations", s.listIntegrations)
	mux.HandleFunc("GET /integrations/bitbucket/connect", s.bitbucketConnect)
	mux.HandleFunc("GET /integrations/bitbucket/callback", s.bitbucketCallback)
	return mux
}

func render(w http.ResponseWriter, title, page string, data any) {
	var body bytes.Buffer
	if err := pageTmpls.ExecuteTemplate(&body, page, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err := layoutTmpl.Execute(w, struct {
		Title string
		Body  template.HTML
	}{title, template.HTML(body.String())})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func actorFrom(r *http.Request) string {
	c, err := r.Cookie(actorCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func setActorCookie(w http.ResponseWriter, actor string) {
	if actor == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: actorCookie, Value: actor, Path: "/"})
}

func toSet(list []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range list {
		m[v] = true
	}
	return m
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	render(w, "kman", "index", nil)
}

type flowEditData struct {
	Name  string
	YAML  string
	Actor string
	Error string
}

func (s *Server) listFlows(w http.ResponseWriter, r *http.Request) {
	names, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "Flows", "flows_list", struct{ Names []string }{names})
}

func (s *Server) newFlow(w http.ResponseWriter, r *http.Request) {
	render(w, "New flow", "flow_edit", flowEditData{Actor: actorFrom(r)})
}

func (s *Server) viewFlow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := config.ReadFlowRaw(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, "Flow: "+name, "flow_edit", flowEditData{Name: name, YAML: string(data), Actor: actorFrom(r)})
}

func (s *Server) saveFlow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	yamlText := r.FormValue("yaml")
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)

	spec, err := flow.Parse([]byte(yamlText))
	if err != nil {
		render(w, "Flow error", "flow_edit", flowEditData{YAML: yamlText, Actor: actorVal, Error: err.Error()})
		return
	}
	if err := config.SaveFlow(s.Home, spec, actorVal); err != nil {
		render(w, "Flow error", "flow_edit", flowEditData{Name: spec.Name, YAML: yamlText, Actor: actorVal, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/flows/"+spec.Name, http.StatusSeeOther)
}

type userEditData struct {
	IsNew    bool
	User     access.User
	AllFlows []string
	Granted  map[string]bool
	Actor    string
	Error    string
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	ids, err := config.ListUserIDs(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	users := make([]access.User, 0, len(ids))
	for _, id := range ids {
		u, err := config.LoadUser(s.Home, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}
	render(w, "Users", "users_list", struct{ Users []access.User }{users})
}

func (s *Server) newUser(w http.ResponseWriter, r *http.Request) {
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "New user", "user_edit", userEditData{IsNew: true, AllFlows: flows, Granted: map[string]bool{}, Actor: actorFrom(r)})
}

func (s *Server) viewUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := config.LoadUser(s.Home, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "User: "+id, "user_edit", userEditData{User: u, AllFlows: flows, Granted: toSet(u.Flows), Actor: actorFrom(r)})
}

func (s *Server) saveUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	u := access.User{
		ID:          r.FormValue("id"),
		DisplayName: r.FormValue("display_name"),
		SlackUserID: r.FormValue("slack_id"),
		Flows:       r.Form["flows"],
	}
	flows, _ := config.ListFlowNames(s.Home)

	data, err := yaml.Marshal(u)
	if err == nil {
		_, err = access.ParseUser(data)
	}
	if err != nil {
		render(w, "User error", "user_edit", userEditData{User: u, AllFlows: flows, Granted: toSet(u.Flows), Actor: actorVal, Error: err.Error()})
		return
	}
	if err := config.SaveUser(s.Home, u, actorVal); err != nil {
		render(w, "User error", "user_edit", userEditData{User: u, AllFlows: flows, Granted: toSet(u.Flows), Actor: actorVal, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/users/"+u.ID, http.StatusSeeOther)
}

type groupEditData struct {
	IsNew    bool
	Group    access.Group
	AllUsers []string
	AllFlows []string
	Members  map[string]bool
	Granted  map[string]bool
	Actor    string
	Error    string
}

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	names, err := config.ListGroupNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	groups := make([]access.Group, 0, len(names))
	for _, name := range names {
		g, err := config.LoadGroup(s.Home, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		groups = append(groups, g)
	}
	render(w, "Groups", "groups_list", struct{ Groups []access.Group }{groups})
}

func (s *Server) newGroup(w http.ResponseWriter, r *http.Request) {
	users, err := config.ListUserIDs(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "New group", "group_edit", groupEditData{
		IsNew: true, AllUsers: users, AllFlows: flows,
		Members: map[string]bool{}, Granted: map[string]bool{}, Actor: actorFrom(r),
	})
}

func (s *Server) viewGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, err := config.LoadGroup(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	users, err := config.ListUserIDs(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "Group: "+name, "group_edit", groupEditData{
		Group: g, AllUsers: users, AllFlows: flows,
		Members: toSet(g.Members), Granted: toSet(g.Flows), Actor: actorFrom(r),
	})
}

func (s *Server) saveGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	g := access.Group{
		Name:    r.FormValue("name"),
		Members: r.Form["members"],
		Flows:   r.Form["flows"],
	}
	users, _ := config.ListUserIDs(s.Home)
	flows, _ := config.ListFlowNames(s.Home)

	data, err := yaml.Marshal(g)
	if err == nil {
		_, err = access.ParseGroup(data)
	}
	if err != nil {
		render(w, "Group error", "group_edit", groupEditData{
			Group: g, AllUsers: users, AllFlows: flows,
			Members: toSet(g.Members), Granted: toSet(g.Flows), Actor: actorVal, Error: err.Error(),
		})
		return
	}
	if err := config.SaveGroup(s.Home, g, actorVal); err != nil {
		render(w, "Group error", "group_edit", groupEditData{
			Group: g, AllUsers: users, AllFlows: flows,
			Members: toSet(g.Members), Granted: toSet(g.Flows), Actor: actorVal, Error: err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/groups/"+g.Name, http.StatusSeeOther)
}

type cronEditData struct {
	IsNew    bool
	Entry    cron.Entry
	AllFlows []string
	ArgsText string
	Actor    string
	Error    string
}

func (s *Server) listCron(w http.ResponseWriter, r *http.Request) {
	names, err := config.ListCronNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entries := make([]cron.Entry, 0, len(names))
	for _, n := range names {
		e, err := config.LoadCronEntry(s.Home, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}
	render(w, "Cron", "cron_list", struct{ Entries []cron.Entry }{entries})
}

func (s *Server) newCron(w http.ResponseWriter, r *http.Request) {
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "New cron entry", "cron_edit", cronEditData{IsNew: true, AllFlows: flows, Actor: actorFrom(r)})
}

func (s *Server) viewCron(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, err := config.LoadCronEntry(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	flows, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "Cron: "+name, "cron_edit", cronEditData{Entry: e, AllFlows: flows, ArgsText: argsToText(e.Args), Actor: actorFrom(r)})
}

func (s *Server) saveCron(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	argsText := r.FormValue("args")
	args, err := textToArgs(argsText)
	flows, _ := config.ListFlowNames(s.Home)
	e := cron.Entry{
		Name:      r.FormValue("name"),
		Flow:      r.FormValue("flow"),
		Schedule:  r.FormValue("schedule"),
		Args:      args,
		CreatedBy: actorVal,
	}
	if err == nil {
		err = e.Validate()
	}
	if err != nil {
		render(w, "Cron error", "cron_edit", cronEditData{Entry: e, AllFlows: flows, ArgsText: argsText, Actor: actorVal, Error: err.Error()})
		return
	}
	if err := config.SaveCronEntry(s.Home, e, actorVal); err != nil {
		render(w, "Cron error", "cron_edit", cronEditData{Entry: e, AllFlows: flows, ArgsText: argsText, Actor: actorVal, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/cron/"+e.Name, http.StatusSeeOther)
}

func argsToText(args map[string]string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, args[k])
	}
	return b.String()
}

func textToArgs(text string) (map[string]string, error) {
	var assignments []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		assignments = append(assignments, line)
	}
	return trigger.ParseAssignments(assignments)
}

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
