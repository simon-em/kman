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
	Text    string `yaml:"text"`
}

func (f File) Base64Content() string {
	if f.Content != "" {
		return f.Content
	}
	return base64.StdEncoding.EncodeToString([]byte(f.Text))
}

type Spec struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
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

var PermissionModes = []string{"acceptEdits", "auto", "bypassPermissions", "manual", "dontAsk", "plan"}

var validPermissionModes = func() map[string]bool {
	m := make(map[string]bool, len(PermissionModes))
	for _, v := range PermissionModes {
		m[v] = true
	}
	return m
}()

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
	for _, ref := range s.Access.Skills {
		if err := s.validateSkillRef(ref); err != nil {
			return err
		}
	}
	for i, step := range s.Steps {
		if err := step.validate(); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Name, err)
		}
	}
	return nil
}

func (s Spec) validateSkillRef(ref string) error {
	parsed, err := ParseSkillRef(ref)
	if err != nil {
		return fmt.Errorf("access.skills: %w", err)
	}
	if parsed.Kind != "local" {
		return nil
	}
	f, ok := fileAtPath(s.Files, parsed.Path)
	if !ok {
		return fmt.Errorf("access.skills: local:%s does not match any files: entry", parsed.Path)
	}
	if isMarkdownPath(parsed.Path) {
		return nil
	}
	if !isExecutableMode(f.Mode) {
		return fmt.Errorf("access.skills: local:%s must have an executable files: mode (e.g. \"0755\")", parsed.Path)
	}
	return nil
}

func isMarkdownPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".md")
}

func fileAtPath(files []File, path string) (File, bool) {
	for _, f := range files {
		if f.Path == path {
			return f, true
		}
	}
	return File{}, false
}

func isExecutableMode(mode string) bool {
	if mode == "" {
		return false
	}
	n, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return false
	}
	return n&0o100 != 0
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
	if f.Content == "" && f.Text == "" {
		return errors.New("needs either content or text")
	}
	if f.Content != "" && f.Text != "" {
		return errors.New("content and text are mutually exclusive")
	}
	if f.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return fmt.Errorf("content is not base64: %w", err)
		}
		if len(decoded) > MaxFileSize {
			return fmt.Errorf("is %d bytes, over the %d byte limit", len(decoded), MaxFileSize)
		}
		return nil
	}
	if len(f.Text) > MaxFileSize {
		return fmt.Errorf("is %d bytes, over the %d byte limit", len(f.Text), MaxFileSize)
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
