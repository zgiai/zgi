package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultDecisionListLimit = 100
	maxDecisionListLimit     = 500
	maxAuditDetailsBytes     = 32 * 1024
)

var detailKeySeparator = regexp.MustCompile(`[^a-z0-9]+`)

// Repository persists immutable, non-secret Connector lifecycle evidence.
// Every read is scoped by both organization and owner.
type Repository interface {
	CreateDecision(ctx context.Context, decision core.LifecycleDecision) error
	FinalizeConfirmation(ctx context.Context, finalization, operationDecision core.LifecycleDecision) error
	GetDecision(ctx context.Context, organizationID, ownerAccountID, decisionID uuid.UUID) (core.LifecycleDecision, error)
	ListDecisions(ctx context.Context, organizationID, ownerAccountID uuid.UUID, filter DecisionListFilter) ([]core.LifecycleDecision, error)
	RecordOutcome(ctx context.Context, outcome core.OperationOutcome) error
	ConvergeOutcome(ctx context.Context, unknown core.OperationOutcome) (core.OperationOutcome, error)
	GetOutcome(ctx context.Context, organizationID, ownerAccountID, outcomeID uuid.UUID) (core.OperationOutcome, error)
	GetOutcomeByDecision(ctx context.Context, organizationID, ownerAccountID, decisionID uuid.UUID) (core.OperationOutcome, error)
	GetOutcomeByLogicalOperation(ctx context.Context, organizationID, ownerAccountID uuid.UUID, logicalOperationID string) (core.OperationOutcome, error)
}

type DecisionListFilter struct {
	ConnectorID        core.Identifier
	ConnectionID       uuid.UUID
	CorrelationID      string
	LogicalOperationID string
	ReceiptID          string
	Statuses           []core.DecisionStatus
	Limit              int
}

type GormRepository struct {
	db *gorm.DB
}

var _ Repository = (*GormRepository)(nil)

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) CreateDecision(ctx context.Context, decision core.LifecycleDecision) error {
	if repository == nil || repository.db == nil {
		return internalError()
	}
	// The finalization marker is valid only through FinalizeConfirmation,
	// which provides the route-receipt CAS. Generic insertion would bypass the
	// advisory lock and is therefore rejected even for an otherwise valid row.
	if decision.Decision == "confirmation_finalization" {
		return invalidInputError()
	}
	record, err := newLifecycleDecisionRecord(decision)
	if err != nil {
		return err
	}

	db := repository.db.WithContext(ctx)
	if conflict, err := decisionIdentityExists(db, record); err != nil {
		return internalError()
	} else if conflict {
		return decisionNotWritten{conflictError()}
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return internalError()
	}
	if result.RowsAffected != 1 {
		return decisionNotWritten{conflictError()}
	}
	return nil
}

// FinalizeConfirmation performs an atomic, once-only CAS scoped to the route
// receipt and writes the winning operation decision in the same transaction.
// A confirmed prompt is therefore inseparable from its unique submitted
// dispatch claim, while cancellation is inseparable from its cancelled audit
// decision. Either both immutable rows commit or neither does.
func (repository *GormRepository) FinalizeConfirmation(
	ctx context.Context,
	decision core.LifecycleDecision,
	operationDecision core.LifecycleDecision,
) error {
	if repository == nil || repository.db == nil {
		return internalError()
	}
	if !validConfirmationFinalizationPair(decision, operationDecision) {
		return invalidInputError()
	}
	record, err := newLifecycleDecisionRecord(decision)
	if err != nil {
		return err
	}
	operationRecord, err := newLifecycleDecisionRecord(operationDecision)
	if err != nil {
		return err
	}
	lockIdentity := decision.OrganizationID.String() + "/" + decision.ReceiptID
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockIdentity).Error; lockErr != nil {
			return internalError()
		}
		var count int64
		if countErr := tx.Model(&lifecycleDecisionRecord{}).
			Where("organization_id = ? AND receipt_id = ? AND decision = ?", decision.OrganizationID, decision.ReceiptID, decision.Decision).
			Count(&count).Error; countErr != nil {
			return internalError()
		}
		if count != 0 {
			return conflictError()
		}
		if err := requireOpenExecutionClaim(tx, decision); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return internalError()
		}
		if result.RowsAffected != 1 {
			return conflictError()
		}
		result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&operationRecord)
		if result.Error != nil {
			return internalError()
		}
		if result.RowsAffected != 1 {
			return conflictError()
		}
		return nil
	})
	return preserveConnectorError(err)
}

