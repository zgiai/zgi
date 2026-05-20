package platform_test

import (
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform"
)

// TestContainerCloudMode tests container initialization in Cloud mode.
// This is a mock test since we don't have a real gRPC server in tests.
func TestContainerCloudMode(t *testing.T) {
	setPlatformTestConfig(t, "CLOUD", "", "localhost:50051")

	container, err := platform.NewContainer(nil)
	if err != nil {
		t.Fatalf("NewContainer should not return error in Cloud mode, got: %v", err)
	}
	if container == nil {
		t.Fatal("Container should not be nil")
	}

	// Note: In current implementation, Cloud mode still uses Standalone providers
	// until gRPC implementations are fully configured. This is by design for safety.
	// When gRPC is properly connected, these would use Remote implementations.

	if container.Billing == nil {
		t.Error("Billing provider should not be nil")
	}
	if container.Identity == nil {
		t.Error("Identity provider should not be nil")
	}
	if container.Channel == nil {
		t.Error("Channel provider should not be nil")
	}
}

// TestContainerSelfHostedMode tests container in SELF_HOSTED mode.
func TestContainerSelfHostedMode(t *testing.T) {
	setPlatformTestConfig(t, "SELF_HOSTED", "", "")

	container, err := platform.NewContainer(nil)
	if err != nil {
		t.Fatalf("NewContainer should not return error, got: %v", err)
	}
	if container == nil {
		t.Fatal("Container should not be nil")
	}

	// SELF_HOSTED mode should use Standalone implementations
	if container.Billing == nil {
		t.Error("Billing provider should not be nil")
	}
	if container.Identity == nil {
		t.Error("Identity provider should not be nil")
	}
	if container.Channel == nil {
		t.Error("Channel provider should not be nil")
	}
}

func setPlatformTestConfig(t *testing.T, edition, adminPass, grpcAddr string) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{
			Edition:   edition,
			AdminPass: adminPass,
		},
		Console: config.ConsoleConfig{
			GRPCAddr: grpcAddr,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
