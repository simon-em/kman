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
