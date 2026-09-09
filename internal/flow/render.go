package flow

import "gopkg.in/yaml.v3"

type kranqStep struct {
	Name            string               `yaml:"name"`
	Run             string               `yaml:"run,omitempty"`
	Claude          string               `yaml:"claude,omitempty"`
	AllowedTools    []string             `yaml:"allowed_tools,omitempty"`
	DisallowedTools []string             `yaml:"disallowed_tools,omitempty"`
	MaxTurns        int                  `yaml:"max_turns,omitempty"`
	Effort          string               `yaml:"effort,omitempty"`
	Model           string               `yaml:"model,omitempty"`
	PermissionMode  string               `yaml:"permission_mode,omitempty"`
	MCPServers      map[string]MCPServer `yaml:"mcp_servers,omitempty"`
	ContinueOn      bool                 `yaml:"continue_on_error,omitempty"`
}

type kranqFile struct {
	Path    string `yaml:"path"`
	Mode    string `yaml:"mode"`
	Content string `yaml:"content"`
}

type kranqTask struct {
	Name      string            `yaml:"name"`
	Repo      string            `yaml:"repo,omitempty"`
	Branch    string            `yaml:"branch,omitempty"`
	Label     string            `yaml:"label,omitempty"`
	Kranqfile string            `yaml:"kranqfile,omitempty"`
	Resources Resources         `yaml:"resources,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Files     []kranqFile       `yaml:"files,omitempty"`
	Steps     []kranqStep       `yaml:"steps"`
}

func Render(s Spec) (string, error) {
	files := make([]kranqFile, len(s.Files))
	for i, f := range s.Files {
		files[i] = kranqFile{Path: f.Path, Mode: f.Mode, Content: f.Base64Content()}
	}
	t := kranqTask{
		Name:      s.Name,
		Repo:      s.Repo,
		Branch:    s.Branch,
		Label:     s.Label,
		Kranqfile: s.Kranqfile,
		Resources: s.Resources,
		Env:       s.Env,
		Files:     files,
		Steps:     make([]kranqStep, len(s.Steps)),
	}
	for i, step := range s.Steps {
		t.Steps[i] = kranqStep{
			Name:            step.Name,
			Run:             step.Run,
			Claude:          step.Claude,
			AllowedTools:    step.AllowedTools,
			DisallowedTools: step.DisallowedTools,
			MaxTurns:        step.MaxTurns,
			Effort:          step.Effort,
			Model:           step.Model,
			PermissionMode:  step.PermissionMode,
			MCPServers:      step.MCPServers,
			ContinueOn:      step.ContinueOn,
		}
	}
	out, err := yaml.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
