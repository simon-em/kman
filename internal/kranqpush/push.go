package kranqpush

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"sort"
	"time"
)

const TaskRefPrefix = "refs/heads/task/"

const MirrorRefPrefix = "refs/heads/mirror/"

type Options struct {
	KranqURL string
	TaskFile string
	Repo     string
	Branch   string
	Label    string
	Keep     string
	Env      map[string]string
	Detach   bool
}

func Push(ctx context.Context, sourceDir, rev string, opts Options) (Result, error) {
	if opts.KranqURL == "" {
		return Result{}, fmt.Errorf("no kranq URL given")
	}
	if opts.TaskFile == "" {
		return Result{}, fmt.Errorf("no task file given")
	}
	ref := taskRef()
	args := []string{"--git-dir=" + sourceDir, "push", "--force", opts.KranqURL, rev + ":" + ref}
	args = append(args, pushOptionArgs(opts)...)

	cmd := exec.CommandContext(ctx, "git", args...)
	out, runErr := cmd.CombinedOutput()

	result := ParseResult(string(out))
	if result.Found {
		return result, nil
	}
	if runErr != nil {
		return Result{}, fmt.Errorf("git push: %w: %s", runErr, out)
	}
	if opts.Detach {
		return Result{}, nil
	}
	return Result{}, fmt.Errorf("the push was accepted but no result came back; the run may still be going")
}

func UpdateMirror(ctx context.Context, sourceDir, rev, kranqURL, slug string) error {
	ref := MirrorRefPrefix + slug
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+sourceDir, "push", kranqURL, rev+":"+ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating %s: %w: %s", ref, err, out)
	}
	return nil
}

func pushOptionArgs(opts Options) []string {
	var args []string
	add := func(v string) { args = append(args, "-o", v) }

	add("task_file=" + opts.TaskFile)
	for _, kv := range [][2]string{{"repo", opts.Repo}, {"branch", opts.Branch}, {"label", opts.Label}, {"keep-vm", opts.Keep}} {
		if kv[1] != "" {
			add(kv[0] + "=" + kv[1])
		}
	}

	names := make([]string, 0, len(opts.Env))
	for name := range opts.Env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add("env." + name + "=" + opts.Env[name])
	}

	if opts.Detach {
		add("detach")
	}
	return args
}

func taskRef() string {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return fmt.Sprintf("%s%s-%x", TaskRefPrefix, time.Now().UTC().Format("20060102T150405"), nonce)
	}
	return fmt.Sprintf("%s%d", TaskRefPrefix, time.Now().UnixNano())
}
