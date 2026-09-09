package skills

import (
	"encoding/base64"
	"fmt"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/flow"
)

func Compose(spec flow.Spec) (flow.Spec, error) {
	for _, ref := range spec.Access.Skills {
		parsed, err := flow.ParseSkillRef(ref)
		if err != nil {
			return flow.Spec{}, err
		}
		switch parsed.Kind {
		case "catalog":
			composed, err := composeCatalog(spec, parsed)
			if err != nil {
				return flow.Spec{}, err
			}
			spec = composed
		case "local":
			spec = spec.WithMCPServer(parsed.Name, flow.MCPServer{Command: parsed.Path})
		}
	}
	return spec, nil
}

func composeCatalog(spec flow.Spec, ref flow.SkillRef) (flow.Spec, error) {
	entry, ok := catalog.Get(ref.Name)
	if !ok {
		return flow.Spec{}, fmt.Errorf("skill %q: no such catalog entry %q", "catalog:"+ref.Name+"@"+ref.Ref, ref.Name)
	}
	if entry.Ref != ref.Ref {
		return flow.Spec{}, fmt.Errorf("skill catalog:%s@%s: the catalog's %s entry is now pinned at %s; update the flow's access.skills entry", ref.Name, ref.Ref, ref.Name, entry.Ref)
	}
	spec = spec.WithFile(flow.File{
		Path:    entry.Path,
		Mode:    "0755",
		Content: base64.StdEncoding.EncodeToString(entry.Content),
	})
	spec = spec.WithMCPServer(entry.Name, flow.MCPServer{
		Command: entry.Command,
		Args:    []string{entry.Path},
	})
	return spec, nil
}
