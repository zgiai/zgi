package connections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const operationPolicyDecision = "connection_operation_policy_updated"

type OperationPolicyCommand struct {
	OrganizationID, OwnerAccountID, ConnectionID uuid.UUID
	ExpectedRevision                             int
	OperationIDs                                 []string
	IdempotencyKey                               string `json:"-"`
}
type OperationPolicyResult struct {
	ConnectionID uuid.UUID `json:"connection_id"`
	Revision     int       `json:"revision"`
	OperationIDs []string  `json:"operation_ids"`
}
type operationPolicyReceipt struct {
	Fingerprint string                `json:"fingerprint"`
	Result      OperationPolicyResult `json:"result"`
}
type operationPolicyRepository interface {
	GetOperationPolicyReceipt(context.Context, uuid.UUID, uuid.UUID, string, string) (OperationPolicyResult, bool, error)
	UpdateOperationPolicy(context.Context, core.PersonalConnection, int, string, string) (OperationPolicyResult, error)
}

func (s *MaintenanceService) UpdateOperationPolicy(ctx context.Context, command OperationPolicyCommand) (OperationPolicyResult, error) {
	if err := s.available(); err != nil {
		return OperationPolicyResult{}, err
	}
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || command.ConnectionID == uuid.Nil || command.ExpectedRevision < 1 || command.OperationIDs == nil || len(command.OperationIDs) > 64 {
		return OperationPolicyResult{}, invalidInputError("operation policy")
	}
	repo, ok := s.repository.(operationPolicyRepository)
	if !ok {
		return OperationPolicyResult{}, internalError("operation policy repository")
	}
	key, err := s.hasher.IdempotencyKeyHash(command.OrganizationID, command.OwnerAccountID, command.IdempotencyKey)
	if err != nil {
		return OperationPolicyResult{}, err
	}
	ids := slices.Clone(command.OperationIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if !core.Identifier(id).Valid() || i > 0 && ids[i-1] == id {
			return OperationPolicyResult{}, invalidInputError("operation policy operation")
		}
	}
	raw, _ := json.Marshal(struct {
		ConnectionID uuid.UUID
		Revision     int
		Operations   []string
	}{command.ConnectionID, command.ExpectedRevision, ids})
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	// Scope is checked even for a receipt. A prior command never grants access
	// to a connection that has since been removed or belongs to another owner.
	c, err := s.repository.GetConnection(ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID)
	if err != nil {
		return OperationPolicyResult{}, err
	}
	if result, found, err := repo.GetOperationPolicyReceipt(ctx, command.OrganizationID, command.OwnerAccountID, key, fingerprint); found || err != nil {
		return result, err
	}
	if c.Revision != command.ExpectedRevision {
		return OperationPolicyResult{}, conflictError("operation policy revision")
	}
	d, ok := s.catalog.Get(c.ConnectorID.String())
	if !ok || (d.ID != "wecom" && d.ID != "weixin") || d.ProviderID != c.ProviderID.String() || d.DriverID != c.DriverID.String() || d.AuthMethodID != c.AuthMethodID.String() || c.State != core.ReadinessReady || c.Constraint.Validate() != nil {
		return OperationPolicyResult{}, invalidInputError("operation policy connection")
	}
	for _, id := range ids {
		known := false
		for _, op := range d.Operations {
			if op.ID == id {
				known = true
			}
		}
		if !known || !slices.Contains(c.UsableOperationIDs, core.Identifier(id)) {
			return OperationPolicyResult{}, invalidInputError("operation policy capability")
		}
	}
	constraint := *c.Constraint
	constraint.AllowedOperationIDs = make([]core.Identifier, 0, len(ids))
	for _, id := range ids {
		constraint.AllowedOperationIDs = append(constraint.AllowedOperationIDs, core.Identifier(id))
	}
	c.Constraint = &constraint
	return repo.UpdateOperationPolicy(ctx, c, command.ExpectedRevision, key, fingerprint)
}

func (r *GormRepository) GetOperationPolicyReceipt(ctx context.Context, org, owner uuid.UUID, key, fingerprint string) (OperationPolicyResult, bool, error) {
	var record connectionLifecycleDecisionRecord
	err := r.db.WithContext(ctx).Where("organization_id = ? AND owner_account_id = ? AND correlation_id = ? AND decision = ? AND status = ?", org, owner, "connection-policy:"+key, operationPolicyDecision, connectionLifecycleSuccessful).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return OperationPolicyResult{}, false, nil
	}
	if err != nil {
		return OperationPolicyResult{}, false, internalError("read operation policy receipt")
	}
	var receipt operationPolicyReceipt
	if json.Unmarshal(record.Details, &receipt) != nil {
		return OperationPolicyResult{}, false, internalError("decode operation policy receipt")
	}
	if receipt.Fingerprint != fingerprint {
		return OperationPolicyResult{}, true, conflictError("operation policy replay")
	}
	return receipt.Result, true, nil
}
func (r *GormRepository) UpdateOperationPolicy(ctx context.Context, c core.PersonalConnection, expected int, key, fingerprint string) (OperationPolicyResult, error) {
	var result OperationPolicyResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		updated, err := nested.UpdateConnection(ctx, c.OrganizationID, c.OwnerAccountID, c, expected)
		if err != nil {
			return err
		}
		result = OperationPolicyResult{ConnectionID: c.ID, Revision: updated.Revision, OperationIDs: []string{}}
		for _, id := range updated.Constraint.AllowedOperationIDs {
			result.OperationIDs = append(result.OperationIDs, id.String())
		}
		raw, err := json.Marshal(operationPolicyReceipt{fingerprint, result})
		if err != nil {
			return internalError("encode operation policy receipt")
		}
		decision := connectionLifecycleDecisionRecord{ID: uuid.New(), OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ActingPrincipalID: c.OwnerAccountID, ConnectorID: c.ConnectorID.String(), ConnectionID: c.ID, Decision: operationPolicyDecision, Status: connectionLifecycleSuccessful, CorrelationID: "connection-policy:" + key, Details: datatypes.JSON(raw), CreatedAt: time.Now().UTC()}
		return createOrVerifyConnectionLifecycleDecision(ctx, tx, decision)
	})
	if err != nil {
		if receipt, found, readErr := r.GetOperationPolicyReceipt(ctx, c.OrganizationID, c.OwnerAccountID, key, fingerprint); found || readErr != nil {
			return receipt, readErr
		}
		return OperationPolicyResult{}, err
	}
	return result, nil
}