func (repository *GormRepository) GetDecision(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	decisionID uuid.UUID,
) (core.LifecycleDecision, error) {
	if repository == nil || repository.db == nil || !validOwnerScope(organizationID, ownerAccountID) || decisionID == uuid.Nil {
		return core.LifecycleDecision{}, invalidInputError()
	}
	var record lifecycleDecisionRecord
	err := repository.db.WithContext(ctx).
		Where("organization_id = ? AND owner_account_id = ? AND id = ?", organizationID, ownerAccountID, decisionID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return core.LifecycleDecision{}, notFoundError()
	}
	if err != nil {
		return core.LifecycleDecision{}, internalError()
	}
	return decisionFromRecord(record)
}

func (repository *GormRepository) ListDecisions(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	filter DecisionListFilter,
) ([]core.LifecycleDecision, error) {
	if repository == nil || repository.db == nil || !validOwnerScope(organizationID, ownerAccountID) {
		return nil, invalidInputError()
	}
	if filter.ConnectorID != "" && !filter.ConnectorID.Valid() {
		return nil, invalidInputError()
	}
	if filter.CorrelationID != "" && !validRequiredText(filter.CorrelationID, 128) {
		return nil, invalidInputError()
	}
	if (filter.LogicalOperationID != "" && !validRequiredText(filter.LogicalOperationID, 128)) ||
		(filter.ReceiptID != "" && !validRequiredText(filter.ReceiptID, 128)) {
		return nil, invalidInputError()
	}
	for _, status := range filter.Statuses {
		if !status.Valid() {
			return nil, invalidInputError()
		}
	}
	limit := filter.Limit
	if limit == 0 {
		limit = defaultDecisionListLimit
	}
	if limit < 0 || limit > maxDecisionListLimit {
		return nil, invalidInputError()
	}

	query := repository.db.WithContext(ctx).
		Where("organization_id = ? AND owner_account_id = ?", organizationID, ownerAccountID)
	if filter.ConnectorID != "" {
		query = query.Where("connector_id = ?", filter.ConnectorID)
	}
	if filter.ConnectionID != uuid.Nil {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}
	if filter.CorrelationID != "" {
		query = query.Where("correlation_id = ?", filter.CorrelationID)
	}
	if filter.LogicalOperationID != "" {
		query = query.Where("logical_operation_id = ?", filter.LogicalOperationID)
	}
	if filter.ReceiptID != "" {
		query = query.Where("receipt_id = ?", filter.ReceiptID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	var records []lifecycleDecisionRecord
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, internalError()
	}
	decisions := make([]core.LifecycleDecision, 0, len(records))
	for _, record := range records {
		decision, err := decisionFromRecord(record)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// RecordOutcome writes exactly one terminal outcome for a fully matching
// submitted decision. Denied, blocked, and cancelled decisions cannot receive
// an outcome because no provider operation was submitted.
func (repository *GormRepository) RecordOutcome(ctx context.Context, outcome core.OperationOutcome) error {
	if repository == nil || repository.db == nil {
		return internalError()
	}
	record, err := newOperationOutcomeRecord(outcome)
	if err != nil {
		return err
	}

	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var decisionRecord lifecycleDecisionRecord
		query := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where(
				"id = ? AND organization_id = ? AND owner_account_id = ? AND status = ?",
				outcome.DecisionID,
				outcome.OrganizationID,
				outcome.OwnerAccountID,
				core.DecisionStatusSubmitted,
			).
			Take(&decisionRecord)
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return notFoundError()
		}
		if query.Error != nil {
			return internalError()
		}
		decision, conversionErr := decisionFromRecord(decisionRecord)
		if conversionErr != nil {
			return conversionErr
		}
		if !outcomeMatchesSubmittedDecision(outcome, decision) {
			return notFoundError()
		}

		conflict, lookupErr := terminalOutcomeExists(tx, record)
		if lookupErr != nil {
			return internalError()
		}
		if conflict {
			return conflictError()
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return internalError()
		}
		if result.RowsAffected != 1 {
			return conflictError()
		}
		return nil
	})
	return preserveConnectorError(err)
}

