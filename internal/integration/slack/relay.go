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
	spec = spec.WithFile(flow.File{
		Path:    askRelayPath,
		Mode:    "0755",
		Content: base64.StdEncoding.EncodeToString(askRelayScript),
	})
	spec = spec.WithMCPServer("kman-ask", flow.MCPServer{
		Command: "python3",
		Args:    []string{askRelayPath},
		Env: map[string]string{
			"KMAN_RUN_ID":          runID,
			"KMAN_FLOW_ID":         spec.Name,
			"KMAN_SLACK_CHANNEL":   channel,
			"KMAN_SLACK_THREAD_TS": threadTS,
		},
	})
	spec = spec.WithDisallowedTool("AskUserQuestion")
	return spec
}
