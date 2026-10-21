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

type ActionPlanStep struct {
	ID          uuid.UUID       `json:"id"`
	ConnectorID core.Identifier `json:"connector_id"`
	OperationID core.Identifier `json:"operation_id"`
}
type ActionPlan struct {
	ShapeDigest string           `json:"shape_digest"`
	Steps       []ActionPlanStep `json:"steps"`
}

// PlanPhaseRef describes a persisted host plan, not execution authorization.
// Phase labels are advisory; server UUIDs and the ordered operation shape own
// action identity. Binding authorization is resolved separately at each call.
type PlanPhaseRef struct {
	ID                       string
	ConnectorID, OperationID core.Identifier
}
type actionPhaseContextKey struct{}

func WithActionPlanPhase(ctx context.Context, phaseID string) context.Context {
	return context.WithValue(ctx, actionPhaseContextKey{}, strings.TrimSpace(phaseID))
}
func ActionPlanPhaseFromContext(ctx context.Context) string {
	value, _ := ctx.Value(actionPhaseContextKey{}).(string)
	return value
}

func planShape(refs []PlanPhaseRef) string {
	shape := make([][2]string, 0, len(refs))
	for _, ref := range refs {
		shape = append(shape, [2]string{ref.ConnectorID.String(), ref.OperationID.String()})
	}
	encoded, _ := json.Marshal(shape)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func ResolveActionPlanStep(ledger *ActionLedger, refs []PlanPhaseRef, phaseID string, identity Identity) (string, bool, error) {
	fail := func() (string, bool, error) {
		return "", false, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	if ledger == nil || ledger.Blocked || len(refs) > 16 || !validActionPlans(ledger.Plans) {
		return fail()
	}
	intent := ledger.IntentID
	if intent == "" {
		intent = "turn"
	}
	if len(refs) == 0 {
		if phaseID != "" {
			return fail()
		}
		if _, exists := ledger.Plans[intent]; exists {
			return fail()
		}
		return intent, false, nil
	}
	selected := -1
	seen := map[string]bool{}
	for i, ref := range refs {
		if strings.TrimSpace(ref.ID) == "" || seen[ref.ID] || !ref.ConnectorID.Valid() || !ref.OperationID.Valid() {
			return fail()
		}
		seen[ref.ID] = true
		if ref.ID == phaseID && ref.ConnectorID == identity.ConnectorID && ref.OperationID == identity.OperationID {
			selected = i
		}
	}
	if selected < 0 {
		return fail()
	}
	shape := planShape(refs)
	if plan, exists := ledger.Plans[intent]; exists {
		if plan.ShapeDigest != shape || len(plan.Steps) != len(refs) {
			return fail()
		}
		ids := map[uuid.UUID]bool{}
		for i, step := range plan.Steps {
			if step.ID == uuid.Nil || ids[step.ID] || step.ConnectorID != refs[i].ConnectorID || step.OperationID != refs[i].OperationID {
				return fail()
			}
			ids[step.ID] = true
		}
		return plan.Steps[selected].ID.String(), false, nil
	}
	// Installing a new model plan after an unplanned action cannot mint another
	// identity for the same user turn. A new user intent creates a new ledger key.
	for _, instance := range ledger.Instances {
		if instance.StepID == intent {
			return fail()
		}
	}
	plan := ActionPlan{ShapeDigest: shape, Steps: make([]ActionPlanStep, 0, len(refs))}
	for _, ref := range refs {
		plan.Steps = append(plan.Steps, ActionPlanStep{ID: uuid.New(), ConnectorID: ref.ConnectorID, OperationID: ref.OperationID})
	}
	if ledger.Plans == nil {
		ledger.Plans = map[string]ActionPlan{}
	}
	ledger.Plans[intent] = plan
	return plan.Steps[selected].ID.String(), true, nil
}

func validActionPlans(plans map[string]ActionPlan) bool {
	if len(plans) > 1024 {
		return false
	}
	ids := map[uuid.UUID]bool{}
	for intent, plan := range plans {
		if intent != "turn" {
			id, err := uuid.Parse(intent)
			if err != nil || id == uuid.Nil {
				return false
			}
		}
		if len(plan.Steps) == 0 || len(plan.Steps) > 16 {
			return false
		}
		refs := make([]PlanPhaseRef, 0, len(plan.Steps))
		for _, step := range plan.Steps {
			if step.ID == uuid.Nil || ids[step.ID] || !step.ConnectorID.Valid() || !step.OperationID.Valid() {
				return false
			}
			ids[step.ID] = true
			refs = append(refs, PlanPhaseRef{ConnectorID: step.ConnectorID, OperationID: step.OperationID})
		}
		if plan.ShapeDigest != planShape(refs) {
			return false
		}
	}
	return true
}
