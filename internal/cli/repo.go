package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/reporegistry"
)

func runRepo(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman repo set|ls|rm ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return runRepoSet(env, rest)
	case "ls":
		return runRepoList(env, rest)
	case "rm":
		return runRepoRemove(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown repo subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runRepoSet(env Env, args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(env.Stderr, "usage: kman repo set <name> <url>")
		return exitcode.Usage
	}
	name, url := args[0], args[1]
	r := reporegistry.Repo{Name: name, URL: url}
	if err := r.Validate(); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	if err := config.SaveRepo(kmanHome(), r, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	fmt.Fprintf(env.Stdout, "%s: set\n", name)
	return exitcode.OK
}

func runRepoList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman repo ls")
		return exitcode.Usage
	}
	repos, err := config.ListRepos(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, r := range repos {
		fmt.Fprintf(env.Stdout, "%s: url=%s\n", r.Name, r.URL)
	}
	return exitcode.OK
}

func runRepoRemove(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman repo rm <name>")
		return exitcode.Usage
	}
	name := args[0]
	if err := config.RemoveRepo(kmanHome(), name, actor()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(env.Stderr, "kman: no such repo %q\n", name)
			return exitcode.InvalidSpec
		}
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	fmt.Fprintf(env.Stdout, "%s: removed\n", name)
	return exitcode.OK
}
