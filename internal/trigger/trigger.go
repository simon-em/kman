package trigger

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/gitcache"
	"github.com/simon-em/kman/internal/integration/bitbucket"
	"github.com/simon-em/kman/internal/kranqpush"
	"github.com/simon-em/kman/internal/vault"
)

type Stage string

const (
	StageArgs        Stage = "args"
	StageCredentials Stage = "credentials"
	StageRender      Stage = "render"
	StageSource      Stage = "source"
	StagePush        Stage = "push"
)

type Error struct {
	Stage Stage
	Err   error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Options struct {
	KranqURL string
	Source   string
	Branch   string
	Label    string
	Keep     string
	Detach   bool
	AsUser   string
	ExtraEnv map[string]string
}

func Run(ctx context.Context, home string, spec flow.Spec, provided map[string]string, opts Options) (kranqpush.Result, error) {
	if opts.KranqURL == "" {
		return kranqpush.Result{}, fmt.Errorf("no kranq url given")
	}

	resolvedArgs, err := flow.ResolveArgs(spec, provided)
	if err != nil {
		return kranqpush.Result{}, &Error{StageArgs, err}
	}
	resolvedCreds, err := ResolveCredentials(home, spec, opts.AsUser)
	if err != nil {
		return kranqpush.Result{}, &Error{StageCredentials, err}
	}
	pushEnv, err := MergeEnv(resolvedArgs, resolvedCreds, opts.ExtraEnv)
	if err != nil {
		return kranqpush.Result{}, &Error{StageArgs, err}
	}

	rendered, err := flow.Render(spec)
	if err != nil {
		return kranqpush.Result{}, &Error{StageRender, err}
	}
	taskFile, err := writeTempTask(rendered)
	if err != nil {
		return kranqpush.Result{}, &Error{StageRender, err}
	}
	defer os.Remove(taskFile)

	sourceDir, err := resolveSource(ctx, home, opts.Source)
	if err != nil {
		return kranqpush.Result{}, &Error{StageSource, err}
	}

	if err := kranqpush.UpdateMirror(ctx, sourceDir, "HEAD", opts.KranqURL, gitcache.Slug(opts.Source)); err != nil {
		fmt.Fprintf(os.Stderr, "kman: warning: %v\n", err)
	}

	result, err := kranqpush.Push(ctx, sourceDir, "HEAD", kranqpush.Options{
		KranqURL: opts.KranqURL,
		TaskFile: taskFile,
		Repo:     spec.Repo,
		Branch:   firstNonEmpty(opts.Branch, spec.Branch),
		Label:    opts.Label,
		Keep:     opts.Keep,
		Env:      pushEnv,
		Detach:   opts.Detach,
	})
	if err != nil {
		return kranqpush.Result{}, &Error{StagePush, err}
	}
	return result, nil
}

func ResolveCredentials(home string, spec flow.Spec, userID string) (map[string]string, error) {
	if len(spec.Credentials) == 0 {
		return nil, nil
	}
	var v *vault.Vault
	resolved := map[string]string{}
	for envName, secretName := range spec.Credentials {
		if name, ok := strings.CutPrefix(secretName, "integration:"); ok {
			value, err := ResolveIntegrationCredential(home, name, userID)
			if err != nil {
				return nil, fmt.Errorf("credential %s (%s): %w", envName, secretName, err)
			}
			resolved[envName] = value
			continue
		}
		if v == nil {
			var err error
			v, err = vault.Open(home)
			if err != nil {
				return nil, err
			}
		}
		value, err := v.Get(secretName)
		if err != nil {
			return nil, fmt.Errorf("credential %s (%s): %w", envName, secretName, err)
		}
		resolved[envName] = value
	}
	return resolved, nil
}

func ResolveIntegrationCredential(home, name, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("integration:%s needs an acting user; pass --as or set KMAN_ACTOR", name)
	}
	switch name {
	case "bitbucket":
		provider, err := bitbucket.FromVault(home)
		if err != nil {
			return "", err
		}
		cred, err := provider.Credential(context.Background(), userID)
		if err != nil {
			return "", err
		}
		return cred.Value, nil
	default:
		return "", fmt.Errorf("unknown integration %q", name)
	}
}

func MergeEnv(sources ...map[string]string) (map[string]string, error) {
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

func ParseAssignments(args []string) (map[string]string, error) {
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

func resolveSource(ctx context.Context, home, source string) (string, error) {
	if LooksLikeRemote(source) {
		return gitcache.Sync(ctx, home, source)
	}
	return gitDirOf(source)
}

func LooksLikeRemote(s string) bool {
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
