package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/kranqpush"
	"github.com/simon-em/kman/internal/trigger"
)

func runPush(env Env, args []string) int {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	kranqURL := fs.String("kranq-url", os.Getenv("KMAN_KRANQ_URL"), "the kranq remote to push to (default: $KMAN_KRANQ_URL)")
	source := fs.String("source", ".", "a local git directory, or a remote URL to mirror first")
	branch := fs.String("branch", "", "branch name to report for this run")
	label := fs.String("label", "", "label for the VM and artifacts")
	keep := fs.String("keep-vm", "", "keep the job VM: never, on-failure, always")
	detach := fs.Bool("detach", false, "queue it and return without waiting for a result")
	asUser := fs.String("as", os.Getenv("KMAN_ACTOR"), "kman user id this push is on behalf of (default: $KMAN_ACTOR); needed for integration: credentials")
	positional, err := parsePermuted(fs, args)
	if err != nil {
		return exitcode.Usage
	}
	if len(positional) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman push <flow.yaml> [NAME=VALUE...] [flags]")
		fs.PrintDefaults()
		return exitcode.Usage
	}

	spec, code, err := loadFlow(positional[0])
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", positional[0], err)
		return code
	}

	provided, err := trigger.ParseAssignments(positional[1:])
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Usage
	}

	if *kranqURL == "" {
		fmt.Fprintln(env.Stderr, "kman: set --kranq-url (or KMAN_KRANQ_URL)")
		return exitcode.Usage
	}

	result, err := trigger.Run(context.Background(), kmanHome(), spec, provided, trigger.Options{
		KranqURL: *kranqURL,
		Source:   *source,
		Branch:   *branch,
		Label:    *label,
		Keep:     *keep,
		Detach:   *detach,
		AsUser:   *asUser,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return pushExitCode(err)
	}
	if *detach {
		fmt.Fprintln(env.Stdout, "queued")
		return exitcode.OK
	}
	fmt.Fprintf(env.Stdout, "id=%s status=%s exit=%d\n", result.ID, result.Status, result.ExitCode)
	if result.Status == kranqpush.StatusRefused {
		return result.ExitCode
	}
	return exitcode.FromTask(result.ExitCode)
}

func pushExitCode(err error) int {
	var te *trigger.Error
	if errors.As(err, &te) {
		switch te.Stage {
		case trigger.StageArgs, trigger.StageCredentials:
			return exitcode.InvalidSpec
		case trigger.StageSource:
			return exitcode.InternalError
		case trigger.StagePush:
			return exitcode.Unreachable
		}
	}
	return exitcode.InternalError
}

type boolFlag interface {
	IsBoolFlag() bool
}

func parsePermuted(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		if strings.Contains(a, "=") {
			continue
		}
		f := fs.Lookup(strings.TrimLeft(a, "-"))
		if f == nil {
			continue
		}
		if b, ok := f.Value.(boolFlag); ok && b.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return positional, nil
}
