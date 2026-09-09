package web

import (
	"net/http"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/reporegistry"
	"github.com/simon-em/kman/internal/vault"
	"gopkg.in/yaml.v3"
)

type flowEditData struct {
	IsNew       bool
	Name        string
	Description string
	Repo        string
	Branch      string
	Kranqfile   string
	Memory      string
	CPUs        string
	Disk        string

	Args        []argRow
	Env         []kvRow
	Credentials []kvRow
	Files       []fileRow
	Steps       []stepRow

	AccessTools          []string
	AccessMetaAsk        bool
	AccessMetaCronCreate bool
	AccessSkills         []string
	AccessCredentials    []string
	AccessFlows          map[string]bool

	AllRepos        []reporegistry.Repo
	AllFlows        []string
	AllSkills       []catalog.Entry
	AllSecrets      []string
	PermissionModes []string

	Actor string
	Error string
}

func (s *Server) flowFormContext() (repos []reporegistry.Repo, flows []string, skills []catalog.Entry, secrets []string, err error) {
	repos, err = config.ListRepos(s.Home)
	if err != nil {
		return
	}
	flows, err = config.ListFlowNames(s.Home)
	if err != nil {
		return
	}
	skills, err = config.ListSkills(s.Home)
	if err != nil {
		return
	}
	if v, verr := vault.Open(s.Home); verr == nil {
		secrets, _ = v.List()
	}
	return
}

func (s *Server) listFlows(w http.ResponseWriter, r *http.Request) {
	names, err := config.ListFlowNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type row struct{ Name, Description string }
	rows := make([]row, 0, len(names))
	for _, n := range names {
		spec, err := config.LoadFlow(s.Home, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rows = append(rows, row{n, spec.Description})
	}
	render(w, "Flows", "flows_list", struct{ Rows []row }{rows})
}

func (s *Server) newFlow(w http.ResponseWriter, r *http.Request) {
	repos, flows, skills, secrets, err := s.flowFormContext()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := specToEditData(flow.Spec{})
	data.IsNew = true
	data.AccessFlows = map[string]bool{}
	data.AllRepos, data.AllFlows, data.AllSkills, data.AllSecrets = repos, flows, skills, secrets
	data.PermissionModes = flow.PermissionModes
	data.Actor = actorFrom(r)
	render(w, "New flow", "flow_edit", data)
}

func (s *Server) viewFlow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	spec, err := config.LoadFlow(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	repos, flows, skills, secrets, err := s.flowFormContext()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := specToEditData(spec)
	data.AllRepos, data.AllFlows, data.AllSkills, data.AllSecrets = repos, flows, skills, secrets
	data.PermissionModes = flow.PermissionModes
	data.Actor = actorFrom(r)
	renderWithWarning(w, "Flow: "+name, "flow_edit", data, pushWarning(r))
}

func (s *Server) saveFlow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	isNew := r.FormValue("is_new") == "true"

	spec := formToSpec(r)
	data, err := yaml.Marshal(spec)
	if err == nil {
		spec, err = flow.Parse(data)
	}
	if err != nil {
		s.rerenderFlowError(w, spec, isNew, actorVal, err)
		return
	}
	if err := config.SaveFlow(s.Home, spec, actorVal); err != nil {
		s.rerenderFlowError(w, spec, isNew, actorVal, err)
		return
	}
	redirectSaved(w, r, s.Home, "/flows/"+spec.Name)
}

func (s *Server) rerenderFlowError(w http.ResponseWriter, spec flow.Spec, isNew bool, actorVal string, saveErr error) {
	repos, flows, skills, secrets, err := s.flowFormContext()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := specToEditData(spec)
	data.IsNew = isNew
	data.AllRepos, data.AllFlows, data.AllSkills, data.AllSecrets = repos, flows, skills, secrets
	data.PermissionModes = flow.PermissionModes
	data.Actor = actorVal
	data.Error = saveErr.Error()
	render(w, "Flow error", "flow_edit", data)
}
