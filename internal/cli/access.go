package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/exitcode"
)

func actor() string {
	return os.Getenv("KMAN_ACTOR")
}

func runUser(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman user set|ls ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return runUserSet(env, rest)
	case "ls":
		return runUserList(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown user subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runUserSet(env Env, args []string) int {
	fs := flag.NewFlagSet("user set", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	displayName := fs.String("display-name", "", "the user's display name")
	slackID := fs.String("slack-id", "", "the user's Slack user ID")
	positional, err := parsePermuted(fs, args)
	if err != nil {
		return exitcode.Usage
	}
	if len(positional) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman user set <id> [--display-name X] [--slack-id Y]")
		return exitcode.Usage
	}
	id := positional[0]

	u, err := config.LoadUser(kmanHome(), id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	u.ID = id
	if *displayName != "" {
		u.DisplayName = *displayName
	}
	if *slackID != "" {
		u.SlackUserID = *slackID
	}
	if err := config.SaveUser(kmanHome(), u, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: set\n", id)
	return exitcode.OK
}

func runUserList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman user ls")
		return exitcode.Usage
	}
	names, err := config.ListUserIDs(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, n := range names {
		fmt.Fprintln(env.Stdout, n)
	}
	return exitcode.OK
}

func runGroup(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman group set|ls|add-member|remove-member ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return runGroupSet(env, rest)
	case "ls":
		return runGroupList(env, rest)
	case "add-member":
		return runGroupMember(env, rest, true)
	case "remove-member":
		return runGroupMember(env, rest, false)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown group subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runGroupSet(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman group set <name>")
		return exitcode.Usage
	}
	name := args[0]
	g, err := config.LoadGroup(kmanHome(), name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	g.Name = name
	if err := config.SaveGroup(kmanHome(), g, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: set\n", name)
	return exitcode.OK
}

func runGroupList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman group ls")
		return exitcode.Usage
	}
	names, err := config.ListGroupNames(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, n := range names {
		fmt.Fprintln(env.Stdout, n)
	}
	return exitcode.OK
}

func runGroupMember(env Env, args []string, add bool) int {
	if len(args) != 2 {
		fmt.Fprintln(env.Stderr, "usage: kman group add-member|remove-member <group> <user-id>")
		return exitcode.Usage
	}
	groupName, userID := args[0], args[1]
	g, err := config.LoadGroup(kmanHome(), groupName)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: no such group %q; create one first with `kman group set`\n", groupName)
		return exitcode.InvalidSpec
	}
	if add {
		if _, err := config.LoadUser(kmanHome(), userID); err != nil {
			fmt.Fprintf(env.Stderr, "kman: no such user %q; create one first with `kman user set`\n", userID)
			return exitcode.InvalidSpec
		}
	}
	g.Members = updateGrantList(g.Members, userID, add)
	if err := config.SaveGroup(kmanHome(), g, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	verb := "added to"
	if !add {
		verb = "removed from"
	}
	fmt.Fprintf(env.Stdout, "%s %s %s\n", userID, verb, groupName)
	return exitcode.OK
}

func runGrant(env Env, args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(env.Stderr, "usage: kman grant <user/ID|group/NAME> <flow>")
		return exitcode.Usage
	}
	return applyGrant(env, args[0], args[1], true)
}

func runRevoke(env Env, args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(env.Stderr, "usage: kman revoke <user/ID|group/NAME> <flow>")
		return exitcode.Usage
	}
	return applyGrant(env, args[0], args[1], false)
}

func applyGrant(env Env, principal, flowName string, grant bool) int {
	kind, name, err := parsePrincipal(principal)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Usage
	}

	if _, err := config.LoadFlow(kmanHome(), flowName); err != nil {
		fmt.Fprintf(env.Stderr, "kman: no such flow %q\n", flowName)
		return exitcode.InvalidSpec
	}

	switch kind {
	case "user":
		u, err := config.LoadUser(kmanHome(), name)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: no such user %q; create one first with `kman user set`\n", name)
			return exitcode.InvalidSpec
		}
		u.Flows = updateGrantList(u.Flows, flowName, grant)
		if err := config.SaveUser(kmanHome(), u, actor()); err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
			return exitcode.InternalError
		}
	case "group":
		g, err := config.LoadGroup(kmanHome(), name)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: no such group %q; create one first with `kman group set`\n", name)
			return exitcode.InvalidSpec
		}
		g.Flows = updateGrantList(g.Flows, flowName, grant)
		if err := config.SaveGroup(kmanHome(), g, actor()); err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
			return exitcode.InternalError
		}
	}

	pushConfigOrWarn(env, kmanHome())
	verb := "granted"
	if !grant {
		verb = "revoked"
	}
	fmt.Fprintf(env.Stdout, "%s: %s %s\n", principal, verb, flowName)
	return exitcode.OK
}

func parsePrincipal(s string) (kind, name string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || (parts[0] != "user" && parts[0] != "group") || parts[1] == "" {
		return "", "", fmt.Errorf("%q is not user/<id> or group/<name>", s)
	}
	return parts[0], parts[1], nil
}

func updateGrantList(list []string, name string, add bool) []string {
	idx := -1
	for i, v := range list {
		if v == name {
			idx = i
			break
		}
	}
	if add {
		if idx == -1 {
			return append(list, name)
		}
		return list
	}
	if idx == -1 {
		return list
	}
	return append(list[:idx], list[idx+1:]...)
}
