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
	metaURL := fs.String("meta-url", os.Getenv("KMAN_META_URL"), "the kman meta endpoint the VM can reach (default: $KMAN_META_URL); needed by a flow with access.meta")
	defaultFlowEnv := os.Getenv("KMAN_DEFAULT_FLOW")
	if defaultFlowEnv == "" {
		defaultFlowEnv = "create-feature"
	}
	defaultFlow := fs.String("default-flow", defaultFlowEnv, "flow to run for a mention that isn't \"run <flow>\", the mention text becomes its TASK argument (empty disables this; ignored if --dispatch is set)")
	dispatchFlag := fs.Bool("dispatch", os.Getenv("KMAN_DISPATCH") == "true", "route a free-form mention to a flow and repo picked from it, via a claude -p classification call, instead of a fixed --default-flow (default: $KMAN_DISPATCH)")
	dispatchModel := fs.String("dispatch-model", os.Getenv("KMAN_DISPATCH_MODEL"), "model for the dispatch classification call (default: $KMAN_DISPATCH_MODEL, or dispatch's own default)")
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

	handler := slack.NewHandler(kmanHome(), *kranqURL, *metaURL, *defaultFlow)
	handler.Dispatch = *dispatchFlag
	handler.DispatchModel = *dispatchModel

	mux := http.NewServeMux()
	mux.Handle("POST /slack/events", handler)
	fmt.Fprintf(env.Stdout, "kman slack serve listening on http://%s/slack/events\n", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	return exitcode.OK
}
