package integration

import (
	"context"
	"time"
)

type Integration interface {
	Name() string
}

type Credential struct {
	Value     string
	ExpiresAt time.Time
}

type CredentialProvider interface {
	Credential(ctx context.Context, userID string) (Credential, error)
}

type TriggerChannel interface {
	Name() string
}
