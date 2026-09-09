package kranqpush

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os/exec"
	"sort"
	"time"
)

const TaskRefPrefix = "refs/heads/task/"

const MirrorRefPrefix = "refs/heads/mirror/"

type Options struct {
	KranqURL string
	Spec     []byte
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
	if len(opts.Spec) == 0 {
		return Result{}, fmt.Errorf("no task spec given")
	}
	optionArgs, err := pushOptionArgs(opts)
	if err != nil {
		return Result{}, err
	}
	ref := taskRef()
	args := []string{"--git-dir=" + sourceDir, "push", "--force", opts.KranqURL, rev + ":" + ref}
	args = append(args, optionArgs...)

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

// encodeSpec matches kranq's own gitsrv.EncodeSpec wire format (gzip, then
// base64) for the -o spec= push option. kman cannot import kranq's internal
// package, so this is a second implementation of the same small encoding,
// not a shared one.
func encodeSpec(spec []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(spec); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func pushOptionArgs(opts Options) ([]string, error) {
	var args []string
	add := func(v string) { args = append(args, "-o", v) }

	encoded, err := encodeSpec(opts.Spec)
	if err != nil {
		return nil, fmt.Errorf("encoding the task spec: %w", err)
	}
	add("spec=" + encoded)
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
	return args, nil
}

func taskRef() string {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return fmt.Sprintf("%s%s-%x", TaskRefPrefix, time.Now().UTC().Format("20060102T150405"), nonce)
	}
	return fmt.Sprintf("%s%d", TaskRefPrefix, time.Now().UnixNano())
}
