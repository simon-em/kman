package web

import (
	"net/http"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/config"
	"gopkg.in/yaml.v3"
)

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
	renderWithWarning(w, "Group: "+name, "group_edit", groupEditData{
		Group: g, AllUsers: users, AllFlows: flows,
		Members: toSet(g.Members), Granted: toSet(g.Flows), Actor: actorFrom(r),
	}, pushWarning(r))
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
	redirectSaved(w, r, s.Home, "/groups/"+g.Name)
}
