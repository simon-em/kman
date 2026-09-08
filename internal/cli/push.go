package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/gitcache"
	"github.com/simon-em/kman/internal/kranqpush"
	"github.com/simon-em/kman/internal/vault"
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

	provided, err := parseArgAssignments(positional[1:])
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Usage
	}
	resolvedArgs, err := flow.ResolveArgs(spec, provided)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	resolvedCreds, err := resolveCredentials(spec)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}
	pushEnv, err := mergeEnv(resolvedArgs, resolvedCreds)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InvalidSpec
	}

	if *kranqURL == "" {
		fmt.Fprintln(env.Stderr, "kman: set --kranq-url (or KMAN_KRANQ_URL)")
		return exitcode.Usage
	}

	rendered, err := flow.Render(spec)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	taskFile, err := writeTempTask(rendered)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}
	defer os.Remove(taskFile)

	ctx := context.Background()
	sourceDir, err := resolveSource(ctx, *source)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.InternalError
	}

	if err := kranqpush.UpdateMirror(ctx, sourceDir, "HEAD", *kranqURL, gitcache.Slug(*source)); err != nil {
		fmt.Fprintf(env.Stderr, "kman: warning: %v\n", err)
	}

	result, err := kranqpush.Push(ctx, sourceDir, "HEAD", kranqpush.Options{
		KranqURL: *kranqURL,
		TaskFile: taskFile,
		Repo:     spec.Repo,
		Branch:   firstNonEmpty(*branch, spec.Branch),
		Label:    *label,
		Keep:     *keep,
		Env:      pushEnv,
		Detach:   *detach,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "kman: %v\n", err)
		return exitcode.Unreachable
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

func resolveCredentials(spec flow.Spec) (map[string]string, error) {
	if len(spec.Credentials) == 0 {
		return nil, nil
	}
	v, err := vault.Open(kmanHome())
	if err != nil {
		return nil, err
	}
	resolved := map[string]string{}
	for envName, secretName := range spec.Credentials {
		value, err := v.Get(secretName)
		if err != nil {
			return nil, fmt.Errorf("credential %s (%s): %w", envName, secretName, err)
		}
		resolved[envName] = value
	}
	return resolved, nil
}

func mergeEnv(sources ...map[string]string) (map[string]string, error) {
	merged := map[string]string{}
	for _, source := range sources {
		for name, value := range source {
			if _, exists := merged[name]; exists {
				return nil, fmt.Errorf("%q is set by more than one of the flow's args/credentials", name)
			}
			merged[name] = value
		}
	}
	return merged, nil
}

func parseArgAssignments(args []string) (map[string]string, error) {
	out := map[string]string{}
	for _, a := range args {
		name, value, ok := strings.Cut(a, "=")
		if !ok {
			return nil, fmt.Errorf("%q is not NAME=VALUE", a)
		}
		out[name] = value
	}
	return out, nil
}

func writeTempTask(content string) (string, error) {
	f, err := os.CreateTemp("", "kman-task-*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func resolveSource(ctx context.Context, source string) (string, error) {
	if looksLikeRemote(source) {
		return gitcache.Sync(ctx, kmanHome(), source)
	}
	return gitDirOf(source)
}

func looksLikeRemote(s string) bool {
	return strings.Contains(s, "://") || strings.Contains(s, "@")
}

func gitDirOf(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--absolute-git-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s is not a git directory: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
