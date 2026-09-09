package flow

func (s Spec) WithFile(f File) Spec {
	for _, existing := range s.Files {
		if existing.Path == f.Path {
			return s
		}
	}
	out := s
	out.Files = append(append([]File{}, s.Files...), f)
	return out
}

func (s Spec) WithMCPServer(name string, srv MCPServer) Spec {
	out := s
	out.Steps = make([]Step, len(s.Steps))
	copy(out.Steps, s.Steps)
	for i, st := range out.Steps {
		if st.Claude == "" {
			continue
		}
		servers := map[string]MCPServer{}
		for k, v := range st.MCPServers {
			servers[k] = v
		}
		servers[name] = srv
		st.MCPServers = servers
		out.Steps[i] = st
	}
	return out
}

func (s Spec) WithDisallowedTool(tool string) Spec {
	out := s
	out.Steps = make([]Step, len(s.Steps))
	copy(out.Steps, s.Steps)
	for i, st := range out.Steps {
		if st.Claude == "" || stringsContain(st.DisallowedTools, tool) {
			continue
		}
		st.DisallowedTools = append(append([]string{}, st.DisallowedTools...), tool)
		out.Steps[i] = st
	}
	return out
}

func stringsContain(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
