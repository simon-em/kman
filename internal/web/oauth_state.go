package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type oauthState struct {
	mu      sync.Mutex
	byState map[string]string
}

func newOAuthState() *oauthState {
	return &oauthState{byState: map[string]string{}}
}

func (s *oauthState) create(userID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := randomState()
	s.byState[state] = userID
	return state
}

func (s *oauthState) consume(state string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.byState[state]
	delete(s.byState, state)
	return userID, ok
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "fallback-state"
	}
	return hex.EncodeToString(b)
}
