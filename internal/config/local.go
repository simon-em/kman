package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/flow"
)

func loadLocal(dir string) (Config, error) {
	flows, err := loadFlows(filepath.Join(dir, "flows"))
	if err != nil {
		return Config{}, err
	}
	registry, err := loadRegistry(dir)
	if err != nil {
		return Config{}, err
	}
	knownFlows := map[string]bool{}
	for _, f := range flows {
		knownFlows[f.Name] = true
	}
	if err := registry.Validate(knownFlows); err != nil {
		return Config{}, err
	}
	if err := validateFlowReferences(flows, knownFlows); err != nil {
		return Config{}, err
	}
	return Config{Flows: flows, Access: registry}, nil
}

func loadFlows(dir string) ([]flow.Spec, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var flows []flow.Spec
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		spec, err := flow.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		flows = append(flows, spec)
	}
	return flows, nil
}

func validateFlowReferences(flows []flow.Spec, knownFlows map[string]bool) error {
	for _, f := range flows {
		for _, ref := range f.Access.Flows {
			if !knownFlows[ref] {
				return fmt.Errorf("flow %s: access.flows references unknown flow %q", f.Name, ref)
			}
		}
	}
	return nil
}

func loadRegistry(dir string) (access.Registry, error) {
	registry := access.NewRegistry()
	users, err := loadUsers(filepath.Join(dir, "users"))
	if err != nil {
		return access.Registry{}, err
	}
	registry.Users = users
	groups, err := loadGroups(filepath.Join(dir, "groups"))
	if err != nil {
		return access.Registry{}, err
	}
	registry.Groups = groups
	return registry, nil
}

func loadUsers(dir string) (map[string]access.User, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]access.User{}, nil
	}
	if err != nil {
		return nil, err
	}
	users := map[string]access.User{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		u, err := access.ParseUser(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		users[u.ID] = u
	}
	return users, nil
}

func loadGroups(dir string) (map[string]access.Group, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]access.Group{}, nil
	}
	if err != nil {
		return nil, err
	}
	groups := map[string]access.Group{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		g, err := access.ParseGroup(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		groups[g.Name] = g
	}
	return groups, nil
}
