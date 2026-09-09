package web

import (
	"net/http"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/reporegistry"
)

type repoEditData struct {
	IsNew bool
	Repo  reporegistry.Repo
	Actor string
	Error string
}

func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := config.ListRepos(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, "Repos", "repos_list", struct{ Repos []reporegistry.Repo }{repos})
}

func (s *Server) newRepo(w http.ResponseWriter, r *http.Request) {
	render(w, "New repo", "repo_edit", repoEditData{IsNew: true, Actor: actorFrom(r)})
}

func (s *Server) viewRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	repo, err := config.LoadRepo(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	renderWithWarning(w, "Repo: "+name, "repo_edit", repoEditData{Repo: repo, Actor: actorFrom(r)}, pushWarning(r))
}

func (s *Server) saveRepo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	isNew := r.FormValue("is_new") == "true"
	repo := reporegistry.Repo{
		Name: r.FormValue("name"),
		URL:  r.FormValue("url"),
	}
	if err := repo.Validate(); err != nil {
		render(w, "Repo error", "repo_edit", repoEditData{IsNew: isNew, Repo: repo, Actor: actorVal, Error: err.Error()})
		return
	}
	if err := config.SaveRepo(s.Home, repo, actorVal); err != nil {
		render(w, "Repo error", "repo_edit", repoEditData{IsNew: isNew, Repo: repo, Actor: actorVal, Error: err.Error()})
		return
	}
	redirectSaved(w, r, s.Home, "/repos/"+repo.Name)
}