// ConvergeOutcome returns existing immutable outcome evidence for the submitted
// logical operation, or atomically records the supplied conservative UNKNOWN
// outcome when no outcome exists. The submitted decision is exclusively locked
// so this method serializes with RecordOutcome's shared decision lock. It never
// overwrites a provider result and never manufactures a successful outcome.
func (repository *GormRepository) ConvergeOutcome(
	ctx context.Context,
	unknown core.OperationOutcome,
) (core.OperationOutcome, error) {
	if repository == nil || repository.db == nil || !validUnknownConvergenceOutcome(unknown) {
		return core.OperationOutcome{}, invalidInputError()
	}
	record, err := newOperationOutcomeRecord(unknown)
	if err != nil {
		return core.OperationOutcome{}, err
	}

	var durable core.OperationOutcome
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var decisionRecord lifecycleDecisionRecord
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND organization_id = ? AND owner_account_id = ? AND status = ?",
				unknown.DecisionID,
				unknown.OrganizationID,
				unknown.OwnerAccountID,
				core.DecisionStatusSubmitted,
			).
			Take(&decisionRecord)
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return notFoundError()
		}
		if query.Error != nil {
			return internalError()
		}
		decision, conversionErr := decisionFromRecord(decisionRecord)
		if conversionErr != nil {
			return conversionErr
		}
		if !outcomeMatchesSubmittedDecision(unknown, decision) {
			return notFoundError()
		}

		existing, found, lookupErr := findConvergedOutcome(tx, unknown)
		if lookupErr != nil {
			return lookupErr
		}
		if found {
			durable = existing
			return nil
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return internalError()
		}
		if result.RowsAffected == 1 {
			durable = unknown
			return nil
		}
		existing, found, lookupErr = findConvergedOutcome(tx, unknown)
		if lookupErr != nil {
			return lookupErr
		}
		if !found {
			return conflictError()
		}
		durable = existing
		return nil
	})
	if err != nil {
		return core.OperationOutcome{}, preserveConnectorError(err)
	}
	return durable, nil
}

func (repository *GormRepository) GetOutcome(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	outcomeID uuid.UUID,
) (core.OperationOutcome, error) {
	if repository == nil || repository.db == nil || !validOwnerScope(organizationID, ownerAccountID) || outcomeID == uuid.Nil {
		return core.OperationOutcome{}, invalidInputError()
	}
	return repository.getOutcome(
		ctx,
		organizationID,
		ownerAccountID,
		"id = ?",
		outcomeID,
	)
}

func (repository *GormRepository) GetOutcomeByDecision(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	decisionID uuid.UUID,
) (core.OperationOutcome, error) {
	if repository == nil || repository.db == nil || !validOwnerScope(organizationID, ownerAccountID) || decisionID == uuid.Nil {
		return core.OperationOutcome{}, invalidInputError()
	}
	return repository.getOutcome(
		ctx,
		organizationID,
		ownerAccountID,
		"decision_id = ?",
		decisionID,
	)
}

