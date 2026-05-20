package runner

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func newBackendFromConfig(cfg config.Config) (backend, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.RuntimeBackend)) {
	case "", "preview", "process":
		return newProcessBackend(), nil
	case "linux-secure":
		return newLinuxSecureBackend(cfg)
	default:
		return nil, fmt.Errorf("unsupported runtime backend: %s", cfg.RuntimeBackend)
	}
}
