package cli

import (
	"fmt"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/exitcode"
)

func runSkills(env Env, args []string) int {
	if len(args) != 1 || args[0] != "ls" {
		fmt.Fprintln(env.Stderr, "usage: kman skills ls")
		return exitcode.Usage
	}
	for _, e := range catalog.List() {
		fmt.Fprintf(env.Stdout, "catalog:%s@%s  %s\n", e.Name, e.Ref, e.Description)
	}
	return exitcode.OK
}