func (repository *GormRepository) GetOutcomeByLogicalOperation(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	logicalOperationID string,
) (core.OperationOutcome, error) {
	if repository == nil || repository.db == nil || !validOwnerScope(organizationID, ownerAccountID) ||
		!validRequiredText(logicalOperationID, 128) {
		return core.OperationOutcome{}, invalidInputError()
	}
	return repository.getOutcome(
		ctx,
		organizationID,
		ownerAccountID,
		"logical_operation_id = ?",
		logicalOperationID,
	)
}

func (repository *GormRepository) getOutcome(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	predicate string,
	value any,
) (core.OperationOutcome, error) {
	var record operationOutcomeRecord
	err := repository.db.WithContext(ctx).
		Where("organization_id = ? AND owner_account_id = ?", organizationID, ownerAccountID).
		Where(predicate, value).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return core.OperationOutcome{}, notFoundError()
	}
	if err != nil {
		return core.OperationOutcome{}, internalError()
	}
	return outcomeFromRecord(record)
}

type lifecycleDecisionRecord struct {
	ID                 uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	OrganizationID     uuid.UUID      `gorm:"column:organization_id;type:uuid;not null;uniqueIndex:idx_connector_decisions_operation_identity,priority:1,where:logical_operation_id IS NOT NULL AND logical_operation_id <> '';uniqueIndex:idx_connector_decisions_lifecycle_identity,priority:1,where:logical_operation_id IS NULL OR logical_operation_id = '';uniqueIndex:idx_connector_decisions_finalization_identity,priority:1,where:decision = 'confirmation_finalization';index:idx_connector_lifecycle_decisions_owner_created,priority:1"`
	OwnerAccountID     uuid.UUID      `gorm:"column:owner_account_id;type:uuid;not null;index:idx_connector_lifecycle_decisions_owner_created,priority:2"`
	ActingPrincipalID  uuid.UUID      `gorm:"column:acting_principal_id;type:uuid;not null"`
	ConnectorID        string         `gorm:"column:connector_id;type:varchar(128);not null"`
	ConnectionID       *uuid.UUID     `gorm:"column:connection_id;type:uuid;index:idx_connector_lifecycle_decisions_connection_created,priority:1"`
	TargetType         *string        `gorm:"column:target_type;type:varchar(16)"`
	TargetScope        *string        `gorm:"column:target_scope"`
	TargetVersionID    *string        `gorm:"column:target_version_id"`
	TargetID           *string        `gorm:"column:target_id;type:varchar(255)"`
	BindingInstanceID  *string        `gorm:"column:binding_instance_id;type:varchar(255)"`
	OperationID        *string        `gorm:"column:operation_id;type:varchar(128)"`
	LogicalOperationID *string        `gorm:"column:logical_operation_id;type:varchar(128);uniqueIndex:idx_connector_decisions_operation_identity,priority:2"`
	Decision           string         `gorm:"column:decision;type:varchar(64);not null;uniqueIndex:idx_connector_decisions_operation_identity,priority:3;uniqueIndex:idx_connector_decisions_lifecycle_identity,priority:3"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;uniqueIndex:idx_connector_decisions_operation_identity,priority:4;uniqueIndex:idx_connector_decisions_lifecycle_identity,priority:4"`
	CorrelationID      string         `gorm:"column:correlation_id;type:varchar(128);not null;uniqueIndex:idx_connector_decisions_lifecycle_identity,priority:2"`
	ReceiptID          *string        `gorm:"column:receipt_id;type:varchar(128);uniqueIndex:idx_connector_decisions_finalization_identity,priority:2"`
	Details            datatypes.JSON `gorm:"column:details;type:jsonb;not null"`
	CreatedAt          time.Time      `gorm:"column:created_at;not null;index:idx_connector_lifecycle_decisions_owner_created,priority:3;index:idx_connector_lifecycle_decisions_connection_created,priority:2"`
}

func (lifecycleDecisionRecord) TableName() string {
	return "connector_lifecycle_decisions"
}

