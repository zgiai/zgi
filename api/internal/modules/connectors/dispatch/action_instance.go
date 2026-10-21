package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ActionInstanceResolver commits the host's action identity before dispatch.
// Input is hashed in memory; neither arguments nor credentials belong in the
// host ledger. Implementations must serialize allocation and preserve the
// ledger across every subsequent host metadata write.
type ActionInstanceResolver func(context.Context, Identity, any) (Identity, error)

const ActionInstancesMetadataKey = "connector_action_instances"

type ActionLedger struct {
	Version   int                       `json:"version"`
	Blocked   bool                      `json:"blocked,omitempty"`
	IntentID  string                    `json:"intent_id,omitempty"`
	Instances map[string]ActionInstance `json:"instances"`
	Plans     map[string]ActionPlan     `json:"plans,omitempty"`
}

type ActionInstance struct {
	ID                 uuid.UUID       `json:"id"`
	StepID             string          `json:"step_id"`
	OrganizationID     uuid.UUID       `json:"organization_id"`
	OwnerAccountID     uuid.UUID       `json:"owner_account_id"`
	BindingID          uuid.UUID       `json:"binding_id"`
	ConnectionID       uuid.UUID       `json:"connection_id"`
	ConnectorID        core.Identifier `json:"connector_id"`
	OperationID        core.Identifier `json:"operation_id"`
	Target             core.TargetRef  `json:"target"`
	InputDigest        string          `json:"input_digest"`
	LogicalOperationID string          `json:"logical_operation_id"`
	CorrelationID      string          `json:"correlation_id"`
}

// ResolveActionInstance operates on an already locked host ledger. stepID is a
// durable server step, never a model tool-call ID or a model-authored phase name.
// The caller must commit a newly allocated entry before using the returned ID.
func ResolveActionInstance(ledger *ActionLedger, stepID, correlationID string, identity Identity, input any) (Identity, bool, error) {
	fail := func() (Identity, bool, error) {
		return Identity{}, false, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	if ledger == nil || ledger.Blocked || (ledger.Version != 0 && ledger.Version != 1) || strings.TrimSpace(stepID) == "" || len(stepID) > 2048 || strings.TrimSpace(correlationID) == "" || validateIdentity(identity) != nil {
		return fail()
	}
	if !validActionLedger(ledger, identity, correlationID) {
		return fail()
	}
	data, err := json.Marshal(input)
	if err != nil || len(data) > 4<<20 {
		clear(data)
		return fail()
	}
	digest := sha256.Sum256(data)
	clear(data)
	inputDigest := hex.EncodeToString(digest[:])
	target := identity.Target
	target.Scope = target.EffectiveScope()
	key := actionInstanceKey(stepID, identity.ConnectorID, identity.OperationID, target, inputDigest)
	if entry, ok := ledger.Instances[key]; ok {
		if entry.BindingID != identity.BindingID || entry.ConnectionID != identity.ConnectionID {
			return fail()
		}
		identity.LogicalOperationID, identity.CorrelationID = entry.LogicalOperationID, entry.CorrelationID
		identity.ActionStepID = entry.StepID
		return identity, false, nil
	}
	if len(ledger.Instances) >= 1024 {
		return fail()
	}
	// A committed plan step owns one business invocation. Changing its input,
	// target or operation cannot manufacture another execution of that step.
	for _, plan := range ledger.Plans {
		for _, step := range plan.Steps {
			if step.ID.String() != stepID {
				continue
			}
			for _, existing := range ledger.Instances {
				if existing.StepID == stepID {
					return fail()
				}
			}
		}
	}
	id := uuid.New()
	identity.LogicalOperationID = "connector-operation-" + id.String()
	identity.CorrelationID = correlationID
	identity.ActionStepID = stepID
	if ledger.Instances == nil {
		ledger.Instances = make(map[string]ActionInstance)
	}
	ledger.Version = 1
	ledger.Instances[key] = ActionInstance{
		ID: id, StepID: stepID, OrganizationID: identity.OrganizationID, OwnerAccountID: identity.OwnerAccountID,
		BindingID: identity.BindingID, ConnectionID: identity.ConnectionID, ConnectorID: identity.ConnectorID,
		OperationID: identity.OperationID, Target: target, InputDigest: inputDigest,
		LogicalOperationID: identity.LogicalOperationID, CorrelationID: correlationID,
	}
	return identity, true, nil
}

func actionInstanceKey(step string, connector, operation core.Identifier, target core.TargetRef, input string) string {
	target.Scope = target.EffectiveScope()
	coordinates, _ := json.Marshal(struct {
		Step                 string
		Connector, Operation core.Identifier
		Target               core.TargetRef
		InputDigest          string
	}{step, connector, operation, target, input})
	keyDigest := sha256.Sum256(coordinates)
	return hex.EncodeToString(keyDigest[:])
}

// Validate every association before allocating another action. Checking only
// the requested entry lets a corrupted or re-keyed ledger bypass replay guards.
func validActionLedger(ledger *ActionLedger, identity Identity, correlation string) bool {
	if !validActionPlans(ledger.Plans) {
		return false
	}
	if len(ledger.Instances) > 1024 || (ledger.Version == 0 && (len(ledger.Instances) != 0 || ledger.IntentID != "")) {
		return false
	}
	if ledger.IntentID != "" {
		id, err := uuid.Parse(ledger.IntentID)
		if err != nil || id == uuid.Nil {
			return false
		}
	}
	ids := make(map[uuid.UUID]bool, len(ledger.Instances))
	for key, entry := range ledger.Instances {
		digest, err := hex.DecodeString(entry.InputDigest)
		if err != nil || len(digest) != sha256.Size || entry.InputDigest != hex.EncodeToString(digest) ||
			entry.ID == uuid.Nil || ids[entry.ID] || strings.TrimSpace(entry.StepID) == "" || len(entry.StepID) > 2048 ||
			entry.OrganizationID != identity.OrganizationID || entry.OwnerAccountID != identity.OwnerAccountID ||
			entry.BindingID == uuid.Nil || entry.ConnectionID == uuid.Nil || !entry.ConnectorID.Valid() || !entry.OperationID.Valid() ||
			entry.Target.Validate() != nil || entry.CorrelationID != correlation ||
			entry.LogicalOperationID != "connector-operation-"+entry.ID.String() ||
			key != actionInstanceKey(entry.StepID, entry.ConnectorID, entry.OperationID, entry.Target, entry.InputDigest) {
			return false
		}
		ids[entry.ID] = true
	}
	return true
}
