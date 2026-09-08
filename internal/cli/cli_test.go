package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func run(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := dispatch(Env{Stdout: &stdout, Stderr: &stderr}, args)
	return stdout.String(), stderr.String(), code
}

func dispatchWithStdin(t *testing.T, env Env, args []string, stdin string) int {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(stdin); err != nil {
		t.Fatal(err)
	}
	w.Close()

	real := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = real }()

	return dispatch(env, args)
}

func TestVersion(t *testing.T) {
	stdout, _, code := run("version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout == "" {
		t.Error("expected a version string on stdout")
	}
}

func TestValidateOK(t *testing.T) {
	path := filepath.Join("..", "flow", "testdata", "flow.yaml")
	stdout, _, code := run("validate", path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, stdout=%q", code, stdout)
	}
}

func TestValidateMissingFile(t *testing.T) {
	_, stderr, code := run("validate", "/no/such/flow.yaml")
	if code == 0 {
		t.Fatal("expected a non-zero exit code for a missing file")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestRenderMatchesKranqShape(t *testing.T) {
	path := filepath.Join("..", "flow", "testdata", "flow.yaml")
	stdout, _, code := run("render", path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !bytes.Contains([]byte(stdout), []byte("run: go test ./...")) {
		t.Errorf("rendered output missing expected step:\n%s", stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, code := run("no-such-command")
	if code == 0 {
		t.Fatal("expected a non-zero exit code for an unknown command")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}