type operationOutcomeRecord struct {
	ID                 uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	DecisionID         uuid.UUID `gorm:"column:decision_id;type:uuid;not null;uniqueIndex"`
	OrganizationID     uuid.UUID `gorm:"column:organization_id;type:uuid;not null;uniqueIndex:idx_connector_operation_outcomes_logical_operation,priority:1"`
	OwnerAccountID     uuid.UUID `gorm:"column:owner_account_id;type:uuid;not null"`
	ActingPrincipalID  uuid.UUID `gorm:"column:acting_principal_id;type:uuid;not null"`
	ConnectorID        string    `gorm:"column:connector_id;type:varchar(128);not null"`
	ConnectionID       uuid.UUID `gorm:"column:connection_id;type:uuid;not null;index:idx_connector_operation_outcomes_connection_created,priority:1"`
	TargetType         string    `gorm:"column:target_type;type:varchar(16);not null"`
	TargetScope        string    `gorm:"column:target_scope"`
	TargetVersionID    string    `gorm:"column:target_version_id"`
	TargetID           string    `gorm:"column:target_id;type:varchar(255);not null"`
	BindingInstanceID  *string   `gorm:"column:binding_instance_id;type:varchar(255)"`
	OperationID        string    `gorm:"column:operation_id;type:varchar(128);not null"`
	LogicalOperationID string    `gorm:"column:logical_operation_id;type:varchar(128);not null;uniqueIndex:idx_connector_operation_outcomes_logical_operation,priority:2"`
	Status             string    `gorm:"column:status;type:varchar(32);not null"`
	ProviderRequestID  *string   `gorm:"column:provider_request_id;type:varchar(128)"`
	DiagnosticCode     *string   `gorm:"column:diagnostic_code;type:varchar(64)"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;index:idx_connector_operation_outcomes_connection_created,priority:2"`
}

func (operationOutcomeRecord) TableName() string {
	return "connector_operation_outcomes"
}

func newLifecycleDecisionRecord(decision core.LifecycleDecision) (lifecycleDecisionRecord, error) {
	if err := decision.Validate(); err != nil ||
		!validRequiredText(decision.Decision, 64) ||
		!validRequiredText(decision.CorrelationID, 128) ||
		!validOptionalText(decision.ReceiptID, 128) ||
		!validOptionalText(decision.LogicalOperationID, 128) {
		return lifecycleDecisionRecord{}, invalidInputError()
	}
	details, err := encodeSafeDetails(decision.Details)
	if err != nil {
		return lifecycleDecisionRecord{}, err
	}
	record := lifecycleDecisionRecord{
		ID:                 decision.ID,
		OrganizationID:     decision.OrganizationID,
		OwnerAccountID:     decision.OwnerAccountID,
		ActingPrincipalID:  decision.ActingPrincipalID,
		ConnectorID:        decision.ConnectorID.String(),
		ConnectionID:       optionalUUID(decision.ConnectionID),
		OperationID:        optionalString(decision.OperationID.String()),
		LogicalOperationID: optionalString(decision.LogicalOperationID),
		Decision:           decision.Decision,
		Status:             string(decision.Status),
		CorrelationID:      decision.CorrelationID,
		ReceiptID:          optionalString(decision.ReceiptID),
		Details:            details,
		CreatedAt:          normalizedCreatedAt(decision.CreatedAt),
	}
	if decision.Target != nil {
		record.TargetType = optionalString(string(decision.Target.Type))
		record.TargetID = optionalString(decision.Target.ID)
		record.TargetScope = optionalString(string(decision.Target.EffectiveScope()))
		// Targeted decisions use the same explicit version representation as
		// claims/outcomes (whose version columns are NOT NULL). SQL NULL is
		// reserved for decisions with no target at all.
		versionID := decision.Target.VersionID
		record.TargetVersionID = &versionID
		record.BindingInstanceID = optionalString(decision.Target.BindingInstanceID)
	}
	return record, nil
}

