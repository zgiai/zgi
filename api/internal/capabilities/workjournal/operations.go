package workjournal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

func (s *Service) Prepare(ctx context.Context, sc Scope, req PrepareRequest) (Operation, error) {
	var op Operation
	if !validScope(sc) || !validKey(req.WorkKey) || !validKey(req.ConnectorID) || !validKey(req.OperationID) || req.ConnectionID == uuid.Nil || req.EmployeeVersionID == uuid.Nil {
		return op, invalid()
	}
	input, err := canonical(req.Input, true)
	if err != nil {
		return op, err
	}
	body, _ := json.Marshal(struct {
		Scope      string
		Connector  string
		Connection uuid.UUID
		Operation  string
		Input      json.RawMessage
	}{scopeKey(sc), req.ConnectorID, req.ConnectionID, req.OperationID, json.RawMessage(input)})
	fingerprint := hash(body)
	digest := hash(append(append([]byte(nil), body...), []byte(req.EmployeeVersionID.String())...))
	now := s.now().UTC()
	op = Operation{ID: uuid.New(), Scope: sc, ScopeKey: scopeKey(sc), WorkKey: req.WorkKey, EmployeeVersionID: req.EmployeeVersionID, ConnectorID: req.ConnectorID, ConnectionID: req.ConnectionID, OperationID: req.OperationID, Input: input, Digest: digest, UnresolvedFingerprint: &fingerprint, State: StatePending, Receipt: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&op)
	if result.Error != nil {
		return Operation{}, result.Error
	}
	if result.RowsAffected == 1 {
		return op, nil
	}
	existing, err := s.GetOperationByKey(ctx, sc, req.WorkKey)
	if err == nil && existing.Digest == digest {
		return existing, nil
	}
	return Operation{}, conflict()
}
func (s *Service) GetOperation(ctx context.Context, sc Scope, id uuid.UUID) (Operation, error) {
	var op Operation
	if !validScope(sc) || id == uuid.Nil {
		return op, invalid()
	}
	err := scoped(s.db.WithContext(ctx), sc).First(&op, "id = ?", id).Error
	return op, readErr(err)
}
func (s *Service) GetOperationByKey(ctx context.Context, sc Scope, key string) (Operation, error) {
	var op Operation
	if !validScope(sc) || !validKey(key) {
		return op, invalid()
	}
	err := scoped(s.db.WithContext(ctx), sc).Where("work_key = ?", key).First(&op).Error
	return op, readErr(err)
}
func validState(state string) bool {
	return state == StatePending || state == StateExecuting || state == StateSucceeded || state == StateFailed || state == StateUnknown
}
func (s *Service) ListOperations(ctx context.Context, sc Scope, state string, after uuid.UUID, limit int) ([]Operation, error) {
	if !validScope(sc) || (state != "" && !validState(state)) {
		return nil, invalid()
	}
	q := scoped(s.db.WithContext(ctx), sc)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if after != uuid.Nil {
		q = q.Where("id > ?", after)
	}
	rows := []Operation{}
	err := q.Order("id ASC").Limit(pageLimit(limit)).Find(&rows).Error
	return rows, err
}
func (s *Service) Dispatch(ctx context.Context, sc Scope, id uuid.UUID, g Gateway) (Operation, error) {
	if g == nil {
		return Operation{}, invalid()
	}
	op, err := s.GetOperation(ctx, sc, id)
	if err != nil {
		return op, err
	}
	if op.State != StatePending {
		return op, nil
	}
	r := scoped(s.db.WithContext(ctx).Model(&Operation{}), sc).Where("id = ? AND state = ? AND attempts = 0", id, StatePending).Updates(map[string]any{"state": StateExecuting, "attempts": 1, "updated_at": s.now().UTC()})
	if r.Error != nil {
		return Operation{}, r.Error
	}
	if r.RowsAffected != 1 {
		return s.GetOperation(ctx, sc, id)
	}
	op.State = StateExecuting
	op.Attempts = 1
	result, callErr := g.Execute(ctx, op)
	return s.persistResult(ctx, sc, id, result, callErr)
}
func (s *Service) Recover(ctx context.Context, sc Scope, id uuid.UUID, g Gateway) (Operation, error) {
	if g == nil {
		return Operation{}, invalid()
	}
	op, err := s.GetOperation(ctx, sc, id)
	if err != nil {
		return op, err
	}
	if op.State != StateExecuting && op.State != StateUnknown {
		return op, nil
	}
	result, callErr := g.Recover(ctx, op)
	return s.persistResult(ctx, sc, id, result, callErr)
}
func (s *Service) persistResult(ctx context.Context, sc Scope, id uuid.UUID, result DispatchResult, callErr error) (Operation, error) {
	state := result.State
	if state != StateSucceeded && state != StateFailed && state != StateUnknown {
		state = StateUnknown
	}
	if callErr != nil && state != StateFailed {
		state = StateUnknown
	}
	receipt := result.Receipt
	if len(receipt) == 0 {
		receipt = []byte(`{}`)
	}
	normalized, err := canonical(receipt, false)
	if err != nil {
		state = StateUnknown
		normalized = []byte(`{}`)
		result.EvidenceID = nil
	}
	if result.EvidenceID != nil && *result.EvidenceID == uuid.Nil {
		result.EvidenceID = nil
	}
	updates := map[string]any{"state": state, "receipt": normalized, "evidence_id": result.EvidenceID, "updated_at": s.now().UTC()}
	if state == StateSucceeded || state == StateFailed {
		updates["unresolved_fingerprint"] = nil
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	r := scoped(s.db.WithContext(persistCtx).Model(&Operation{}), sc).Where("id = ? AND state IN ?", id, []string{StateExecuting, StateUnknown}).Updates(updates)
	if r.Error != nil {
		return Operation{}, r.Error
	}
	return s.GetOperation(persistCtx, sc, id)
}
