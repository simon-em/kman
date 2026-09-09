package cli

import (
	"fmt"

	"github.com/simon-em/kman/internal/config"
)

func pushConfigOrWarn(env Env, home string) {
	if _, err := config.PushIfConfigured(home); err != nil {
		fmt.Fprintf(env.Stderr, "kman: warning: saved locally, but could not push the config repo: %v\n", err)
	}
}