func decisionFromRecord(record lifecycleDecisionRecord) (core.LifecycleDecision, error) {
	details := map[string]any{}
	if len(record.Details) > 0 {
		if err := json.Unmarshal(record.Details, &details); err != nil {
			return core.LifecycleDecision{}, internalError()
		}
	}
	if _, err := encodeSafeDetails(details); err != nil {
		return core.LifecycleDecision{}, internalError()
	}
	decision := core.LifecycleDecision{
		ID:                 record.ID,
		OrganizationID:     record.OrganizationID,
		OwnerAccountID:     record.OwnerAccountID,
		ActingPrincipalID:  record.ActingPrincipalID,
		ConnectorID:        core.Identifier(record.ConnectorID),
		ConnectionID:       dereferenceUUID(record.ConnectionID),
		OperationID:        core.Identifier(dereferenceString(record.OperationID)),
		LogicalOperationID: dereferenceString(record.LogicalOperationID),
		Decision:           record.Decision,
		Status:             core.DecisionStatus(record.Status),
		CorrelationID:      record.CorrelationID,
		ReceiptID:          dereferenceString(record.ReceiptID),
		Details:            details,
		CreatedAt:          record.CreatedAt,
	}
	if record.TargetType != nil || record.TargetID != nil || record.BindingInstanceID != nil {
		decision.Target = &core.TargetRef{
			Type: core.TargetType(dereferenceString(record.TargetType)), Scope: core.TargetScope(dereferenceString(record.TargetScope)), VersionID: dereferenceString(record.TargetVersionID),
			ID:                dereferenceString(record.TargetID),
			BindingInstanceID: dereferenceString(record.BindingInstanceID),
		}
	}
	if err := decision.Validate(); err != nil {
		return core.LifecycleDecision{}, internalError()
	}
	return decision, nil
}

func newOperationOutcomeRecord(outcome core.OperationOutcome) (operationOutcomeRecord, error) {
	if err := outcome.Validate(); err != nil ||
		!validRequiredText(outcome.LogicalOperationID, 128) ||
		!validOptionalOpaqueText(outcome.ProviderRequestID, 128) ||
		!validOptionalCode(outcome.DiagnosticCode, 64) {
		return operationOutcomeRecord{}, invalidInputError()
	}
	return operationOutcomeRecord{
		ID:                outcome.ID,
		DecisionID:        outcome.DecisionID,
		OrganizationID:    outcome.OrganizationID,
		OwnerAccountID:    outcome.OwnerAccountID,
		ActingPrincipalID: outcome.ActingPrincipalID,
		ConnectorID:       outcome.ConnectorID.String(),
		ConnectionID:      outcome.ConnectionID,
		TargetType:        string(outcome.Target.Type),
		TargetID:          outcome.Target.ID, TargetScope: string(outcome.Target.EffectiveScope()), TargetVersionID: outcome.Target.VersionID,
		BindingInstanceID:  optionalString(outcome.Target.BindingInstanceID),
		OperationID:        outcome.OperationID.String(),
		LogicalOperationID: outcome.LogicalOperationID,
		Status:             string(outcome.Status),
		ProviderRequestID:  optionalString(outcome.ProviderRequestID),
		DiagnosticCode:     optionalString(outcome.DiagnosticCode),
		CreatedAt:          normalizedCreatedAt(outcome.CreatedAt),
	}, nil
}

