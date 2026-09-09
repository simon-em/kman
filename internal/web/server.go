package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"net/url"

	"github.com/simon-em/kman/internal/config"
)

//go:embed templates/*.html
var templateFS embed.FS

var pageTmpls = template.Must(template.ParseFS(templateFS, "templates/*.html"))

const layoutHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}} - kman</title>
<style>
body{font-family:sans-serif;max-width:900px;margin:2em auto;padding:0 1em;color:#222}
textarea{width:100%;font-family:monospace}
.error{color:#b00;font-weight:bold}
.warning{color:#960;font-weight:bold;background:#ffefcf;padding:.5em 1em;border-radius:4px}
fieldset{margin:1em 0;border:1px solid #ccc;border-radius:4px}
fieldset fieldset{background:#fafafa}
legend{font-weight:bold}
nav{margin-bottom:2em}
table{border-collapse:collapse;width:100%}
th,td{text-align:left;padding:.3em .6em;border-bottom:1px solid #eee}
.row{display:flex;gap:.5em;align-items:baseline;margin:.3em 0}
.row input[type=text],.row input[type=number]{flex:1}
.hint{color:#777;font-size:.9em}
label{display:block;margin:.5em 0}
label.inline{display:inline-block;margin-right:1em}
</style></head>
<body>
<nav><a href="/">kman</a> | <a href="/flows">flows</a> | <a href="/repos">repos</a> | <a href="/users">users</a> | <a href="/groups">groups</a> | <a href="/cron">cron</a> | <a href="/skills">skills</a> | <a href="/integrations">integrations</a></nav>
{{if .Warning}}<p class="warning">{{.Warning}}</p>{{end}}
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
	mux.HandleFunc("GET /repos", s.listRepos)
	mux.HandleFunc("GET /repos/new", s.newRepo)
	mux.HandleFunc("GET /repos/{name}", s.viewRepo)
	mux.HandleFunc("POST /repos/save", s.saveRepo)
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
	renderWithWarning(w, title, page, data, "")
}

func renderWithWarning(w http.ResponseWriter, title, page string, data any, warning string) {
	var body bytes.Buffer
	if err := pageTmpls.ExecuteTemplate(&body, page, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err := layoutTmpl.Execute(w, struct {
		Title   string
		Body    template.HTML
		Warning string
	}{title, template.HTML(body.String()), warning})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func redirectSaved(w http.ResponseWriter, r *http.Request, home, path string) {
	if _, err := config.PushIfConfigured(home); err != nil {
		path += "?push_warning=" + url.QueryEscape("saved locally, but could not push the config repo: "+err.Error())
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func pushWarning(r *http.Request) string {
	return r.URL.Query().Get("push_warning")
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
