package slack

import (
	_ "embed"
	"encoding/base64"

	"github.com/simon-em/kman/internal/flow"
)

const AskMeta = "ask"

const askRelayPath = "/kman/tools/kman-ask.py"

//go:embed assets/kman-ask.py
var askRelayScript []byte

func InjectAskRelay(spec flow.Spec, runID, channel, threadTS string) flow.Spec {
	spec.Files = append(append([]flow.File{}, spec.Files...), flow.File{
		Path:    askRelayPath,
		Mode:    "0755",
		Content: base64.StdEncoding.EncodeToString(askRelayScript),
	})

	steps := make([]flow.Step, len(spec.Steps))
	copy(steps, spec.Steps)
	for i, st := range steps {
		if st.Claude == "" {
			continue
		}
		servers := map[string]flow.MCPServer{}
		for name, srv := range st.MCPServers {
			servers[name] = srv
		}
		servers["kman-ask"] = flow.MCPServer{
			Command: "python3",
			Args:    []string{askRelayPath},
			Env: map[string]string{
				"KMAN_RUN_ID":          runID,
				"KMAN_FLOW_ID":         spec.Name,
				"KMAN_SLACK_CHANNEL":   channel,
				"KMAN_SLACK_THREAD_TS": threadTS,
			},
		}
		st.MCPServers = servers
		if !containsString(st.DisallowedTools, "AskUserQuestion") {
			st.DisallowedTools = append(append([]string{}, st.DisallowedTools...), "AskUserQuestion")
		}
		steps[i] = st
	}
	spec.Steps = steps
	return spec
}

func containsString(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
