package flow

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Resources struct {
	Memory string `yaml:"memory"`
	CPUs   int    `yaml:"cpus"`
	Disk   string `yaml:"disk"`
}

type Arg struct {
	Default     string `yaml:"default"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

type MCPServer struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

type Step struct {
	Name            string               `yaml:"name"`
	Run             string               `yaml:"run"`
	Claude          string               `yaml:"claude"`
	AllowedTools    []string             `yaml:"allowed_tools"`
	DisallowedTools []string             `yaml:"disallowed_tools"`
	MaxTurns        int                  `yaml:"max_turns"`
	Effort          string               `yaml:"effort"`
	Model           string               `yaml:"model"`
	PermissionMode  string               `yaml:"permission_mode"`
	MCPServers      map[string]MCPServer `yaml:"mcp_servers"`
	ContinueOn      bool                 `yaml:"continue_on_error"`
}

type Spec struct {
	Name        string            `yaml:"name"`
	Repo        string            `yaml:"repo"`
	Branch      string            `yaml:"branch"`
	Label       string            `yaml:"label"`
	Kranqfile   string            `yaml:"kranqfile"`
	Resources   Resources         `yaml:"resources"`
	Args        map[string]Arg    `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Credentials map[string]string `yaml:"credentials"`
	Access      Access            `yaml:"access"`
	Steps       []Step            `yaml:"steps"`
}

var validPermissionModes = map[string]bool{
	"acceptEdits":       true,
	"auto":              true,
	"bypassPermissions": true,
	"manual":            true,
	"dontAsk":           true,
	"plan":              true,
}

func Parse(data []byte) (Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Spec{}, fmt.Errorf("parsing flow: %w", err)
	}
	if err := s.validate(); err != nil {
		return Spec{}, err
	}
	return s, nil
}

func (s Spec) validate() error {
	if s.Name == "" {
		return errors.New("flow needs a name")
	}
	if len(s.Steps) == 0 {
		return errors.New("flow needs at least one step")
	}
	for argName, arg := range s.Args {
		if arg.Required && arg.Default != "" {
			return fmt.Errorf("arg %q: required and default are mutually exclusive", argName)
		}
	}
	for envName, secretName := range s.Credentials {
		if envName == "" || secretName == "" {
			return fmt.Errorf("credentials: an empty env name or secret name (env=%q secret=%q)", envName, secretName)
		}
		if !s.Access.grants(secretName) {
			return fmt.Errorf("credentials: %q binds secret %q, which is not listed in access.credentials", envName, secretName)
		}
	}
	if err := s.Access.validate(); err != nil {
		return err
	}
	for i, step := range s.Steps {
		if err := step.validate(); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Name, err)
		}
	}
	return nil
}

func (s Step) validate() error {
	if s.Name == "" {
		return errors.New("needs a name")
	}
	if s.Run == "" && s.Claude == "" {
		return errors.New("needs either run or claude")
	}
	if s.Run != "" && s.Claude != "" {
		return errors.New("run and claude are mutually exclusive")
	}
	if s.PermissionMode != "" && !validPermissionModes[s.PermissionMode] {
		return fmt.Errorf("invalid permission_mode %q", s.PermissionMode)
	}
	if len(s.MCPServers) > 0 && s.Claude == "" {
		return errors.New("mcp_servers is only valid on a claude step")
	}
	return nil
}
