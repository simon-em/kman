package cli

import (
	"fmt"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/integration/slack"
)

func runIntegration(env Env, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(env.Stderr, "usage: kman integration bitbucket status [user-id...]\n       kman integration slack status")
		return exitcode.Usage
	}
	switch args[0] {
	case "bitbucket":
		return runBitbucketStatus(env, args[1:])
	case "slack":
		return runSlackStatus(env, args[1:])
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown integration %q\n", args[0])
		return exitcode.Usage
	}
}

func runBitbucketStatus(env Env, args []string) int {
	if len(args) < 1 || args[0] != "status" {
		fmt.Fprintln(env.Stderr, "usage: kman integration bitbucket status [user-id...]")
		return exitcode.Usage
	}
	userIDs := args[1:]
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

func runSlackStatus(env Env, args []string) int {
	if len(args) != 1 || args[0] != "status" {
		fmt.Fprintln(env.Stderr, "usage: kman integration slack status")
		return exitcode.Usage
	}
	if _, err := slack.FromVault(kmanHome()); err != nil {
		fmt.Fprintf(env.Stdout, "not configured: %v\n", err)
		return exitcode.OK
	}
	fmt.Fprintln(env.Stdout, "configured")
	return exitcode.OK
}
