package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/cron"
	"github.com/simon-em/kman/internal/trigger"
)

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
	renderWithWarning(w, "Cron: "+name, "cron_edit", cronEditData{Entry: e, AllFlows: flows, ArgsText: argsToText(e.Args), Actor: actorFrom(r)}, pushWarning(r))
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
	redirectSaved(w, r, s.Home, "/cron/"+e.Name)
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
