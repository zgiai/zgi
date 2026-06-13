package sql_base

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
)

func TestCheckSQLGuardLoadsPolicyFromProvider(t *testing.T) {
	policy := guard.DefaultPolicy()
	policy.Mode = guard.ModeEnforce
	policy.Readonly = true

	result, guarded, err := checkSQLGuard(
		context.Background(),
		"INSERT INTO users(id) VALUES (1)",
		&audit.Context{DataSourceID: "ds-1"},
		func(ctx context.Context, dataSourceID string) (*guard.Policy, error) {
			if dataSourceID != "ds-1" {
				t.Fatalf("dataSourceID = %s, want ds-1", dataSourceID)
			}
			return &policy, nil
		},
	)
	if err == nil {
		t.Fatal("expected guard denied error")
	}
	if !guarded {
		t.Fatal("expected guard to run")
	}
	if result.Action != guard.ActionDeny {
		t.Fatalf("action = %s, want deny", result.Action)
	}
}

func TestCheckSQLGuardPrefersAuditContextPolicy(t *testing.T) {
	contextPolicy := guard.DefaultPolicy()
	contextPolicy.Mode = guard.ModeWarn

	result, guarded, err := checkSQLGuard(
		context.Background(),
		"SELECT 1",
		&audit.Context{DataSourceID: "ds-1", GuardPolicy: &contextPolicy},
		func(ctx context.Context, dataSourceID string) (*guard.Policy, error) {
			t.Fatal("provider should not be called when audit context has policy")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("check guard: %v", err)
	}
	if !guarded {
		t.Fatal("expected guard to run")
	}
	if result.Policy.Mode != guard.ModeWarn {
		t.Fatalf("mode = %s, want warn", result.Policy.Mode)
	}
}
