package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/vault"
)

func runSecret(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman secret set|ls|rm ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return runSecretSet(env, rest)
	case "ls":
		return runSecretList(env, rest)
	case "rm":
		return runSecretRemove(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown secret subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runSecretSet(env Env, args []string) int {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(env.Stderr, "usage: kman secret set <name> [value]   (value read from stdin if omitted)")
		return exitcode.Usage
	}
	name := args[0]
	value := ""
	if len(args) == 2 {
		value = args[1]
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
			return exitcode.InternalError
		}
		value = strings.TrimRight(string(data), "\n")
	}

	v, err := vault.Open(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	if err := v.Set(name, value); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	fmt.Fprintf(env.Stdout, "%s: set\n", name)
	return exitcode.OK
}

func runSecretList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman secret ls")
		return exitcode.Usage
	}
	v, err := vault.Open(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	names, err := v.List()
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, name := range names {
		fmt.Fprintln(env.Stdout, name)
	}
	return exitcode.OK
}

func runSecretRemove(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman secret rm <name>")
		return exitcode.Usage
	}
	v, err := vault.Open(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	if err := v.Remove(args[0]); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	fmt.Fprintf(env.Stdout, "%s: removed\n", args[0])
	return exitcode.OK
}
