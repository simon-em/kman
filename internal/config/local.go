package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/simon-em/kman/internal/flow"
)

func loadLocal(dir string) (Config, error) {
	flowsDir := filepath.Join(dir, "flows")
	entries, err := os.ReadDir(flowsDir)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(flowsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		spec, err := flow.Parse(data)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Flows = append(cfg.Flows, spec)
	}
	return cfg, nil
}
