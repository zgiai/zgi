package container

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// A model cannot create this context claim. The gateway creates it only after
// the journal has atomically claimed its one dispatch. Re-read durable state
// and compare the complete request before consuming the existing scoped grant.
func (s *ConnectorBackgroundAuthorization) authorizeWorkJournalClaim(ctx context.Context, restored ActiveBackgroundTaskRun, request execution.BackgroundAuthorizationRequest, input map[string]any) error {
	claim, ok := ctx.Value(workJournalClaimContextKey{}).(workJournalClaim)
	if !ok || claim.OperationID == uuid.Nil || s.resolver == nil || s.resolver.db == nil || restored.SocialOperations {
		return backgroundDenied(nil)
	}
	digest, err := backgroundInputDigest(input)
	if err != nil || digest != claim.InputDigest {
		return backgroundDenied(err)
	}
	var op workjournal.Operation
	err = s.resolver.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ?", claim.OperationID, restored.Run.OrganizationID, restored.WorkspaceID, restored.OwnerID, restored.Run.DigitalHumanID).Take(&op).Error
	if err != nil {
		return backgroundDenied(err)
	}
	r := request.Request
	if op.State != workjournal.StateExecuting || op.Attempts != 1 || op.EmployeeVersionID != restored.Run.DigitalHumanVersionID || op.ConnectionID != request.Connection.ID || op.ConnectionID != r.ConnectionID || op.ConnectorID != r.ConnectorID.String() || op.OperationID != r.OperationID.String() || op.LogicalID() != r.LogicalOperationID {
		return backgroundDenied(nil)
	}
	var frozen map[string]any
	d := json.NewDecoder(bytes.NewReader(op.Input))
	d.UseNumber()
	if d.Decode(&frozen) != nil {
		return backgroundDenied(nil)
	}
	frozenDigest, err := backgroundInputDigest(frozen)
	if err != nil || frozenDigest != digest {
		return backgroundDenied(err)
	}
	return nil
}
