package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalFallback(t *testing.T) {
	home := t.TempDir()
	flowsDir := filepath.Join(home, "config", "flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	flowYAML := "name: hello\nsteps:\n  - name: a\n    run: echo hi\n"
	if err := os.WriteFile(filepath.Join(flowsDir, "hello.yaml"), []byte(flowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Flows) != 1 {
		t.Fatalf("len(Flows) = %d, want 1", len(cfg.Flows))
	}
	if cfg.Flows[0].Name != "hello" {
		t.Errorf("Flows[0].Name = %q, want hello", cfg.Flows[0].Name)
	}
}

func TestLoadLocalFallbackNoConfigDir(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Flows) != 0 {
		t.Errorf("len(Flows) = %d, want 0", len(cfg.Flows))
	}
}

func writeConfigFile(t *testing.T, home, rel, content string) {
	t.Helper()
	path := filepath.Join(home, "config", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadUsersAndGroups(t *testing.T) {
	home := t.TempDir()
	writeConfigFile(t, home, "flows/deploy.yaml", "name: deploy-review\nsteps:\n  - name: a\n    run: echo hi\n")
	writeConfigFile(t, home, "users/simon.yaml", "id: simon\nslack_user_id: U123\nflows: [deploy-review]\n")
	writeConfigFile(t, home, "groups/oncall.yaml", "name: oncall\nmembers: [simon]\nflows: [deploy-review]\n")

	cfg, err := Load(home, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Access.Users) != 1 || len(cfg.Access.Groups) != 1 {
		t.Fatalf("Access = %+v", cfg.Access)
	}
	if !cfg.Access.CanTrigger("simon", "deploy-review") {
		t.Error("expected simon to be able to trigger deploy-review")
	}
}

func TestLoadRejectsAGrantForAnUnknownFlow(t *testing.T) {
	home := t.TempDir()
	writeConfigFile(t, home, "users/simon.yaml", "id: simon\nflows: [no-such-flow]\n")

	if _, err := Load(home, ""); err == nil {
		t.Fatal("expected an error for a user grant referencing an unknown flow")
	}
}

func TestLoadRejectsAFlowAccessReferencingAnUnknownFlow(t *testing.T) {
	home := t.TempDir()
	writeConfigFile(t, home, "flows/a.yaml", "name: a\naccess:\n  flows: [no-such-flow]\nsteps:\n  - name: s\n    run: echo hi\n")

	if _, err := Load(home, ""); err == nil {
		t.Fatal("expected an error for access.flows referencing an unknown flow")
	}
}
