package skills

import (
	"fmt"
	"strings"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/flow"
)

type Lookup func(name string) (catalog.Entry, bool)

func Compose(spec flow.Spec, lookup Lookup) (flow.Spec, error) {
	for _, ref := range spec.Access.Skills {
		parsed, err := flow.ParseSkillRef(ref)
		if err != nil {
			return flow.Spec{}, err
		}
		switch parsed.Kind {
		case "catalog":
			composed, err := composeCatalog(spec, parsed, lookup)
			if err != nil {
				return flow.Spec{}, err
			}
			spec = composed
		case "local":
			if isMarkdownPath(parsed.Path) {
				continue
			}
			spec = spec.WithMCPServer(parsed.Name, flow.MCPServer{Command: parsed.Path})
		}
	}
	return spec, nil
}

func composeCatalog(spec flow.Spec, ref flow.SkillRef, lookup Lookup) (flow.Spec, error) {
	entry, ok := lookup(ref.Name)
	if !ok {
		return flow.Spec{}, fmt.Errorf("skill %q: no such catalog entry %q", "catalog:"+ref.Name+"@"+ref.Ref, ref.Name)
	}
	if entry.Ref != ref.Ref {
		return flow.Spec{}, fmt.Errorf("skill catalog:%s@%s: the catalog's %s entry is now pinned at %s; update the flow's access.skills entry", ref.Name, ref.Ref, ref.Name, entry.Ref)
	}
	mode := "0755"
	if entry.Kind == catalog.KindDoc {
		mode = ""
	}
	spec = spec.WithFile(flow.File{
		Path: entry.Path,
		Mode: mode,
		Text: string(entry.Content),
	})
	if entry.Kind == catalog.KindMCP {
		spec = spec.WithMCPServer(entry.Name, flow.MCPServer{
			Command: entry.Command,
			Args:    []string{entry.Path},
		})
	}
	return spec, nil
}

func isMarkdownPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".md")
}
