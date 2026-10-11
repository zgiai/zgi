package gateway

import (
	"os"
	"testing"

	"github.com/zgiai/zgi/api/config"
)

// TestMain pins a minimal app config for the package. Without one,
// config.Current() loads the process environment and panics on branches that
// require connector credential keys; the panic aborts the test binary, so
// every test after the first one to reach the config silently never runs.
// Tests that need specific settings still set and restore their own config.
func TestMain(m *testing.M) {
	if config.GlobalConfig == nil {
		config.GlobalConfig = &config.Config{}
	}
	os.Exit(m.Run())
}
