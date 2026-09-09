package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
)

type skillRow struct {
	catalog.Entry
	Custom bool
}

type skillEditData struct {
	IsNew    bool
	Entry    catalog.StoredEntry
	FetchURL string
	Actor    string
	Error    string
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	entries, err := config.ListSkills(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	customNames, err := config.ListSkillNames(s.Home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	custom := toSet(customNames)
	rows := make([]skillRow, len(entries))
	for i, e := range entries {
		rows[i] = skillRow{Entry: e, Custom: custom[e.Name]}
	}
	render(w, "Skills", "skills_list", struct{ Rows []skillRow }{rows})
}

func (s *Server) newSkill(w http.ResponseWriter, r *http.Request) {
	render(w, "New skill", "skill_edit", skillEditData{
		IsNew: true,
		Entry: catalog.StoredEntry{Kind: catalog.KindDoc},
		Actor: actorFrom(r),
	})
}

func (s *Server) viewSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	e, err := config.LoadSkill(s.Home, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	renderWithWarning(w, "Skill: "+name, "skill_edit", skillEditData{Entry: e, Actor: actorFrom(r)}, pushWarning(r))
}

func skillFromForm(r *http.Request) catalog.StoredEntry {
	return catalog.StoredEntry{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: r.FormValue("description"),
		Kind:        r.FormValue("kind"),
		Path:        r.FormValue("path"),
		Command:     r.FormValue("command"),
		Text:        r.FormValue("text"),
		SourceURL:   strings.TrimSpace(r.FormValue("source_url")),
	}
}

func (s *Server) saveSkill(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorVal := r.FormValue("actor")
	setActorCookie(w, actorVal)
	isNew := r.FormValue("is_new") == "true"
	e := skillFromForm(r)

	if err := e.Validate(); err != nil {
		render(w, "Skill error", "skill_edit", skillEditData{IsNew: isNew, Entry: e, Actor: actorVal, Error: err.Error()})
		return
	}
	if err := config.SaveSkill(s.Home, e, actorVal); err != nil {
		render(w, "Skill error", "skill_edit", skillEditData{IsNew: isNew, Entry: e, Actor: actorVal, Error: err.Error()})
		return
	}
	redirectSaved(w, r, s.Home, "/skills/"+e.Name)
}

func (s *Server) fetchSkillPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	isNew := r.FormValue("is_new") == "true"
	e := skillFromForm(r)
	fetchURL := strings.TrimSpace(r.FormValue("fetch_url"))
	data := skillEditData{IsNew: isNew, Entry: e, FetchURL: fetchURL, Actor: r.FormValue("actor")}

	if fetchURL == "" {
		data.Error = "enter a URL to fetch first"
		render(w, "Skill", "skill_edit", data)
		return
	}
	text, err := fetchSkillContent(fetchURL)
	if err != nil {
		data.Error = fmt.Sprintf("fetching %s: %v", fetchURL, err)
		render(w, "Skill", "skill_edit", data)
		return
	}
	data.Entry.Text = text
	data.Entry.SourceURL = fetchURL
	render(w, "Skill (fetched — review before saving)", "skill_edit", data)
}
