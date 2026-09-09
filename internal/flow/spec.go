package flow

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

const MaxFileSize = 1 << 20

type File struct {
	Path    string `yaml:"path"`
	Mode    string `yaml:"mode"`
	Content string `yaml:"content"`
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
	Files       []File            `yaml:"files"`
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
	for i, f := range s.Files {
		if err := f.validate(); err != nil {
			return fmt.Errorf("files[%d] (%s): %w", i, f.Path, err)
		}
	}
	for i, step := range s.Steps {
		if err := step.validate(); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Name, err)
		}
	}
	return nil
}

func (f File) validate() error {
	if f.Path == "" {
		return errors.New("needs a path")
	}
	if strings.Contains(f.Path, "..") {
		return fmt.Errorf("path %q must not contain \"..\"", f.Path)
	}
	if f.Mode != "" {
		if _, err := strconv.ParseUint(f.Mode, 8, 32); err != nil {
			return fmt.Errorf("mode %q is not valid octal", f.Mode)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return fmt.Errorf("content is not base64: %w", err)
	}
	if len(decoded) > MaxFileSize {
		return fmt.Errorf("is %d bytes, over the %d byte limit", len(decoded), MaxFileSize)
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
