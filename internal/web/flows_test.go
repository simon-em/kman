package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simon-em/kman/internal/config"
	"github.com/simon-em/kman/internal/reporegistry"
)

func TestSaveFlowRoundTripsEveryField(t *testing.T) {
	srv, home := newTestServer(t)

	form := url.Values{
		"is_new":      {"true"},
		"name":        {"create-feature"},
		"description": {"opens a small PR"},
		"repo":        {"https://bitbucket.org/smntlbt/kman-demo.git"},
		"branch":      {"main"},
		"memory":      {"4GiB"},
		"cpus":        {"2"},

		"arg_count":              {"1"},
		"arg_name_0":             {"TASK"},
		"arg_description_0":      {"what to do"},
		"arg_required_0":         {"on"},
		"env_count":              {"1"},
		"env_name_0":             {"REGION"},
		"env_value_0":            {"us-east-1"},
		"cred_count":             {"1"},
		"cred_name_0":            {"BITBUCKET_TOKEN"},
		"cred_value_0":           {"integration:bitbucket:repository:write"},
		"file_count":             {"1"},
		"file_path_0":            {".claude/skills/x/SKILL.md"},
		"file_mode_0":            {"0644"},
		"file_text_0":            {"---\nname: x\n---\nhi"},
		"skill_count":            {"1"},
		"skill_0":                {"catalog:bitbucket@abc123"},
		"access_cred_count":      {"1"},
		"access_cred_0":          {"integration:bitbucket:repository:write"},
		"meta_ask":               {"on"},
		"meta_cron_create":       {"on"},
		"tool_count":             {"1"},
		"tool_0":                 {"Bash"},
		"step_count":             {"1"},
		"step_name_0":            {"a"},
		"step_type_0":            {"claude"},
		"step_body_0":            {"do the thing"},
		"step_permission_mode_0": {"bypassPermissions"},
		"step_max_turns_0":       {"10"},
		"step_continue_0":        {"on"},
	}

	resp := mustPost(t, srv, "/flows/save", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	spec, err := config.LoadFlow(home, "create-feature")
	if err != nil {
		t.Fatalf("LoadFlow: %v", err)
	}
	if spec.Description != "opens a small PR" || spec.Repo != "https://bitbucket.org/smntlbt/kman-demo.git" {
		t.Errorf("spec = %+v", spec)
	}
	if spec.Resources.CPUs != 2 || spec.Resources.Memory != "4GiB" {
		t.Errorf("Resources = %+v", spec.Resources)
	}
	if a, ok := spec.Args["TASK"]; !ok || !a.Required || a.Description != "what to do" {
		t.Errorf("Args = %+v", spec.Args)
	}
	if spec.Env["REGION"] != "us-east-1" {
		t.Errorf("Env = %+v", spec.Env)
	}
	if spec.Credentials["BITBUCKET_TOKEN"] != "integration:bitbucket:repository:write" {
		t.Errorf("Credentials = %+v", spec.Credentials)
	}
	if len(spec.Files) != 1 || spec.Files[0].Path != ".claude/skills/x/SKILL.md" || spec.Files[0].Mode != "0644" {
		t.Errorf("Files = %+v", spec.Files)
	}
	if len(spec.Access.Skills) != 1 || spec.Access.Skills[0] != "catalog:bitbucket@abc123" {
		t.Errorf("Access.Skills = %v", spec.Access.Skills)
	}
	if len(spec.Access.Credentials) != 1 || spec.Access.Credentials[0] != "integration:bitbucket:repository:write" {
		t.Errorf("Access.Credentials = %v", spec.Access.Credentials)
	}
	if len(spec.Access.Meta) != 2 || !contains(spec.Access.Meta, "ask") || !contains(spec.Access.Meta, "cron.create") {
		t.Errorf("Access.Meta = %v", spec.Access.Meta)
	}
	if len(spec.Access.Tools) != 1 || spec.Access.Tools[0] != "Bash" {
		t.Errorf("Access.Tools = %v", spec.Access.Tools)
	}
	if len(spec.Steps) != 1 {
		t.Fatalf("Steps = %+v", spec.Steps)
	}
	step := spec.Steps[0]
	if step.Claude != "do the thing" || step.Run != "" {
		t.Errorf("step = %+v, want Claude set and Run empty", step)
	}
	if step.PermissionMode != "bypassPermissions" || step.MaxTurns != 10 || !step.ContinueOn {
		t.Errorf("step = %+v", step)
	}
}

func contains(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}

func TestFlowEditPageOffersRegisteredReposAndCatalogSkills(t *testing.T) {
	srv, home := newTestServer(t)
	if err := config.SaveRepo(home, reporegistry.Repo{Name: "dx", URL: "https://bitbucket.org/smntlbt/dx.git"}, ""); err != nil {
		t.Fatal(err)
	}

	resp := mustGet(t, srv, "/flows/new")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "https://bitbucket.org/smntlbt/dx.git") {
		t.Error("expected the registered repo's URL to appear as a suggestion")
	}
	if !strings.Contains(body, "catalog:bitbucket@") {
		t.Error("expected the built-in bitbucket skill to appear as a suggestion")
	}
}
