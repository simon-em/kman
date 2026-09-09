package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/gitcache"
)

func runMirror(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman mirror <remote-url>")
		return exitcode.Usage
	}
	dir, err := gitcache.Sync(context.Background(), kmanHome(), args[0], "")
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	fmt.Fprintln(env.Stdout, dir)
	return exitcode.OK
}

func kmanHome() string {
	if h := os.Getenv("KMAN_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kman")
}
