package cli

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/meta"
)

func runMeta(env Env, args []string) int {
	if len(args) < 1 || args[0] != "serve" {
		fmt.Fprintln(env.Stderr, "usage: kman meta serve [--addr HOST:PORT]")
		return exitcode.Usage
	}
	return runMetaServe(env, args[1:])
}

func runMetaServe(env Env, args []string) int {
	fs := flag.NewFlagSet("meta serve", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	addr := fs.String("addr", "0.0.0.0:8082", "address to listen on; this endpoint is meant to be reachable from a kranq VM")
	if err := fs.Parse(args); err != nil {
		return exitcode.Usage
	}
	fmt.Fprintf(env.Stdout, "kman meta serve listening on http://%s/meta/cron\n", *addr)
	if err := http.ListenAndServe(*addr, meta.NewServer(kmanHome()).Handler()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	return exitcode.OK
}
