package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/cron"
	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/trigger"
)

func runCron(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman cron set|ls|rm|tick|serve ...")
		return exitcode.Usage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return runCronSet(env, rest)
	case "ls":
		return runCronList(env, rest)
	case "rm":
		return runCronRemove(env, rest)
	case "tick":
		return runCronTick(env, rest)
	case "serve":
		return runCronServe(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "kman: unknown cron subcommand %q\n", sub)
		return exitcode.Usage
	}
}

func runCronSet(env Env, args []string) int {
	fs := flag.NewFlagSet("cron set", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	flowName := fs.String("flow", "", "the flow this schedule fires")
	schedule := fs.String("schedule", "", `a 5-field cron expression ("minute hour dom month dow")`)
	positional, err := parsePermuted(fs, args)
	if err != nil {
		return exitcode.Usage
	}
	if len(positional) < 1 {
		fmt.Fprintln(env.Stderr, `usage: kman cron set <name> --flow <flow> --schedule "<expr>" [NAME=VALUE...]`)
		return exitcode.Usage
	}
	name := positional[0]
	if *flowName == "" || *schedule == "" {
		fmt.Fprintln(env.Stderr, "kman: --flow and --schedule are required")
		return exitcode.Usage
	}
	if _, err := config.LoadFlow(kmanHome(), *flowName); err != nil {
		fmt.Fprintf(env.Stderr, "kman: no such flow %q\n", *flowName)
		return exitcode.InvalidSpec
	}
	entryArgs, err := trigger.ParseAssignments(positional[1:])
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Usage
	}
	entry := cron.Entry{Name: name, Flow: *flowName, Schedule: *schedule, Args: entryArgs, CreatedBy: actor()}
	if err := entry.Validate(); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	if err := config.SaveCronEntry(kmanHome(), entry, actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: set\n", name)
	return exitcode.OK
}

func runCronList(env Env, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: kman cron ls")
		return exitcode.Usage
	}
	names, err := config.ListCronNames(kmanHome())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	for _, n := range names {
		e, err := config.LoadCronEntry(kmanHome(), n)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
			return exitcode.InternalError
		}
		fmt.Fprintf(env.Stdout, "%s: flow=%s schedule=%q created_by=%s\n", e.Name, e.Flow, e.Schedule, e.CreatedBy)
	}
	return exitcode.OK
}

func runCronRemove(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman cron rm <name>")
		return exitcode.Usage
	}
	if err := config.RemoveCronEntry(kmanHome(), args[0], actor()); err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	pushConfigOrWarn(env, kmanHome())
	fmt.Fprintf(env.Stdout, "%s: removed\n", args[0])
	return exitcode.OK
}

func runCronTick(env Env, args []string) int {
	fs := flag.NewFlagSet("cron tick", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	kranqURL := fs.String("kranq-url", os.Getenv("KMAN_KRANQ_URL"), "the kranq remote to push to (default: $KMAN_KRANQ_URL)")
	if err := fs.Parse(args); err != nil {
		return exitcode.Usage
	}
	if *kranqURL == "" {
		fmt.Fprintln(env.Stderr, "kman: set --kranq-url (or KMAN_KRANQ_URL)")
		return exitcode.Usage
	}
	fired, err := tickCron(context.Background(), env, kmanHome(), *kranqURL, time.Now())
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	fmt.Fprintf(env.Stdout, "fired %d entr%s\n", fired, plural(fired))
	return exitcode.OK
}

func runCronServe(env Env, args []string) int {
	fs := flag.NewFlagSet("cron serve", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	kranqURL := fs.String("kranq-url", os.Getenv("KMAN_KRANQ_URL"), "the kranq remote to push to (default: $KMAN_KRANQ_URL)")
	interval := fs.Duration("interval", time.Minute, "how often to check for due schedule entries")
	if err := fs.Parse(args); err != nil {
		return exitcode.Usage
	}
	if *kranqURL == "" {
		fmt.Fprintln(env.Stderr, "kman: set --kranq-url (or KMAN_KRANQ_URL)")
		return exitcode.Usage
	}
	fmt.Fprintf(env.Stdout, "kman cron serve ticking every %s\n", *interval)
	for {
		if _, err := tickCron(context.Background(), env, kmanHome(), *kranqURL, time.Now()); err != nil {
			fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		}
		time.Sleep(*interval)
	}
}

func tickCron(ctx context.Context, env Env, home, kranqURL string, now time.Time) (int, error) {
	cfg, err := config.Load(home, "")
	if err != nil {
		return 0, err
	}
	flowsByName := map[string]flow.Spec{}
	for _, f := range cfg.Flows {
		flowsByName[f.Name] = f
	}

	fired := 0
	for _, entry := range cfg.Cron {
		schedule, err := cron.ParseSchedule(entry.Schedule)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kman: cron %s: %v\n", entry.Name, err)
			continue
		}
		if !schedule.Matches(now) {
			continue
		}
		spec, ok := flowsByName[entry.Flow]
		if !ok {
			fmt.Fprintf(env.Stderr, "kman: cron %s: flow %q no longer exists\n", entry.Name, entry.Flow)
			continue
		}
		if !trigger.LooksLikeRemote(spec.Repo) {
			fmt.Fprintf(env.Stderr, "kman: cron %s: flow %q has no repo; set repo: to a remote URL for cron-triggered runs\n", entry.Name, entry.Flow)
			continue
		}

		fired++
		fmt.Fprintf(env.Stderr, "kman: cron %s: firing %s\n", entry.Name, entry.Flow)
		if _, err := trigger.Run(ctx, home, spec, entry.Args, trigger.Options{
			KranqURL: kranqURL,
			Source:   spec.Repo,
			AsUser:   entry.CreatedBy,
			Detach:   true,
		}); err != nil {
			fmt.Fprintf(env.Stderr, "kman: cron %s: %v\n", entry.Name, err)
		}
	}
	return fired, nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
