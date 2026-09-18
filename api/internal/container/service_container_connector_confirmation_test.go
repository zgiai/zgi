package container

import (
	"context"
	"testing"

	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorruntimeconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/gorm"
)

// TestConnectorDigitalHumanContinuationGuardWiringFailsClosed pins that the
// production container builds the durable DigitalHuman guard from its database
// and that an unwired or malformed gate fails closed. It proves the gate is
// wired, not merely declared.
func TestConnectorDigitalHumanContinuationGuardWiringFailsClosed(t *testing.T) {
	unconfigured := &ServiceContainer{}
	if guard := unconfigured.getConnectorDigitalHumanContinuationGuard(); guard != nil {
		t.Fatalf("missing configuration exposed DigitalHuman guard: %#v", guard)
	}

	withoutDatabase := &ServiceContainer{config: connectorOnlyTestConfig()}
	if guard := withoutDatabase.getConnectorDigitalHumanContinuationGuard(); guard != nil {
		t.Fatalf("missing database exposed DigitalHuman guard: %#v", guard)
	}

	configured := &ServiceContainer{config: connectorOnlyTestConfig(), db: &gorm.DB{}}
	guard := configured.getConnectorDigitalHumanContinuationGuard()
	if guard == nil {
		t.Fatal("configured container did not build the DigitalHuman guard")
	}
	if err := guard.AuthorizeContinuation(
		context.Background(),
		connectorruntimeconfirmation.DigitalHumanContinuationGuardRequest{},
	); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("malformed guard request error = %v, want conflict", err)
	}
}
