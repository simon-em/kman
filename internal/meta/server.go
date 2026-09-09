package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/cron"
)

type Server struct {
	Home  string
	Store *Store
}

func NewServer(home string) *Server {
	return &Server{Home: home, Store: Open(home)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /meta/cron", s.createCron)
	return mux
}

type createCronRequest struct {
	Name     string            `json:"name"`
	Schedule string            `json:"schedule"`
	Args     map[string]string `json:"args"`
}

func (s *Server) createCron(w http.ResponseWriter, r *http.Request) {
	tok, err := s.authenticate(r, cron.CreateMeta)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var body createCronRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry := cron.Entry{
		Name:      body.Name,
		Flow:      tok.FlowName,
		Schedule:  body.Schedule,
		Args:      body.Args,
		CreatedBy: tok.UserID,
	}
	if err := entry.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := config.SaveCronEntry(s.Home, entry, tok.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"name": entry.Name})
}

func (s *Server) authenticate(r *http.Request, capability string) (Token, error) {
	value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || value == "" {
		return Token{}, errors.New("missing bearer token")
	}
	return s.Store.Validate(value, capability)
}
