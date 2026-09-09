package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/exitcode"
)

func runSkills(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman skills ls|set|rm ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		return runSkillsList(env, rest)
	case "set":
		return runSkillsSet(env, rest)
	case "rm":
		return runSkillsRemove(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown skills subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runSkillsList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman skills ls")
		return exitcode.Usage
	}
	entries, err := config.ListSkills(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, e := range entries {
		fmt.Fprintf(env.Stdout, "catalog:%s@%s  %s\n", e.Name, e.Ref, e.Description)
	}
	return exitcode.OK
}

func runSkillsSet(env Env, args []string) int {
	fs := flag.NewFlagSet("skills set", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	kind := fs.String("kind", catalog.KindDoc, "doc (a markdown skill) or mcp (an executable MCP server)")
	path := fs.String("path", "", "path the content is staged at inside the VM, e.g. .claude/skills/x/SKILL.md")
	command := fs.String("command", "", "interpreter for an mcp entry, e.g. python3 (leave blank for a self-executing shebang script)")
	description := fs.String("description", "", "shown in kman skills ls and the flow editor")
	sourceURL := fs.String("source-url", "", "where this was fetched from, for provenance; purely informational")
	file := fs.String("file", "", "read content from this local file instead of stdin")
	positional, err := parsePermuted(fs, args)
	if err != nil {
		return exitcode.Usage
	}
	if len(positional) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman skills set <name> --kind doc|mcp --path <path> [--command <cmd>] [--description <desc>] [--source-url <url>] [--file <path>]   (content read from stdin if --file is omitted)")
		return exitcode.Usage
	}
	name := positional[0]

	var content []byte
	if *file != "" {
		content, err = os.ReadFile(*file)
	} else {
		content, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}

	e := catalog.StoredEntry{
		Name:        name,
		Description: *description,
		Kind:        *kind,
		Path:        *path,
		Command:     *command,
		Text:        string(content),
		SourceURL:   *sourceURL,
	}
	if err := e.Validate(); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	if err := config.SaveSkill(kmanHome(), e, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: set\n", name)
	return exitcode.OK
}

func runSkillsRemove(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman skills rm <name>")
		return exitcode.Usage
	}
	name := args[0]
	if err := config.RemoveSkill(kmanHome(), name, actor()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(env.Stderr, "kman: no such skill %q\n", name)
			return exitcode.InvalidSpec
		}
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: removed\n", name)
	return exitcode.OK
}
