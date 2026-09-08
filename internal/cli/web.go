package cli

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/web"
)

func runWeb(env Env, args []string) int {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	addr := fs.String("addr", "127.0.0.1:8080", "address to listen on (loopback by default: this UI has no login yet)")
	if err := fs.Parse(args); err != nil {
		return exitcode.Usage
	}
	fmt.Fprintf(env.Stdout, "kman web listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, web.New(kmanHome()).Handler()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	return exitcode.OK
}
