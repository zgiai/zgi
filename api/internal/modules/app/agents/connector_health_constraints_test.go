package agents

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
	"time"
)

func TestAgentConnectorHealthIntersectsConnectionAndSnapshotAuthority(t *testing.T) {
	owner, targetID := uuid.New(), uuid.NewString()
	now := time.Now().UTC()
	for _, restriction := range []string{"ready", "connection-operation", "binding-operation", "expired", "foreign-owner", "different-version", "deny", "missing"} {
		t.Run(restriction, func(t *testing.T) {
			binding := core.TargetBinding{OwnerAccountID: owner, Target: core.TargetRef{Type: core.TargetTypeAgent, ID: targetID, Scope: core.ScopePublished, VersionID: uuid.NewString()}, Constraint: core.OwnerConstraint(owner, []core.Identifier{"github.user.get"})}
			connection := core.PersonalConnection{Constraint: core.OwnerConstraint(owner, []core.Identifier{"github.user.get"})}
			switch restriction {
			case "connection-operation":
				connection.Constraint.AllowedOperationIDs = []core.Identifier{"github.repo.list"}
			case "binding-operation":
				binding.Constraint.AllowedOperationIDs = []core.Identifier{}
			case "expired":
				expiry := now.Add(-time.Second)
				connection.Constraint.ExpiresAt = &expiry
			case "foreign-owner":
				connection.Constraint.AudienceOwner = uuid.New()
			case "different-version":
				binding.Constraint.TargetBoundaries = []core.TargetBoundary{{Type: core.TargetTypeAgent, ID: targetID, Scope: "published", VersionID: uuid.NewString()}}
			case "deny":
				binding.Constraint.DenyReasons = []string{"source-revoked"}
			case "missing":
				binding.Constraint = nil
			}
			if got := hasUsableAgentConnectorOperation(binding, connection, now); got != (restriction == "ready") {
				t.Fatalf("usable=%t", got)
			}
		})
	}
}
