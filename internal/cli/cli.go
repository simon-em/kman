package cli

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/simon-em/kman/internal/exitcode"
)

var Version = "dev"

type Env struct {
	Stdout io.Writer
	Stderr io.Writer
}

type Command struct {
	Usage   string
	Summary string
	Run     func(env Env, args []string) int
}

var commands map[string]Command

func init() {
	commands = map[string]Command{
		"version":     {"version [--json]", "print the kman version", runVersion},
		"validate":    {"validate <flow.yaml>...", "check that a flow parses", runValidate},
		"render":      {"render <flow.yaml>", "print the kranq task-spec YAML a flow projects to", runRender},
		"push":        {"push <flow.yaml> [NAME=VALUE...] [flags]", "compile a flow and push it to kranq", runPush},
		"mirror":      {"mirror <remote-url>", "sync a host-side cache mirror of a remote repo", runMirror},
		"secret":      {"secret set|ls|rm ...", "manage kman's encrypted credential store", runSecret},
		"user":        {"user set|ls ...", "manage kman users", runUser},
		"group":       {"group set|ls|add-member|remove-member ...", "manage kman groups", runGroup},
		"repo":        {"repo set|ls|rm ...", "manage named source repos a flow or dispatch can target", runRepo},
		"grant":       {"grant <user/ID|group/NAME> <flow>", "let a user or group trigger a flow", runGrant},
		"revoke":      {"revoke <user/ID|group/NAME> <flow>", "undo a grant", runRevoke},
		"web":         {"web [--addr HOST:PORT]", "serve the admin UI over the config repo", runWeb},
		"integration": {"integration bitbucket|slack status ...", "check integration connection status", runIntegration},
		"slack":       {"slack serve [--addr HOST:PORT] --kranq-url <url>", "serve the Slack events endpoint (separate from the admin UI)", runSlack},
		"meta":        {"meta serve [--addr HOST:PORT]", "serve the scoped meta endpoint a pushed VM calls back through", runMeta},
		"cron":        {"cron set|ls|rm|tick|serve ...", "manage and fire scheduled flow pushes", runCron},
		"skills":      {"skills ls", "list kman's built-in MCP/skill catalog", runSkills},
		"help":        {"help [command]", "show usage", runHelp},
	}
}

func Main(argv []string) int {
	return dispatch(Env{Stdout: os.Stdout, Stderr: os.Stderr}, argv[1:])
}

func dispatch(env Env, args []string) int {
	if len(args) == 0 {
		usage(env.Stderr)
		return exitcode.Usage
	}
	name := args[0]
	if name == "-h" || name == "--help" {
		usage(env.Stdout)
		return exitcode.OK
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(env.Stderr, "kman: unknown command %q\n\n", name)
		usage(env.Stderr)
		return exitcode.Usage
	}
	return cmd.Run(env, args[1:])
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "kman %s\n\nusage: kman <command> [arguments]\n\n", Version)
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	list(w, names)
}

func list(w io.Writer, names []string) {
	width := 0
	for _, name := range names {
		if n := len(commands[name].Usage); n > width {
			width = n
		}
	}
	for _, name := range names {
		fmt.Fprintf(w, "  %-*s  %s\n", width, commands[name].Usage, commands[name].Summary)
	}
}

func runHelp(env Env, args []string) int {
	if len(args) == 0 {
		usage(env.Stdout)
		return exitcode.OK
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(env.Stderr, "kman: unknown command %q\n", args[0])
		return exitcode.Usage
	}
	fmt.Fprintf(env.Stdout, "usage: kman %s\n\n%s\n", cmd.Usage, cmd.Summary)
	return exitcode.OK
}

func runVersion(env Env, args []string) int {
	if len(args) > 0 && args[0] == "--json" {
		fmt.Fprintf(env.Stdout, "{\"version\":%q}\n", Version)
		return exitcode.OK
	}
	fmt.Fprintln(env.Stdout, Version)
	return exitcode.OK
}

func readFlowFile(path string) ([]byte, int, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, exitcode.OK, nil
	}
	if os.IsNotExist(err) {
		return nil, exitcode.NoSuchFile, err
	}
	return nil, exitcode.InternalError, err
}
