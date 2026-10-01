package runtimeconfirmation

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"testing"
)

func TestNativeRecoveryRejectsUnconvertedAndMixedAuditPairs(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		request, decisions, outcome := newAuditRecoveryFixture(confirmed)
		for index := range decisions {
			decisions[index].Details["execution_path"] = "legacy"
		}
		reader := &auditRecoveryReaderStub{decisions: decisions, outcome: outcome}
		recoverer, _ := NewAuditTerminalRecoverer(reader)
		if _, err := recoverer.RecoverTerminal(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatal("retired audit was accepted by native runtime", err)
		}
		decisions[1].Details["execution_path"] = string(execution.PathConnectorNative)
		if _, err := recoverer.RecoverTerminal(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("mixed path recovery error=%v", err)
		}
	}
}
