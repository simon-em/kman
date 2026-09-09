package flow

import (
	"fmt"
	"strings"
)

type SkillRef struct {
	Kind string
	Name string
	Ref  string
	Path string
}

func ParseSkillRef(s string) (SkillRef, error) {
	switch {
	case strings.HasPrefix(s, "catalog:"):
		rest := strings.TrimPrefix(s, "catalog:")
		name, ref, ok := strings.Cut(rest, "@")
		if !ok || name == "" || ref == "" {
			return SkillRef{}, fmt.Errorf("skill %q: want catalog:<name>@<ref>", s)
		}
		return SkillRef{Kind: "catalog", Name: name, Ref: ref}, nil
	case strings.HasPrefix(s, "local:"):
		path := strings.TrimPrefix(s, "local:")
		if path == "" {
			return SkillRef{}, fmt.Errorf("skill %q: want local:<path>", s)
		}
		return SkillRef{Kind: "local", Name: skillNameFromPath(path), Path: path}, nil
	default:
		return SkillRef{}, fmt.Errorf("skill %q: want catalog:<name>@<ref> or local:<path>", s)
	}
}

func skillNameFromPath(path string) string {
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return base
}
