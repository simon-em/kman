package cli

import (
	"context"
	"fmt"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/integration/bitbucket"
)

func resolveIntegrationCredential(name, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("integration:%s needs an acting user; pass --as or set KMAN_ACTOR", name)
	}
	switch name {
	case "bitbucket":
		provider, err := bitbucket.FromVault(kmanHome())
		if err != nil {
			return "", err
		}
		cred, err := provider.Credential(context.Background(), userID)
		if err != nil {
			return "", err
		}
		return cred.Value, nil
	default:
		return "", fmt.Errorf("unknown integration %q", name)
	}
}

func runIntegration(env Env, args []string) int {
	if len(args) < 2 || args[0] != "bitbucket" || args[1] != "status" {
		fmt.Fprintln(env.Stderr, "usage: kman integration bitbucket status [user-id...]")
		return exitcode.Usage
	}
	userIDs := args[2:]
	if len(userIDs) == 0 {
		var err error
		userIDs, err = config.ListUserIDs(kmanHome())
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
			return exitcode.InternalError
		}
	}

	provider, err := bitbucket.FromVault(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Misconfigured
	}
	for _, id := range userIDs {
		status := "not connected"
		if provider.Connected(id) {
			status = "connected"
		}
		fmt.Fprintf(env.Stdout, "%s: %s\n", id, status)
	}
	return exitcode.OK
}
