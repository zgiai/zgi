package runtimeworkspace

import (
	"os"
	"strings"
)

func RuntimeEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_RUNTIME_WORKSPACE_ENABLED")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
