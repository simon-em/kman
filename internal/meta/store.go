package meta

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DefaultTokenTTL = 2 * time.Hour

var ErrInvalid = errors.New("meta token: invalid or expired")

var ErrForbidden = errors.New("meta token: not granted this capability")

type Token struct {
	Value     string    `json:"value"`
	FlowName  string    `json:"flow_name"`
	UserID    string    `json:"user_id"`
	Grants    []string  `json:"grants"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	path string
}

func Open(home string) *Store {
	return &Store{path: filepath.Join(home, "meta-tokens.json")}
}

func (s *Store) Mint(flowName, userID string, grants []string, ttl time.Duration) (Token, error) {
	tok := Token{
		Value:     randomValue(),
		FlowName:  flowName,
		UserID:    userID,
		Grants:    append([]string{}, grants...),
		ExpiresAt: time.Now().Add(ttl),
	}
	tokens, err := s.load()
	if err != nil {
		return Token{}, err
	}
	tokens[tok.Value] = tok
	if err := s.save(prune(tokens)); err != nil {
		return Token{}, err
	}
	return tok, nil
}

func (s *Store) Validate(value, capability string) (Token, error) {
	tokens, err := s.load()
	if err != nil {
		return Token{}, err
	}
	tok, ok := tokens[value]
	if !ok || time.Now().After(tok.ExpiresAt) {
		return Token{}, ErrInvalid
	}
	if !containsString(tok.Grants, capability) {
		return Token{}, ErrForbidden
	}
	return tok, nil
}

func (s *Store) load() (map[string]Token, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]Token{}, nil
	}
	if err != nil {
		return nil, err
	}
	var tokens map[string]Token
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	return tokens, nil
}

func (s *Store) save(tokens map[string]Token) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func prune(tokens map[string]Token) map[string]Token {
	now := time.Now()
	for k, v := range tokens {
		if now.After(v.ExpiresAt) {
			delete(tokens, k)
		}
	}
	return tokens
}

func randomValue() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func containsString(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
