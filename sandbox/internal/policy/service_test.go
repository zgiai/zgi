package policy

import (
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

func TestNormalizeCreateClampsTTLAndRejectsDeniedNetwork(t *testing.T) {
	cfg := config.FromEnv()
	cfg.SessionTTL = 120
	service := NewService(cfg)

	decision, err := service.NormalizeCreate("session", 999, false, "", "stdlib", 0)
	if err != nil {
		t.Fatalf("expected normalize create, got %v", err)
	}
	if decision.TTL.Seconds() != 120 {
		t.Fatalf("expected ttl clamp to 120 seconds, got %.0f", decision.TTL.Seconds())
	}

	if _, err := service.NormalizeCreate("session", 60, true, "deny-by-default", "stdlib", 0); err == nil {
		t.Fatal("expected denied network policy to reject outbound access")
	}
}

func TestNormalizeCommandLimitsUsesProfileAndClampsRequest(t *testing.T) {
	cfg := config.FromEnv()
	cfg.CommandTimeout = 10
	cfg.OutputLimitKB = 512
	service := NewService(cfg)

	limits, err := service.NormalizeCommandLimits("skill-python", 60, 0, 2048, 2048)
	if err != nil {
		t.Fatalf("expected command limits, got %v", err)
	}
	if limits.Profile != "skill-python" {
		t.Fatalf("unexpected profile: %s", limits.Profile)
	}
	if limits.Timeout != 10*time.Second {
		t.Fatalf("expected timeout clamp to 10s, got %s", limits.Timeout)
	}
	if limits.StdoutLimitBytes != 512*1024 || limits.StderrLimitBytes != 512*1024 {
		t.Fatalf("expected output limits to clamp to config cap, got stdout=%d stderr=%d", limits.StdoutLimitBytes, limits.StderrLimitBytes)
	}

	if _, err := service.NormalizeCommandLimits("unknown", 0, 0, 0, 0); err == nil {
		t.Fatal("expected unknown command profile to be rejected")
	}
}

func TestValidateCodeExecutionRejectsUnauthorizedNetwork(t *testing.T) {
	service := NewService(config.FromEnv())
	box := sandbox.Sandbox{
		NetworkEnabled: false,
		NetworkPolicy:  "deny-by-default",
	}

	if err := service.ValidateCodeExecution(box, true); err == nil {
		t.Fatal("expected network validation failure")
	}
}