func outcomeFromRecord(record operationOutcomeRecord) (core.OperationOutcome, error) {
	outcome := core.OperationOutcome{
		ID:                record.ID,
		DecisionID:        record.DecisionID,
		OrganizationID:    record.OrganizationID,
		OwnerAccountID:    record.OwnerAccountID,
		ActingPrincipalID: record.ActingPrincipalID,
		ConnectorID:       core.Identifier(record.ConnectorID),
		ConnectionID:      record.ConnectionID,
		Target: core.TargetRef{
			Type: core.TargetType(record.TargetType), Scope: core.TargetScope(record.TargetScope), VersionID: record.TargetVersionID,
			ID:                record.TargetID,
			BindingInstanceID: dereferenceString(record.BindingInstanceID),
		},
		OperationID:        core.Identifier(record.OperationID),
		LogicalOperationID: record.LogicalOperationID,
		Status:             core.OutcomeStatus(record.Status),
		ProviderRequestID:  dereferenceString(record.ProviderRequestID),
		DiagnosticCode:     dereferenceString(record.DiagnosticCode),
		CreatedAt:          record.CreatedAt,
	}
	if err := outcome.Validate(); err != nil {
		return core.OperationOutcome{}, internalError()
	}
	return outcome, nil
}

func outcomeMatchesSubmittedDecision(outcome core.OperationOutcome, decision core.LifecycleDecision) bool {
	return decision.Status == core.DecisionStatusSubmitted &&
		decision.ID == outcome.DecisionID &&
		decision.OrganizationID == outcome.OrganizationID &&
		decision.OwnerAccountID == outcome.OwnerAccountID &&
		decision.ActingPrincipalID == outcome.ActingPrincipalID &&
		decision.ConnectorID == outcome.ConnectorID &&
		decision.ConnectionID == outcome.ConnectionID &&
		decision.Target != nil && decision.Target.SameIdentity(outcome.Target) &&
		decision.OperationID == outcome.OperationID &&
		decision.LogicalOperationID == outcome.LogicalOperationID
}

func decisionIdentityExists(db *gorm.DB, record lifecycleDecisionRecord) (bool, error) {
	var count int64
	query := db.Model(&lifecycleDecisionRecord{})
	if record.LogicalOperationID != nil && *record.LogicalOperationID != "" {
		query = query.Where("id = ? OR (organization_id = ? AND logical_operation_id = ? AND decision = ? AND status = ?)",
			record.ID, record.OrganizationID, *record.LogicalOperationID, record.Decision, record.Status)
	} else {
		query = query.Where("id = ? OR (organization_id = ? AND (logical_operation_id IS NULL OR logical_operation_id = '') AND correlation_id = ? AND decision = ? AND status = ?)",
			record.ID, record.OrganizationID, record.CorrelationID, record.Decision, record.Status)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func terminalOutcomeExists(db *gorm.DB, record operationOutcomeRecord) (bool, error) {
	var count int64
	err := db.Model(&operationOutcomeRecord{}).
		Where("organization_id = ? AND owner_account_id = ?", record.OrganizationID, record.OwnerAccountID).
		Where(
			"id = ? OR decision_id = ? OR logical_operation_id = ?",
			record.ID,
			record.DecisionID,
			record.LogicalOperationID,
		).
		Count(&count).Error
	return count > 0, err
}

func findConvergedOutcome(
	db *gorm.DB,
	unknown core.OperationOutcome,
) (core.OperationOutcome, bool, error) {
	var record operationOutcomeRecord
	err := db.
		Where("organization_id = ? AND owner_account_id = ?", unknown.OrganizationID, unknown.OwnerAccountID).
		Where("decision_id = ? OR logical_operation_id = ?", unknown.DecisionID, unknown.LogicalOperationID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return core.OperationOutcome{}, false, nil
	}
	if err != nil {
		return core.OperationOutcome{}, false, internalError()
	}
	outcome, err := outcomeFromRecord(record)
	if err != nil {
		return core.OperationOutcome{}, false, err
	}
	if !sameOutcomeAuthority(outcome, unknown) {
		return core.OperationOutcome{}, false, internalError()
	}
	return outcome, true, nil
}

func validUnknownConvergenceOutcome(outcome core.OperationOutcome) bool {
	return outcome.Validate() == nil && outcome.Status == core.OutcomeStatusUnknown &&
		outcome.ProviderRequestID == "" && outcome.DiagnosticCode == string(core.ErrorCodeOutcomeUnknown)
}

func sameOutcomeAuthority(left, right core.OperationOutcome) bool {
	return left.DecisionID == right.DecisionID && left.OrganizationID == right.OrganizationID &&
		left.OwnerAccountID == right.OwnerAccountID && left.ActingPrincipalID == right.ActingPrincipalID &&
		left.ConnectorID == right.ConnectorID && left.ConnectionID == right.ConnectionID &&
		left.Target.SameIdentity(right.Target) && left.OperationID == right.OperationID &&
		left.LogicalOperationID == right.LogicalOperationID
}

func encodeSafeDetails(details map[string]any) (datatypes.JSON, error) {
	if details == nil || !safeDetailValue(reflect.ValueOf(details), 0) {
		return nil, invalidInputError()
	}
	encoded, err := json.Marshal(details)
	if err != nil || len(encoded) > maxAuditDetailsBytes {
		return nil, invalidInputError()
	}
	return datatypes.JSON(encoded), nil
}

func safeDetailValue(value reflect.Value, depth int) bool {
	if depth > 8 || !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return true
		}
		if errValue, ok := value.Interface().(error); ok && errValue != nil {
			return false
		}
		return safeDetailValue(value.Elem(), depth+1)
	}
	switch value.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String || value.Len() > 256 {
			return false
		}
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key().String()
			if !validDetailKey(key) || !safeDetailValue(iterator.Value(), depth+1) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 || value.Len() > 256 {
			return false
		}
		for index := 0; index < value.Len(); index++ {
			if !safeDetailValue(value.Index(index), depth+1) {
				return false
			}
		}
		return true
	case reflect.Pointer:
		return value.IsNil()
	default:
		return false
	}
}

