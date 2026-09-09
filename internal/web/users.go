package web

import (
	"net/http"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/config"
	"gopkg.in/yaml.v3"
)

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
	renderWithWarning(w, "User: "+id, "user_edit", userEditData{User: u, AllFlows: flows, Granted: toSet(u.Flows), Actor: actorFrom(r)}, pushWarning(r))
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
	redirectSaved(w, r, s.Home, "/users/"+u.ID)
}
