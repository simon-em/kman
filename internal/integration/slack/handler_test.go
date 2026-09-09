package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/reporegistry"
	"github.com/simon-em/kman/internal/vault"
)

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func newRemoteWithCommit(t *testing.T) string {
	t.Helper()
	origin := t.TempDir()
	mustRunGit(t, origin, "init", "-q")
	mustRunGit(t, origin, "config", "user.email", "test@example.com")
	mustRunGit(t, origin, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(origin, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, origin, "add", "file.txt")
	mustRunGit(t, origin, "commit", "-q", "-m", "initial")
	return "file://" + origin
}

func newBareKranqRepoWithHook(t *testing.T, hookBody string) string {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-q", "--bare")
	mustRunGit(t, dir, "config", "receive.denyCurrentBranch", "ignore")
	mustRunGit(t, dir, "config", "receive.advertisePushOptions", "true")
	hookPath := filepath.Join(dir, "hooks", "post-receive")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"+hookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

type fakeSlackAPI struct {
	mu       sync.Mutex
	messages []string
}

func (f *fakeSlackAPI) add(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, text)
}

func (f *fakeSlackAPI) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.messages))
	copy(out, f.messages)
	return out
}

func newFakeSlackServer(t *testing.T) (*httptest.Server, *fakeSlackAPI) {
	t.Helper()
	fake := &fakeSlackAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		fake.add(body.Text)
		w.Write([]byte(`{"ok":true,"ts":"1.1"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, fake
}

func configureSlack(t *testing.T, home, slackBaseURL string) string {
	t.Helper()
	v, err := vault.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(BotTokenKey, "xoxb-test"); err != nil {
		t.Fatal(err)
	}
	secret := "shhh-signing-secret"
	if err := v.Set(SigningSecretKey, secret); err != nil {
		t.Fatal(err)
	}
	if err := v.Set(BaseURLKey, slackBaseURL); err != nil {
		t.Fatal(err)
	}
	return secret
}

func signedRequest(t *testing.T, url, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := sign(secret, ts, string(body))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	return req
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestHandlerAnswersURLVerification(t *testing.T) {
	home := t.TempDir()
	slackSrv, _ := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)

	h := NewHandler(home, "unused", "", "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	body := []byte(`{"type":"url_verification","challenge":"the-challenge"}`)
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if buf.String() != "the-challenge" {
		t.Errorf("body = %q", buf.String())
	}
}

func TestHandlerRejectsAnInvalidSignature(t *testing.T) {
	home := t.TempDir()
	slackSrv, _ := newFakeSlackServer(t)
	configureSlack(t, home, slackSrv.URL)

	h := NewHandler(home, "unused", "", "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	body := []byte(`{"type":"url_verification","challenge":"x"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Slack-Signature", "v0=bogus")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHandlerTriggersAnAllowedFlowAndRepliesWithTheResult(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)

	remote := newRemoteWithCommit(t)
	kranq := newBareKranqRepoWithHook(t, `echo "KRANQ-RESULT id=slack-test status=ok exit=0"`)

	spec := flow.Spec{
		Name:  "deploy-review",
		Repo:  remote,
		Steps: []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	if err := config.SaveFlow(home, spec, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123", Flows: []string{"deploy-review"}}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, kranq, "", "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> run deploy-review","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "status=ok") {
				return true
			}
		}
		return false
	})
}

func TestHandlerRoutesFreeFormTextToTheDefaultFlow(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)

	taskFile := filepath.Join(t.TempDir(), "captured_task.txt")
	remote := newRemoteWithCommit(t)
	kranq := newBareKranqRepoWithHook(t, fmt.Sprintf(`
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    env.TASK=*) echo "${val#env.TASK=}" > %s ;;
  esac
  i=$((i+1))
done
echo "KRANQ-RESULT id=default-flow-test status=ok exit=0"
`, taskFile))

	spec := flow.Spec{
		Name:  "create-feature",
		Repo:  remote,
		Args:  map[string]flow.Arg{"TASK": {Required: true}},
		Steps: []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	if err := config.SaveFlow(home, spec, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123", Flows: []string{"create-feature"}}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, kranq, "", "create-feature")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> add a TEST.md file to kman-demo","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "create-feature") && strings.Contains(m, "status=ok") {
				return true
			}
		}
		return false
	})

	taskBytes, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatalf("hook did not see the TASK env var: %v", err)
	}
	if strings.TrimSpace(string(taskBytes)) != "add a TEST.md file to kman-demo" {
		t.Errorf("TASK = %q", taskBytes)
	}
}

func TestHandlerRefusesAFlowTheUserIsNotGrantedFor(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)

	spec := flow.Spec{
		Name:  "deploy-review",
		Repo:  newRemoteWithCommit(t),
		Steps: []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	if err := config.SaveFlow(home, spec, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123"}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, "unused", "", "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> run deploy-review","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "not authorized") {
				return true
			}
		}
		return false
	})
}

