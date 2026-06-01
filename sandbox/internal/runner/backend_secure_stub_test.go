//go:build !linux

package runner

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func TestLinuxSecureBackendRejectsUnsupportedPlatform(t *testing.T) {
	_, err := NewServiceFromConfig(config.Config{
		RuntimeBackend: "linux-secure",
		SecureRootFS:   "/srv/zgi-sandbox/rootfs",
	})
	if err == nil || !strings.Contains(err.Error(), "only available on linux") {
		t.Fatalf("expected unsupported platform rejection, got %v", err)
	}
}
