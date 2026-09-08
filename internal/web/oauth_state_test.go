package web

import "testing"

func TestOAuthStateRoundTrip(t *testing.T) {
	s := newOAuthState()
	state := s.create("simon")
	userID, ok := s.consume(state)
	if !ok || userID != "simon" {
		t.Errorf("consume = %q, %v", userID, ok)
	}
}

func TestOAuthStateIsConsumedOnce(t *testing.T) {
	s := newOAuthState()
	state := s.create("simon")
	s.consume(state)
	if _, ok := s.consume(state); ok {
		t.Error("expected the state to be gone after the first consume")
	}
}

func TestOAuthStateUnknownFails(t *testing.T) {
	s := newOAuthState()
	if _, ok := s.consume("never-issued"); ok {
		t.Error("expected an unknown state to fail")
	}
}