func newFakeClaude(t *testing.T, response string) string {
	t.Helper()
	binDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "claude-args.log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf 'ARG>>>%s<<<\\n' \"$a\" >> \"$FAKE_CLAUDE_LOG\"; done\n" +
		"printf '%s' \"$FAKE_CLAUDE_RESPONSE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_LOG", logFile)
	t.Setenv("FAKE_CLAUDE_RESPONSE", response)
	return logFile
}

func TestHandlerRoutesFreeFormTextViaDispatch(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)
	newFakeClaude(t, `{"is_error":false,"result":"{}","structured_output":{"flow":"create-feature","repo":"dx","task":"add a TEST.md file","clarify":""}}`)

	repoFile := filepath.Join(t.TempDir(), "captured_repo.txt")
	taskFile := filepath.Join(t.TempDir(), "captured_task.txt")
	remote := newRemoteWithCommit(t)
	kranq := newBareKranqRepoWithHook(t, fmt.Sprintf(`
count="${GIT_PUSH_OPTION_COUNT:-0}"
i=0
while [ "$i" -lt "$count" ]; do
  eval "val=\$GIT_PUSH_OPTION_$i"
  case "$val" in
    repo=*) echo "${val#repo=}" > %s ;;
    env.TASK=*) echo "${val#env.TASK=}" > %s ;;
  esac
  i=$((i+1))
done
echo "KRANQ-RESULT id=dispatch-test status=ok exit=0"
`, repoFile, taskFile))

	spec := flow.Spec{
		Name:        "create-feature",
		Description: "opens a small PR making a requested change",
		Args:        map[string]flow.Arg{"TASK": {Required: true}},
		Steps:       []flow.Step{{Name: "a", Run: "echo hi"}},
	}
	if err := config.SaveFlow(home, spec, "test"); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveRepo(home, reporegistry.Repo{Name: "dx", URL: remote}, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123", Flows: []string{"create-feature"}}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, kranq, "", "")
	h.Dispatch = true
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> add a TEST.md file to dx","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "create-feature") && strings.Contains(m, "status=ok") {
				return true
			}
		}
		return false
	})

	if got, _ := os.ReadFile(repoFile); strings.TrimSpace(string(got)) != remote {
		t.Errorf("repo option = %q, want the dispatch-resolved repo %q", got, remote)
	}
	if got, _ := os.ReadFile(taskFile); strings.TrimSpace(string(got)) != "add a TEST.md file" {
		t.Errorf("TASK = %q", got)
	}
}

func TestHandlerDispatchOnlyOffersFlowsTheUserIsGrantedFor(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)
	logFile := newFakeClaude(t, `{"is_error":false,"result":"{}","structured_output":{"flow":"","repo":"","task":"","clarify":"no match"}}`)

	granted := flow.Spec{Name: "create-feature", Description: "opens a small PR", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}
	notGranted := flow.Spec{Name: "delete-everything", Description: "a dangerous flow simon is not granted", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}
	if err := config.SaveFlow(home, granted, "test"); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFlow(home, notGranted, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123", Flows: []string{"create-feature"}}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, "unused", "", "")
	h.Dispatch = true
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> do something","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "no match") {
				return true
			}
		}
		return false
	})

	logged, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("fake claude was never invoked: %v", err)
	}
	if strings.Contains(string(logged), "delete-everything") {
		t.Errorf("logged args = %q, want the ungranted flow never offered as a candidate", logged)
	}
	if !strings.Contains(string(logged), "create-feature") {
		t.Errorf("logged args = %q, want the granted flow offered as a candidate", logged)
	}
}

func TestHandlerDispatchClarifiesWithoutPushingWhenAmbiguous(t *testing.T) {
	home := t.TempDir()
	slackSrv, fakeSlack := newFakeSlackServer(t)
	secret := configureSlack(t, home, slackSrv.URL)
	newFakeClaude(t, `{"is_error":false,"result":"{}","structured_output":{"flow":"","repo":"","task":"","clarify":"which repo did you mean?"}}`)

	spec := flow.Spec{Name: "create-feature", Description: "opens a small PR", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}
	if err := config.SaveFlow(home, spec, "test"); err != nil {
		t.Fatal(err)
	}
	user := access.User{ID: "simon", SlackUserID: "U123", Flows: []string{"create-feature"}}
	if err := config.SaveUser(home, user, "test"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(home, "should-never-be-used", "", "")
	h.Dispatch = true
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	event := `{"type":"event_callback","event":{"type":"app_mention","channel":"C1","user":"U123","text":"<@UBOT> do the thing","ts":"1.1"}}`
	resp, err := http.DefaultClient.Do(signedRequest(t, srv.URL, secret, []byte(event)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	waitFor(t, 5*time.Second, func() bool {
		for _, m := range fakeSlack.snapshot() {
			if strings.Contains(m, "which repo did you mean?") {
				return true
			}
		}
		return false
	})
}
