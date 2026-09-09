package meta

import (
	"errors"
	"testing"
	"time"
)

func TestMintThenValidate(t *testing.T) {
	home := t.TempDir()
	s := Open(home)
	tok, err := s.Mint("deploy-review", "simon", []string{"cron.create"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Validate(tok.Value, "cron.create")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.FlowName != "deploy-review" || got.UserID != "simon" {
		t.Errorf("got = %+v", got)
	}
}

func TestValidateRejectsAnUnknownToken(t *testing.T) {
	s := Open(t.TempDir())
	if _, err := s.Validate("does-not-exist", "cron.create"); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestValidateRejectsAnExpiredToken(t *testing.T) {
	home := t.TempDir()
	s := Open(home)
	tok, err := s.Mint("deploy-review", "simon", []string{"cron.create"}, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(tok.Value, "cron.create"); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestValidateRejectsAnUngrantedCapability(t *testing.T) {
	home := t.TempDir()
	s := Open(home)
	tok, err := s.Mint("deploy-review", "simon", []string{"cron.create"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(tok.Value, "something.else"); !errors.Is(err, ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestMintPersistsAcrossStoreInstances(t *testing.T) {
	home := t.TempDir()
	tok, err := Open(home).Mint("deploy-review", "simon", []string{"cron.create"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(home).Validate(tok.Value, "cron.create"); err != nil {
		t.Fatalf("Validate from a fresh Store: %v", err)
	}
}

func TestExpiredTokensArePrunedOnMint(t *testing.T) {
	home := t.TempDir()
	s := Open(home)
	expired, err := s.Mint("a", "simon", []string{"cron.create"}, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mint("b", "simon", []string{"cron.create"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(expired.Value, "cron.create"); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid for a pruned token", err)
	}
}
