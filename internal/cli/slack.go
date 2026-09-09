package cli

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/integration/slack"
)

func runSlack(env Env, args []string) int {
	if len(args) < 1 || args[0] != "serve" {
		fmt.Fprintln(env.Stderr, "usage: kman slack serve [--addr HOST:PORT] --kranq-url <url>")
		return exitcode.Usage
	}
	return runSlackServe(env, args[1:])
}

func runSlackServe(env Env, args []string) int {
	fs := flag.NewFlagSet("slack serve", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	addr := fs.String("addr", "0.0.0.0:8081", "address to listen on; this endpoint is meant to be reachable from Slack")
	kranqURL := fs.String("kranq-url", os.Getenv("KMAN_KRANQ_URL"), "the kranq remote to push to (default: $KMAN_KRANQ_URL)")
	if err := fs.Parse(args); err != nil {
		return exitcode.Usage
	}
	if *kranqURL == "" {
		fmt.Fprintln(env.Stderr, "kman: set --kranq-url (or KMAN_KRANQ_URL)")
		return exitcode.Usage
	}
	if _, err := slack.FromVault(kmanHome()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Misconfigured
	}

	mux := http.NewServeMux()
	mux.Handle("POST /slack/events", slack.NewHandler(kmanHome(), *kranqURL))
	fmt.Fprintf(env.Stdout, "kman slack serve listening on http://%s/slack/events\n", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	return exitcode.OK
}
