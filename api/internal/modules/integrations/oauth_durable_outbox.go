package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	oauthRecoveryStatusPending    = "pending"
	oauthRecoveryStatusProcessing = "processing"
	oauthRecoveryStatusDeadLetter = "dead_letter"
)

// IntegrationOAuthRecoveryOperation is the durable source of truth for OAuth
// cleanup which must survive both a process restart and a Redis outage. The
// payload contains encrypted envelopes only while recovery remains actionable;
// acknowledgement atomically erases it and retains the remaining columns as a
// secret-free audit tombstone. The record deliberately has no foreign key to a
// connection or OAuth client configuration because both may be deleted before
// provider revocation succeeds.
type IntegrationOAuthRecoveryOperation struct {
	ID             string         `gorm:"size:80;primaryKey"`
	Kind           string         `gorm:"size:16;not null"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null"`
	ConnectionID   uuid.UUID      `gorm:"type:uuid;not null"`
	IntegrationID  string         `gorm:"size:64;not null"`
	DriverID       string         `gorm:"size:64;not null"`
	AuthMethodID   string         `gorm:"size:128;not null"`
	Payload        datatypes.JSON `gorm:"type:jsonb"`
	Status         string         `gorm:"size:24;not null"`
	Attempts       int            `gorm:"not null;default:0"`
	AvailableAt    time.Time      `gorm:"not null"`
	LeaseOwner     *uuid.UUID     `gorm:"type:uuid"`
	LeaseUntil     *time.Time
	LastErrorCode  *string `gorm:"size:64"`
	DeadLetteredAt *time.Time
	AcknowledgedAt *time.Time
	AcknowledgedBy *uuid.UUID `gorm:"type:uuid"`
	ResolutionCode *string    `gorm:"size:64"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (IntegrationOAuthRecoveryOperation) TableName() string {
	return "integration_oauth_recovery_operations"
}

type OAuthRevocationDeleteRepository interface {
	DeleteWithOAuthRevocation(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		*uuid.UUID,
		OAuthRecoveryTask,
	) error
}

type OAuthRecoveryImpactRepository interface {
	CountPendingRevocations(context.Context, uuid.UUID, string, []string) (int64, error)
}

type OAuthRecoveryRemediationItem struct {
	OperationRef  string    `json:"operation_ref"`
	IntegrationID string    `json:"integration_id"`
	AuthMethodID  string    `json:"auth_method_id"`
	ReasonCode    string    `json:"reason_code"`
	Attempts      int       `json:"attempts"`
	CreatedAt     time.Time `json:"created_at"`
	FailedAt      time.Time `json:"failed_at"`
}

type OAuthRecoveryAdminSummary struct {
	PendingRevocations    int64                          `json:"pending_revocations"`
	ManualActionRequired  int64                          `json:"manual_action_required"`
	FailedRevocations     int64                          `json:"failed_revocations"`
	UnresolvedDeadLetters int64                          `json:"unresolved_dead_letters"`
	RemediationOperations []OAuthRecoveryRemediationItem `json:"remediation_operations"`
}

type OAuthRecoveryAdminRepository interface {
	OAuthRecoverySummary(context.Context, uuid.UUID, int) (OAuthRecoveryAdminSummary, error)
	AcknowledgeOAuthRecovery(context.Context, uuid.UUID, string, uuid.UUID, string) error
}

const (
	OAuthRecoveryResolutionProviderAccessRemoved = "provider_access_removed"
	OAuthRecoveryResolutionTokenExpired          = "token_confirmed_expired"
	oauthRecoveryManualReason                    = "manual_provider_revocation_required"
)

// DatabaseOAuthRecoveryOutbox uses PostgreSQL row locks and expiring leases so
// multiple API instances can safely drain the same durable queue.
type DatabaseOAuthRecoveryOutbox struct {
	db            *gorm.DB
	workerID      uuid.UUID
	now           func() time.Time
	leaseDuration time.Duration
	retention     time.Duration
	maxAttempts   int
}

func NewDatabaseOAuthRecoveryOutbox(db *gorm.DB) *DatabaseOAuthRecoveryOutbox {
	return &DatabaseOAuthRecoveryOutbox{
		db:            db,
		workerID:      uuid.New(),
		now:           func() time.Time { return time.Now().UTC() },
		leaseDuration: oauthRecoveryLease,
		retention:     oauthRecoveryRetention,
		maxAttempts:   oauthRecoveryMaxAttempts,
	}
}

func (outbox *DatabaseOAuthRecoveryOutbox) Enqueue(ctx context.Context, task OAuthRecoveryTask) error {
	if outbox == nil || outbox.db == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	task = normalizeOAuthRecoveryTask(task, outbox.now())
	if err := validateOAuthRecoveryTask(task); err != nil {
		return err
	}
	record, err := oauthRecoveryRecord(task, outbox.now())
	if err != nil {
		return err
	}
	result := outbox.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(record)
	if result.Error != nil {
		return fmt.Errorf("enqueue integration OAuth durable recovery task: %w", result.Error)
	}
	return nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) Claim(ctx context.Context, limit int64) ([]OAuthRecoveryTask, error) {
	if outbox == nil || outbox.db == nil {
		return nil, fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	if limit <= 0 {
		return nil, nil
	}
	if limit > 100 {
		limit = 100
	}
	now := outbox.now()
	lease := outbox.leaseDuration
	if lease <= 0 {
		lease = oauthRecoveryLease
	}
	var records []IntegrationOAuthRecoveryOperation
	err := outbox.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"(status = ? AND available_at <= ?) OR (status = ? AND lease_until <= ?)",
				oauthRecoveryStatusPending,
				now,
				oauthRecoveryStatusProcessing,
				now,
			).
			Order("available_at ASC, created_at ASC").
			Limit(int(limit))
		if err := query.Find(&records).Error; err != nil {
			return fmt.Errorf("claim integration OAuth durable recovery tasks: %w", err)
		}
		if len(records) == 0 {
			return nil
		}
		ids := make([]string, 0, len(records))
		for index := range records {
			ids = append(ids, records[index].ID)
		}
		leaseUntil := now.Add(lease)
		result := tx.Model(&IntegrationOAuthRecoveryOperation{}).
			Where("id IN ? AND ((status = ? AND available_at <= ?) OR (status = ? AND lease_until <= ?))",
				ids,
				oauthRecoveryStatusPending,
				now,
				oauthRecoveryStatusProcessing,
				now,
			).
			Updates(map[string]any{
				"status":      oauthRecoveryStatusProcessing,
				"lease_owner": outbox.workerID,
				"lease_until": leaseUntil,
				"updated_at":  now,
			})
		if result.Error != nil {
			return fmt.Errorf("lease integration OAuth durable recovery tasks: %w", result.Error)
		}
		if result.RowsAffected != int64(len(records)) {
			return fmt.Errorf("integration OAuth durable recovery lease changed concurrently")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	tasks := make([]OAuthRecoveryTask, 0, len(records))
	var decodeErr error
	for _, record := range records {
		task, err := decodeOAuthRecoveryRecord(record)
		if err != nil {
			decodeErr = errors.Join(decodeErr, err)
			_ = outbox.deadLetterRecord(ctx, record.ID, "invalid_task")
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, decodeErr
}

func (outbox *DatabaseOAuthRecoveryOutbox) Get(ctx context.Context, id string) (*OAuthRecoveryTask, error) {
	if outbox == nil || outbox.db == nil {
		return nil, fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("integration OAuth recovery task id is required")
	}
	var record IntegrationOAuthRecoveryOperation
	err := outbox.db.WithContext(ctx).
		Where("id = ? AND status <> ?", id, oauthRecoveryStatusDeadLetter).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, redis.Nil
	}
	if err != nil {
		return nil, fmt.Errorf("read integration OAuth durable recovery task: %w", err)
	}
	task, err := decodeOAuthRecoveryRecord(record)
	if err != nil {
		_ = outbox.deadLetterRecord(ctx, record.ID, "invalid_task")
		return nil, err
	}
	return &task, nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) Ack(ctx context.Context, id string) error {
	if outbox == nil || outbox.db == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	result := outbox.db.WithContext(ctx).
		Where(
			"id = ? AND (status = ? OR (status = ? AND lease_owner = ?))",
			id,
			oauthRecoveryStatusPending,
			oauthRecoveryStatusProcessing,
			outbox.workerID,
		).
		Delete(&IntegrationOAuthRecoveryOperation{})
	if result.Error != nil {
		return fmt.Errorf("complete integration OAuth durable recovery task: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("integration OAuth durable recovery lease was lost")
	}
	return nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) Retry(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	if outbox == nil || outbox.db == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	task.Attempts++
	maxAttempts := outbox.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = oauthRecoveryMaxAttempts
	}
	retention := outbox.retention
	if retention <= 0 {
		retention = oauthRecoveryRetention
	}
	now := outbox.now()
	if task.Attempts >= maxAttempts || now.Sub(task.CreatedAt) >= retention {
		return outbox.DeadLetter(ctx, task, reason)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("encode integration OAuth durable recovery retry: %w", err)
	}
	safeReason := oauthRecoverySafeReason(reason)
	result := outbox.db.WithContext(ctx).
		Model(&IntegrationOAuthRecoveryOperation{}).
		Where("id = ? AND status = ? AND lease_owner = ?", task.ID, oauthRecoveryStatusProcessing, outbox.workerID).
		Updates(map[string]any{
			"payload":         datatypes.JSON(payload),
			"status":          oauthRecoveryStatusPending,
			"attempts":        task.Attempts,
			"available_at":    now.Add(oauthRecoveryRetryDelay(task.Attempts)),
			"lease_owner":     nil,
			"lease_until":     nil,
			"last_error_code": safeReason,
			"updated_at":      now,
		})
	if result.Error != nil {
		return fmt.Errorf("retry integration OAuth durable recovery task: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("integration OAuth durable recovery lease was lost")
	}
	return nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) DeadLetter(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	return outbox.deadLetterRecord(ctx, task.ID, reason)
}

func (outbox *DatabaseOAuthRecoveryOutbox) OAuthRecoverySummary(
	ctx context.Context,
	organizationID uuid.UUID,
	limit int,
) (OAuthRecoveryAdminSummary, error) {
	summary := OAuthRecoveryAdminSummary{RemediationOperations: []OAuthRecoveryRemediationItem{}}
	if outbox == nil || outbox.db == nil {
		return summary, fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	if organizationID == uuid.Nil {
		return summary, fmt.Errorf("integration OAuth recovery organization is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	base := outbox.db.WithContext(ctx).
		Model(&IntegrationOAuthRecoveryOperation{}).
		Where("organization_id = ? AND kind = ?", organizationID, OAuthRecoveryRevoke)
	if err := base.Where("status IN ?", []string{oauthRecoveryStatusPending, oauthRecoveryStatusProcessing}).
		Count(&summary.PendingRevocations).Error; err != nil {
		return summary, fmt.Errorf("count integration OAuth pending revocations: %w", err)
	}
	unresolved := base.Where("status = ? AND acknowledged_at IS NULL", oauthRecoveryStatusDeadLetter)
	if err := unresolved.Count(&summary.UnresolvedDeadLetters).Error; err != nil {
		return summary, fmt.Errorf("count integration OAuth unresolved revocations: %w", err)
	}
	if err := unresolved.Where("last_error_code = ?", oauthRecoveryManualReason).
		Count(&summary.ManualActionRequired).Error; err != nil {
		return summary, fmt.Errorf("count integration OAuth manual revocations: %w", err)
	}
	summary.FailedRevocations = summary.UnresolvedDeadLetters - summary.ManualActionRequired

	var records []IntegrationOAuthRecoveryOperation
	if err := unresolved.
		Order("dead_lettered_at DESC, created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return summary, fmt.Errorf("list integration OAuth remediation operations: %w", err)
	}
	for _, record := range records {
		failedAt := record.UpdatedAt.UTC()
		if record.DeadLetteredAt != nil {
			failedAt = record.DeadLetteredAt.UTC()
		}
		reasonCode := ""
		if record.LastErrorCode != nil {
			reasonCode = strings.TrimSpace(*record.LastErrorCode)
		}
		summary.RemediationOperations = append(summary.RemediationOperations, OAuthRecoveryRemediationItem{
			OperationRef: record.ID, IntegrationID: record.IntegrationID,
			AuthMethodID: record.AuthMethodID, ReasonCode: reasonCode,
			Attempts: record.Attempts, CreatedAt: record.CreatedAt.UTC(), FailedAt: failedAt,
		})
	}
	return summary, nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) AcknowledgeOAuthRecovery(
	ctx context.Context,
	organizationID uuid.UUID,
	operationRef string,
	actorID uuid.UUID,
	resolutionCode string,
) error {
	if outbox == nil || outbox.db == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	operationRef = strings.TrimSpace(operationRef)
	resolutionCode = strings.ToLower(strings.TrimSpace(resolutionCode))
	if organizationID == uuid.Nil || actorID == uuid.Nil || operationRef == "" {
		return fmt.Errorf("integration OAuth remediation acknowledgement is invalid")
	}
	switch resolutionCode {
	case OAuthRecoveryResolutionProviderAccessRemoved, OAuthRecoveryResolutionTokenExpired:
	default:
		return fmt.Errorf("integration OAuth remediation resolution is invalid")
	}
	now := outbox.now()
	result := outbox.db.WithContext(ctx).
		Model(&IntegrationOAuthRecoveryOperation{}).
		Where(
			"id = ? AND organization_id = ? AND kind = ? AND status = ? AND acknowledged_at IS NULL",
			operationRef,
			organizationID,
			OAuthRecoveryRevoke,
			oauthRecoveryStatusDeadLetter,
		).
		Updates(map[string]any{
			"acknowledged_at": now,
			"acknowledged_by": actorID,
			"resolution_code": resolutionCode,
			"payload":         nil,
			"updated_at":      now,
		})
	if result.Error != nil {
		return fmt.Errorf("acknowledge integration OAuth remediation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionNotFound
	}
	logger.InfoContext(
		ctx,
		"integration OAuth remediation acknowledged",
		"operation_ref", operationRef,
		"organization_id", organizationID.String(),
		"resolution_code", resolutionCode,
	)
	return nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) CountPendingRevocations(
	ctx context.Context,
	organizationID uuid.UUID,
	integrationID string,
	authMethodIDs []string,
) (int64, error) {
	if outbox == nil || outbox.db == nil {
		return 0, fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	integrationID = normalizeOAuthIdentifier(integrationID)
	authMethodIDs = normalizeCatalogStringList(authMethodIDs, 64)
	if organizationID == uuid.Nil || integrationID == "" || len(authMethodIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := outbox.db.WithContext(ctx).
		Model(&IntegrationOAuthRecoveryOperation{}).
		Where(
			"organization_id = ? AND integration_id = ? AND auth_method_id IN ? AND kind = ? AND status IN ?",
			organizationID,
			integrationID,
			authMethodIDs,
			OAuthRecoveryRevoke,
			[]string{oauthRecoveryStatusPending, oauthRecoveryStatusProcessing},
		).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count integration OAuth pending revocations: %w", err)
	}
	return count, nil
}

func (outbox *DatabaseOAuthRecoveryOutbox) deadLetterRecord(ctx context.Context, id, reason string) error {
	if outbox == nil || outbox.db == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	now := outbox.now()
	safeReason := oauthRecoverySafeReason(reason)
	result := outbox.db.WithContext(ctx).
		Model(&IntegrationOAuthRecoveryOperation{}).
		Where(
			"id = ? AND status = ? AND lease_owner = ?",
			strings.TrimSpace(id),
			oauthRecoveryStatusProcessing,
			outbox.workerID,
		).
		Updates(map[string]any{
			"status":           oauthRecoveryStatusDeadLetter,
			"lease_owner":      nil,
			"lease_until":      nil,
			"last_error_code":  safeReason,
			"dead_lettered_at": now,
			"updated_at":       now,
		})
	if result.Error != nil {
		return fmt.Errorf("dead-letter integration OAuth durable recovery task: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("integration OAuth durable recovery lease was lost")
	}
	logger.WarnContext(
		ctx,
		"integration OAuth recovery operation entered dead letter state",
		"operation_ref", strings.TrimSpace(id),
		"reason_code", safeReason,
	)
	return nil
}

func oauthRecoveryRecord(task OAuthRecoveryTask, availableAt time.Time) (*IntegrationOAuthRecoveryOperation, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("encode integration OAuth durable recovery task: %w", err)
	}
	return &IntegrationOAuthRecoveryOperation{
		ID:             task.ID,
		Kind:           string(task.Kind),
		OrganizationID: task.OrganizationID,
		ConnectionID:   task.ConnectionID,
		IntegrationID:  task.IntegrationID,
		DriverID:       task.DriverID,
		AuthMethodID:   task.AuthMethodID,
		Payload:        datatypes.JSON(payload),
		Status:         oauthRecoveryStatusPending,
		Attempts:       task.Attempts,
		AvailableAt:    availableAt.UTC(),
		CreatedAt:      task.CreatedAt.UTC(),
		UpdatedAt:      task.CreatedAt.UTC(),
	}, nil
}

func decodeOAuthRecoveryRecord(record IntegrationOAuthRecoveryOperation) (OAuthRecoveryTask, error) {
	var task OAuthRecoveryTask
	if err := json.Unmarshal(record.Payload, &task); err != nil {
		return OAuthRecoveryTask{}, fmt.Errorf("decode integration OAuth durable recovery task: %w", err)
	}
	task.Attempts = record.Attempts
	if err := validateOAuthRecoveryTask(task); err != nil {
		return OAuthRecoveryTask{}, err
	}
	if task.ID != record.ID ||
		string(task.Kind) != record.Kind ||
		task.OrganizationID != record.OrganizationID ||
		task.ConnectionID != record.ConnectionID ||
		task.IntegrationID != record.IntegrationID ||
		task.DriverID != record.DriverID ||
		task.AuthMethodID != record.AuthMethodID {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth durable recovery identity does not match payload")
	}
	return task, nil
}

// SplitOAuthRecoveryOutbox keeps rotating refresh-token recovery in Redis so
// it remains available during a transient database write failure, while
// revocation uses PostgreSQL as its durable source of truth.
type SplitOAuthRecoveryOutbox struct {
	revocations OAuthRecoveryOutbox
	refreshes   OAuthRecoveryOutbox
}

func NewSplitOAuthRecoveryOutbox(revocations, refreshes OAuthRecoveryOutbox) *SplitOAuthRecoveryOutbox {
	return &SplitOAuthRecoveryOutbox{revocations: revocations, refreshes: refreshes}
}

func (outbox *SplitOAuthRecoveryOutbox) route(kind OAuthRecoveryTaskKind) OAuthRecoveryOutbox {
	if outbox == nil {
		return nil
	}
	if kind == OAuthRecoveryRevoke {
		return outbox.revocations
	}
	return outbox.refreshes
}

func (outbox *SplitOAuthRecoveryOutbox) routeID(id string) OAuthRecoveryOutbox {
	if strings.HasPrefix(strings.TrimSpace(id), "revoke-") {
		return outbox.route(OAuthRecoveryRevoke)
	}
	return outbox.route(OAuthRecoveryRefresh)
}

func (outbox *SplitOAuthRecoveryOutbox) Enqueue(ctx context.Context, task OAuthRecoveryTask) error {
	target := outbox.route(task.Kind)
	if target == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return target.Enqueue(ctx, task)
}

func (outbox *SplitOAuthRecoveryOutbox) Claim(ctx context.Context, limit int64) ([]OAuthRecoveryTask, error) {
	if limit <= 0 {
		return nil, nil
	}
	var tasks []OAuthRecoveryTask
	var resultErr error
	if outbox != nil && outbox.revocations != nil {
		claimed, err := outbox.revocations.Claim(ctx, limit)
		tasks = append(tasks, claimed...)
		resultErr = errors.Join(resultErr, err)
	}
	remaining := limit - int64(len(tasks))
	if remaining > 0 && outbox != nil && outbox.refreshes != nil {
		claimed, err := outbox.refreshes.Claim(ctx, remaining)
		tasks = append(tasks, claimed...)
		resultErr = errors.Join(resultErr, err)
	}
	return tasks, resultErr
}

func (outbox *SplitOAuthRecoveryOutbox) Get(ctx context.Context, id string) (*OAuthRecoveryTask, error) {
	target := outbox.routeID(id)
	if target == nil {
		return nil, fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return target.Get(ctx, id)
}

func (outbox *SplitOAuthRecoveryOutbox) Ack(ctx context.Context, id string) error {
	target := outbox.routeID(id)
	if target == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return target.Ack(ctx, id)
}

func (outbox *SplitOAuthRecoveryOutbox) Retry(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	target := outbox.route(task.Kind)
	if target == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return target.Retry(ctx, task, reason)
}

func (outbox *SplitOAuthRecoveryOutbox) DeadLetter(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	target := outbox.route(task.Kind)
	if target == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return target.DeadLetter(ctx, task, reason)
}

func (outbox *SplitOAuthRecoveryOutbox) CountPendingRevocations(
	ctx context.Context,
	organizationID uuid.UUID,
	integrationID string,
	authMethodIDs []string,
) (int64, error) {
	if outbox == nil || outbox.revocations == nil {
		return 0, nil
	}
	counter, ok := outbox.revocations.(OAuthRecoveryImpactRepository)
	if !ok {
		return 0, nil
	}
	return counter.CountPendingRevocations(ctx, organizationID, integrationID, authMethodIDs)
}

func (outbox *SplitOAuthRecoveryOutbox) OAuthRecoverySummary(
	ctx context.Context,
	organizationID uuid.UUID,
	limit int,
) (OAuthRecoveryAdminSummary, error) {
	if outbox == nil || outbox.revocations == nil {
		return OAuthRecoveryAdminSummary{}, fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	repository, ok := outbox.revocations.(OAuthRecoveryAdminRepository)
	if !ok {
		return OAuthRecoveryAdminSummary{}, fmt.Errorf("integration OAuth recovery administration is unavailable")
	}
	return repository.OAuthRecoverySummary(ctx, organizationID, limit)
}

func (outbox *SplitOAuthRecoveryOutbox) AcknowledgeOAuthRecovery(
	ctx context.Context,
	organizationID uuid.UUID,
	operationRef string,
	actorID uuid.UUID,
	resolutionCode string,
) error {
	if outbox == nil || outbox.revocations == nil {
		return fmt.Errorf("integration OAuth durable recovery outbox is unavailable")
	}
	repository, ok := outbox.revocations.(OAuthRecoveryAdminRepository)
	if !ok {
		return fmt.Errorf("integration OAuth recovery administration is unavailable")
	}
	return repository.AcknowledgeOAuthRecovery(ctx, organizationID, operationRef, actorID, resolutionCode)
}