func validDetailKey(key string) bool {
	if !validRequiredText(key, 128) {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(key))
	parts := detailKeySeparator.Split(normalized, -1)
	for _, part := range parts {
		switch part {
		case "key", "secret", "token", "credential", "credentials", "password", "authorization", "cookie", "envelope", "cause":
			return false
		}
	}
	compact := strings.Join(parts, "")
	for _, forbidden := range []string{"apikey", "accesstoken", "refreshtoken", "clientsecret", "credentialvalue", "providercause"} {
		if strings.Contains(compact, forbidden) {
			return false
		}
	}
	return true
}

func validOwnerScope(organizationID, ownerAccountID uuid.UUID) bool {
	return organizationID != uuid.Nil && ownerAccountID != uuid.Nil
}

func validRequiredText(value string, maximum int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len(value) <= maximum && !strings.ContainsAny(value, "\x00\r\n")
}

func validOptionalText(value string, maximum int) bool {
	return value == "" || validRequiredText(value, maximum)
}

func validOptionalOpaqueText(value string, maximum int) bool {
	return validOptionalText(value, maximum) && !strings.ContainsAny(value, " \t")
}

func validOptionalCode(value string, maximum int) bool {
	if value == "" {
		return true
	}
	if !validRequiredText(value, maximum) {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func normalizedCreatedAt(createdAt time.Time) time.Time {
	if createdAt.IsZero() {
		return time.Now().UTC()
	}
	return createdAt.UTC()
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func optionalUUID(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	copy := value
	return &copy
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dereferenceUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func preserveConnectorError(err error) error {
	if err == nil {
		return nil
	}
	var connectorError *core.Error
	if errors.As(err, &connectorError) {
		return connectorError
	}
	return internalError()
}

func invalidInputError() error {
	return core.NewError(core.ErrorCodeInvalidInput, false, nil)
}

func notFoundError() error {
	return core.NewError(core.ErrorCodeNotFound, false, nil)
}

func conflictError() error {
	return core.NewError(core.ErrorCodeConflict, false, nil)
}

func internalError() error {
	return core.NewError(core.ErrorCodeInternal, true, nil)
}
