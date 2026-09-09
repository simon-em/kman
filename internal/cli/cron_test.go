package cli

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/cron"
)

func TestCronSetListAndRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	writeFlow(t, home, "report", "name: report\nsteps:\n  - name: a\n    run: echo hi\n")

	if _, stderr, code := run("cron", "set", "nightly", "--flow", "report", "--schedule", "0 6 * * *"); code != 0 {
		t.Fatalf("cron set exit code = %d, stderr=%s", code, stderr)
	}
	stdout, _, code := run("cron", "ls")
	if code != 0 {
		t.Fatalf("cron ls exit code = %d", code)
	}
	if !strings.Contains(stdout, "nightly") || !strings.Contains(stdout, "report") {
		t.Errorf("cron ls = %q", stdout)
	}

	if _, stderr, code := run("cron", "rm", "nightly"); code != 0 {
		t.Fatalf("cron rm exit code = %d, stderr=%s", code, stderr)
	}
	stdout, _, code = run("cron", "ls")
	if code != 0 || strings.Contains(stdout, "nightly") {
		t.Errorf("cron ls after rm = %q, code=%d", stdout, code)
	}
}

func TestCronSetRejectsAnUnknownFlow(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())
	if _, _, code := run("cron", "set", "nightly", "--flow", "no-such-flow", "--schedule", "0 6 * * *"); code == 0 {
		t.Fatal("expected a non-zero exit code for an unknown flow")
	}
}

func TestCronSetRejectsAnInvalidSchedule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)
	writeFlow(t, home, "report", "name: report\nsteps:\n  - name: a\n    run: echo hi\n")
	if _, _, code := run("cron", "set", "nightly", "--flow", "report", "--schedule", "nonsense"); code == 0 {
		t.Fatal("expected a non-zero exit code for an invalid schedule")
	}
}

func TestTickCronFiresADueEntryAndSkipsAFutureOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	remote := newSourceRepo(t)
	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=tick-test status=ok exit=0"`)

	writeFlow(t, home, "report", "name: report\nrepo: file://"+remote+"\nsteps:\n  - name: a\n    run: echo hi\n")
	if err := config.SaveCronEntry(home, cron.Entry{Name: "due", Flow: "report", Schedule: "* * * * *"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveCronEntry(home, cron.Entry{Name: "not-due", Flow: "report", Schedule: "5 0 1 1 *"}, ""); err != nil {
		t.Fatal(err)
	}

	fired, err := tickCron(context.Background(), Env{Stdout: os.Stdout, Stderr: os.Stderr}, home, kranq, time.Now())
	if err != nil {
		t.Fatalf("tickCron: %v", err)
	}
	if fired != 1 {
		t.Errorf("fired = %d, want 1", fired)
	}
}

func TestTickCronSkipsAnEntryWhoseFlowHasNoRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KMAN_HOME", home)

	writeFlow(t, home, "report", "name: report\nsteps:\n  - name: a\n    run: echo hi\n")
	if err := config.SaveCronEntry(home, cron.Entry{Name: "due", Flow: "report", Schedule: "* * * * *"}, ""); err != nil {
		t.Fatal(err)
	}

	fired, err := tickCron(context.Background(), Env{Stdout: os.Stdout, Stderr: os.Stderr}, home, "unused", time.Now())
	if err != nil {
		t.Fatalf("tickCron: %v", err)
	}
	if fired != 0 {
		t.Errorf("fired = %d, want 0 (the flow has no remote repo)", fired)
	}
}

func TestCronServeRequiresKranqURL(t *testing.T) {
	t.Setenv("KMAN_HOME", t.TempDir())
	t.Setenv("KMAN_KRANQ_URL", "")
	if _, _, code := run("cron", "serve"); code == 0 {
		t.Fatal("expected a non-zero exit code with no --kranq-url")
	}
}
