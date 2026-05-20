package platform_test

import (
	"context"
	"testing"

	"github.com/zgiai/ginext/internal/infra/platform"
	"github.com/zgiai/ginext/internal/infra/platform/billing"
	"github.com/zgiai/ginext/internal/infra/platform/channel"
	"github.com/zgiai/ginext/internal/infra/platform/identity"
)

func TestBillingStandalone_PreCheck(t *testing.T) {
	standalone := billing.NewStandaloneBilling()

	allowed, reason, err := standalone.PreCheck(context.Background(), "tenant-1", "gpt-4", "openai")
	if err != nil {
		t.Fatalf("PreCheck should not return error, got: %v", err)
	}
	if !allowed {
		t.Error("PreCheck should return allowed=true in standalone mode")
	}
	if reason != "standalone mode allowed" {
		t.Errorf("Expected reason 'standalone mode allowed', got: %s", reason)
	}
}

func TestBillingStandalone_RecordUsage(t *testing.T) {
	standalone := billing.NewStandaloneBilling()

	usage := billing.Usage{
		Model:            "gpt-4",
		Provider:         "openai",
		PromptTokens:     100,
		CompletionTokens: 50,
		RequestID:        "req-123",
	}

	err := standalone.RecordUsage(context.Background(), "tenant-1", usage)
	if err != nil {
		t.Fatalf("RecordUsage should not return error, got: %v", err)
	}
}

func TestIdentityStandalone_NoAdminPass(t *testing.T) {
	setPlatformTestConfig(t, "SELF_HOSTED", "", "")
	standalone := identity.NewStandalone()

	ctx, err := standalone.Identify(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("Identify should not return error when no admin pass set, got: %v", err)
	}
	if ctx.TenantID != "default" {
		t.Errorf("Expected TenantID 'default', got: %s", ctx.TenantID)
	}
	if ctx.Role != "user" {
		t.Errorf("Expected Role 'user', got: %s", ctx.Role)
	}
}

func TestIdentityStandalone_ValidToken(t *testing.T) {
	setPlatformTestConfig(t, "SELF_HOSTED", "secret123", "")

	standalone := identity.NewStandalone()

	ctx, err := standalone.Identify(context.Background(), "secret123")
	if err != nil {
		t.Fatalf("Identify should not return error for valid token, got: %v", err)
	}
	if ctx.Role != "admin" {
		t.Errorf("Expected Role 'admin', got: %s", ctx.Role)
	}
}

func TestIdentityStandalone_InvalidToken(t *testing.T) {
	setPlatformTestConfig(t, "SELF_HOSTED", "secret123", "")

	standalone := identity.NewStandalone()

	_, err := standalone.Identify(context.Background(), "wrong-token")
	if err == nil {
		t.Error("Identify should return error for invalid token")
	}
}

func TestChannelStandalone_ListChannels(t *testing.T) {
	standalone := channel.NewStandalone(nil)

	channels, err := standalone.ListChannels(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("ListChannels should not return error, got: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("Standalone should return empty list, got: %d channels", len(channels))
	}
}

func TestContainerStandalone(t *testing.T) {
	setPlatformTestConfig(t, "SELF_HOSTED", "", "")

	container, err := platform.NewContainer(nil)
	if err != nil {
		t.Fatalf("NewContainer should not return error, got: %v", err)
	}
	if container == nil {
		t.Fatal("Container should not be nil")
	}
	if container.Billing == nil {
		t.Error("Billing provider should not be nil")
	}
	if container.Identity == nil {
		t.Error("Identity provider should not be nil")
	}
	if container.Channel == nil {
		t.Error("Channel provider should not be nil")
	}

	// Verify Billing works
	allowed, _, err := container.Billing.PreCheck(context.Background(), "t1", "gpt-4", "openai")
	if err != nil || !allowed {
		t.Error("Billing.PreCheck should allow in standalone mode")
	}

	// Verify Channel returns empty
	channels, _ := container.Channel.ListChannels(context.Background(), "t1")
	if len(channels) != 0 {
		t.Error("Channel.ListChannels should return empty in standalone mode")
	}
}
