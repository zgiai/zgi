package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormConfirmationContinuationStore persists only an opaque protected
// envelope plus non-secret lookup and terminal projection fields.
type GormConfirmationContinuationStore struct {
	db        *gorm.DB
	protector ContinuationPayloadProtector
	now       func() time.Time
}

var _ ConfirmationContinuationStore = (*GormConfirmationContinuationStore)(nil)

func NewGormConfirmationContinuationStore(
	db *gorm.DB,
	protector ContinuationPayloadProtector,
) (*GormConfirmationContinuationStore, error) {
	if db == nil || protector == nil {
		return nil, connectorError(core.ErrorCodeInternal, false)
	}
	return &GormConfirmationContinuationStore{
		db: db, protector: protector, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Freeze inserts the immutable request once per route receipt. A retry with
// the exact same identity, origin, and input returns the existing record; a
// retry that attempts to change any frozen value returns a conflict.
func (store *GormConfirmationContinuationStore) Freeze(
	ctx context.Context,
	frozen FrozenConfirmationRequest,
) (ConfirmationContinuation, error) {
	if err := store.available(); err != nil {
		return ConfirmationContinuation{}, err
	}
	frozen = bindConfirmationOrigin(frozen)
	if err := validateFrozenConfirmationRequest(frozen); err != nil {
		return ConfirmationContinuation{}, err
	}
	if err := store.verifyPinnedConfirmationRoute(ctx, frozen); err != nil {
		return ConfirmationContinuation{}, err
	}

	envelope, err := store.protector.Seal(
		ctx,
		frozen.Request.OrganizationID,
		frozen.RouteReceipt,
		frozen,
	)
	if err != nil || !validContinuationEnvelope(envelope) {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	now := store.now().UTC()
	if now.IsZero() {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	record := confirmationContinuationRecordFromFrozen(frozen, envelope, now)
	result := store.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "route_receipt"}},
		DoNothing: true,
	}).Create(&record)
	if result.Error != nil {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	if result.RowsAffected == 1 {
		return ConfirmationContinuation{
			Frozen: frozen, State: ConfirmationContinuationPending,
			CreatedAt: now, UpdatedAt: now,
		}, nil
	}

	existing, err := store.Load(
		ctx,
		frozen.Request.OrganizationID,
		frozen.Request.OwnerAccountID,
		frozen.RouteReceipt,
	)
	if err != nil {
		return ConfirmationContinuation{}, err
	}
	equal, err := frozenConfirmationRequestsEqual(existing.Frozen, frozen)
	if err != nil {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	if !equal {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeConflict, false)
	}
	return existing, nil
}

// Load requires both tenant and personal owner scope in addition to the
// unguessable receipt. Missing and cross-owner receipts are intentionally
// indistinguishable to callers.
func (store *GormConfirmationContinuationStore) Load(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	receipt RouteClaimReceipt,
) (ConfirmationContinuation, error) {
	if err := store.available(); err != nil {
		return ConfirmationContinuation{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || !validUUIDReceipt(receipt) {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInvalidInput, false)
	}
	receiptID, _ := uuid.Parse(string(receipt))
	var record confirmationContinuationRecord
	err := store.db.WithContext(ctx).
		Where(
			"route_receipt = ? AND organization_id = ? AND owner_account_id = ?",
			receiptID,
			organizationID,
			ownerAccountID,
		).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeAccessDenied, false)
	}
	if err != nil {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	return store.continuationFromRecord(ctx, record)
}

// RecordTerminal performs a single pending-to-terminal compare-and-swap. An
// exact replay returns the already stored safe result. Any different replay,
// including confirm racing cancel, returns a conflict without changing the
// winning terminal state.
func (store *GormConfirmationContinuationStore) RecordTerminal(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	receipt RouteClaimReceipt,
	terminal ConfirmationTerminal,
) (ConfirmationContinuation, error) {
	if err := store.available(); err != nil {
		return ConfirmationContinuation{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || !validUUIDReceipt(receipt) {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInvalidInput, false)
	}
	if err := validateConfirmationTerminal(terminal); err != nil {
		return ConfirmationContinuation{}, err
	}
	now := store.now().UTC()
	if now.IsZero() {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	receiptID, _ := uuid.Parse(string(receipt))
	updates := map[string]any{
		"state":           string(terminal.State),
		"decision_id":     optionalContinuationUUID(terminal.DecisionID),
		"outcome_id":      optionalContinuationUUID(terminal.OutcomeID),
		"diagnostic_code": optionalContinuationString(terminal.DiagnosticCode),
		"updated_at":      now,
		"terminal_at":     now,
	}
	result := store.db.WithContext(ctx).
		Model(&confirmationContinuationRecord{}).
		Where(
			"route_receipt = ? AND organization_id = ? AND owner_account_id = ? AND state = ?",
			receiptID,
			organizationID,
			ownerAccountID,
			ConfirmationContinuationPending,
		).
		Updates(updates)
	if result.Error != nil {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}

	current, err := store.Load(ctx, organizationID, ownerAccountID, receipt)
	if err != nil {
		return ConfirmationContinuation{}, err
	}
	if result.RowsAffected == 1 {
		if !continuationMatchesTerminal(current, terminal) {
			return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
		}
		return current, nil
	}
	if continuationMatchesTerminal(current, terminal) {
		return current, nil
	}
	return ConfirmationContinuation{}, connectorError(core.ErrorCodeConflict, false)
}

func (store *GormConfirmationContinuationStore) available() error {
	if store == nil || store.db == nil || store.protector == nil || store.now == nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	return nil
}

func (store *GormConfirmationContinuationStore) verifyPinnedConfirmationRoute(
	ctx context.Context,
	frozen FrozenConfirmationRequest,
) error {
	receiptID, _ := uuid.Parse(string(frozen.RouteReceipt))
	var claim invocationRecord
	err := store.db.WithContext(ctx).
		Where(
			"receipt_id = ? AND organization_id = ? AND owner_account_id = ?",
			receiptID,
			frozen.Request.OrganizationID,
			frozen.Request.OwnerAccountID,
		).
		Take(&claim).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	if err != nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	digest, err := invocationInputDigest(frozen.Request.Input)
	if err != nil || (claim.State != "claimed" && claim.State != "awaiting_confirmation") || claim.InputDigest != digest || claim.ContractDigest != frozen.AcceptedOperation.ContractDigest {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	expected := invocationRow(InvocationClaim{Identity: routeResolutionRequest(frozen.Request), Snapshot: InvocationSnapshot{InputDigest: digest, ContractDigest: claim.ContractDigest, BindingRevision: claim.BindingRevision, CredentialVersion: claim.CredentialVersion}})
	claim.ReceiptID = uuid.Nil
	claim.CreatedAt = time.Time{}
	claim.State = ""
	if claim != expected {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	return nil
}

func (store *GormConfirmationContinuationStore) continuationFromRecord(
	ctx context.Context,
	record confirmationContinuationRecord,
) (ConfirmationContinuation, error) {
	if err := validateConfirmationContinuationRecord(record); err != nil {
		return ConfirmationContinuation{}, err
	}
	receipt := RouteClaimReceipt(record.RouteReceipt.String())
	frozen, err := store.protector.Open(
		ctx,
		record.OrganizationID,
		receipt,
		ContinuationPayloadEnvelope(record.ProtectedPayloadEnvelope),
	)
	if err != nil || validateFrozenConfirmationRequest(frozen) != nil ||
		frozen.RouteReceipt != receipt || frozen.Request.OrganizationID != record.OrganizationID ||
		frozen.Request.OwnerAccountID != record.OwnerAccountID ||
		!confirmationOriginMatchesRecord(frozen.Origin, record) {
		return ConfirmationContinuation{}, connectorError(core.ErrorCodeInternal, false)
	}
	continuation := ConfirmationContinuation{
		Frozen: frozen, State: ConfirmationContinuationState(record.State),
		DecisionID: dereferenceContinuationUUID(record.DecisionID),
		OutcomeID:  dereferenceContinuationUUID(record.OutcomeID),
		CreatedAt:  record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
		TerminalAt: dereferenceContinuationTime(record.TerminalAt),
	}
	if record.DiagnosticCode != nil {
		continuation.DiagnosticCode = *record.DiagnosticCode
	}
	return continuation, nil
}

type confirmationContinuationRecord struct {
	RouteReceipt             uuid.UUID  `gorm:"column:route_receipt;type:uuid;primaryKey"`
	OrganizationID           uuid.UUID  `gorm:"column:organization_id;type:uuid;not null;index:idx_connector_confirmation_continuations_owner_state,priority:1"`
	OwnerAccountID           uuid.UUID  `gorm:"column:owner_account_id;type:uuid;not null;index:idx_connector_confirmation_continuations_owner_state,priority:2"`
	OriginVersion            int        `gorm:"column:origin_version;not null;default:0"`
	DigitalHumanID           *string    `gorm:"column:digital_human_id;type:varchar(255)"`
	OriginSurface            string     `gorm:"column:origin_surface;type:varchar(16);not null"`
	ConversationID           *string    `gorm:"column:conversation_id;type:varchar(255)"`
	MessageID                *string    `gorm:"column:message_id;type:varchar(255)"`
	AgentID                  *string    `gorm:"column:agent_id;type:varchar(255)"`
	WorkflowID               *string    `gorm:"column:workflow_id;type:varchar(255)"`
	RunID                    *string    `gorm:"column:run_id;type:varchar(255)"`
	NodeID                   *string    `gorm:"column:node_id;type:varchar(255)"`
	ProtectedPayloadEnvelope string     `gorm:"column:protected_payload_envelope;type:text;not null" json:"-"`
	State                    string     `gorm:"column:state;type:varchar(16);not null;index:idx_connector_confirmation_continuations_owner_state,priority:3"`
	DecisionID               *uuid.UUID `gorm:"column:decision_id;type:uuid"`
	OutcomeID                *uuid.UUID `gorm:"column:outcome_id;type:uuid"`
	DiagnosticCode           *string    `gorm:"column:diagnostic_code;type:varchar(64)"`
	CreatedAt                time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt                time.Time  `gorm:"column:updated_at;not null;index:idx_connector_confirmation_continuations_owner_state,priority:4"`
	TerminalAt               *time.Time `gorm:"column:terminal_at"`
}

func (confirmationContinuationRecord) TableName() string {
	return "connector_confirmation_continuations"
}

func confirmationContinuationRecordFromFrozen(
	frozen FrozenConfirmationRequest,
	envelope ContinuationPayloadEnvelope,
	now time.Time,
) confirmationContinuationRecord {
	origin := frozen.Origin
	receiptID, _ := uuid.Parse(string(frozen.RouteReceipt))
	return confirmationContinuationRecord{
		RouteReceipt: receiptID, OrganizationID: frozen.Request.OrganizationID,
		OwnerAccountID: frozen.Request.OwnerAccountID, OriginSurface: string(origin.Surface),
		OriginVersion: origin.OriginVersion, DigitalHumanID: optionalContinuationString(origin.DigitalHumanID),
		ConversationID: optionalContinuationString(origin.ConversationID),
		MessageID:      optionalContinuationString(origin.MessageID),
		AgentID:        optionalContinuationString(origin.AgentID),
		WorkflowID:     optionalContinuationString(origin.WorkflowID),
		RunID:          optionalContinuationString(origin.RunID), NodeID: optionalContinuationString(origin.NodeID),
		ProtectedPayloadEnvelope: string(envelope), State: string(ConfirmationContinuationPending),
		CreatedAt: now, UpdatedAt: now,
	}
}

func validateConfirmationContinuationRecord(record confirmationContinuationRecord) error {
	state := ConfirmationContinuationState(record.State)
	if record.RouteReceipt == uuid.Nil || record.OrganizationID == uuid.Nil || record.OwnerAccountID == uuid.Nil ||
		!validContinuationEnvelope(ContinuationPayloadEnvelope(record.ProtectedPayloadEnvelope)) ||
		!state.valid() || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return connectorError(core.ErrorCodeInternal, false)
	}
	origin := confirmationOriginFromRecord(record)
	if err := validateStoredConfirmationOrigin(origin); err != nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	terminal := ConfirmationTerminal{
		State: state, DecisionID: dereferenceContinuationUUID(record.DecisionID),
		OutcomeID: dereferenceContinuationUUID(record.OutcomeID),
	}
	if record.DiagnosticCode != nil {
		terminal.DiagnosticCode = *record.DiagnosticCode
	}
	if state == ConfirmationContinuationPending {
		if record.DecisionID != nil || record.OutcomeID != nil || record.DiagnosticCode != nil || record.TerminalAt != nil {
			return connectorError(core.ErrorCodeInternal, false)
		}
		return nil
	}
	if record.TerminalAt == nil || record.TerminalAt.IsZero() || record.TerminalAt.Before(record.CreatedAt) ||
		record.TerminalAt.After(record.UpdatedAt) || validateConfirmationTerminal(terminal) != nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	return nil
}

func validateStoredConfirmationOrigin(origin ConfirmationOrigin) error {
	for _, value := range []string{
		origin.ConversationID, origin.MessageID, origin.AgentID,
		origin.WorkflowID, origin.RunID, origin.NodeID,
	} {
		if value != "" && !validOpaqueText(value, 255) {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	}
	if origin.Surface != core.TargetTypeDigitalHuman && (origin.OriginVersion != 0 || origin.DigitalHumanID != "") {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	switch origin.Surface {
	case core.TargetTypeChat:
		if origin.ConversationID == "" || origin.MessageID == "" || origin.AgentID != "" ||
			origin.WorkflowID != "" || origin.RunID != "" || origin.NodeID != "" {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case core.TargetTypeAgent:
		if origin.ConversationID == "" || origin.MessageID == "" || origin.AgentID == "" ||
			origin.WorkflowID != "" || origin.NodeID != "" {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case core.TargetTypeDigitalHuman:
		return validateConfirmationOrigin(origin, core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: origin.DigitalHumanID})
	case core.TargetTypeWorkflow:
		if origin.WorkflowID == "" || origin.RunID == "" || origin.NodeID == "" ||
			origin.ConversationID != "" || origin.MessageID != "" || origin.AgentID != "" {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	default:
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

func confirmationOriginFromRecord(record confirmationContinuationRecord) ConfirmationOrigin {
	return ConfirmationOrigin{
		OriginVersion: record.OriginVersion, DigitalHumanID: dereferenceContinuationString(record.DigitalHumanID),
		Surface: core.TargetType(record.OriginSurface), ConversationID: dereferenceContinuationString(record.ConversationID),
		MessageID: dereferenceContinuationString(record.MessageID), AgentID: dereferenceContinuationString(record.AgentID),
		WorkflowID: dereferenceContinuationString(record.WorkflowID), RunID: dereferenceContinuationString(record.RunID),
		NodeID: dereferenceContinuationString(record.NodeID),
	}
}

func confirmationOriginMatchesRecord(origin ConfirmationOrigin, record confirmationContinuationRecord) bool {
	return origin == confirmationOriginFromRecord(record)
}

func frozenConfirmationRequestsEqual(left, right FrozenConfirmationRequest) (bool, error) {
	if left.RouteReceipt != right.RouteReceipt || left.Origin != right.Origin ||
		routeResolutionRequest(left.Request) != routeResolutionRequest(right.Request) ||
		left.Request.Interactive != right.Request.Interactive ||
		!reflect.DeepEqual(left.AcceptedOperation, right.AcceptedOperation) ||
		left.Request.Confirmation != nil || right.Request.Confirmation != nil {
		return false, nil
	}
	leftInput, err := json.Marshal(left.Request.Input)
	if err != nil {
		return false, err
	}
	rightInput, err := json.Marshal(right.Request.Input)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftInput, rightInput), nil
}

func continuationMatchesTerminal(continuation ConfirmationContinuation, terminal ConfirmationTerminal) bool {
	return continuation.State == terminal.State && continuation.DecisionID == terminal.DecisionID &&
		continuation.OutcomeID == terminal.OutcomeID && continuation.DiagnosticCode == terminal.DiagnosticCode
}

func optionalContinuationString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func optionalContinuationUUID(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	copy := value
	return &copy
}

func dereferenceContinuationString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dereferenceContinuationUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func dereferenceContinuationTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}
