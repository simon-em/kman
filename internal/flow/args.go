package flow

import "fmt"

func ResolveArgs(s Spec, provided map[string]string) (map[string]string, error) {
	resolved := map[string]string{}
	for name, arg := range s.Args {
		if v, ok := provided[name]; ok {
			resolved[name] = v
			continue
		}
		if arg.Default != "" {
			resolved[name] = arg.Default
			continue
		}
		if arg.Required {
			return nil, fmt.Errorf("missing required arg %q", name)
		}
	}
	return resolved, nil
}
