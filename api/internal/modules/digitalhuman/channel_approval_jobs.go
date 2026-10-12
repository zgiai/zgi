package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelApprovalJobStatus is the durable lifecycle of a channel approval
// reply accepted inside a provider acknowledgement window.
type ChannelApprovalJobStatus string

const (
	ChannelApprovalJobPending   ChannelApprovalJobStatus = "pending"
	ChannelApprovalJobRunning   ChannelApprovalJobStatus = "running"
	ChannelApprovalJobSucceeded ChannelApprovalJobStatus = "succeeded"
	ChannelApprovalJobFailed    ChannelApprovalJobStatus = "failed"
)

const (
	// channelApprovalExecutionTimeout bounds one approval continuation, which
	// may resume a workflow or run the DigitalHuman model.
	channelApprovalExecutionTimeout = 10 * time.Minute
	// channelApprovalLease lets another worker reclaim a job whose worker died.
	channelApprovalLease       = channelApprovalExecutionTimeout + time.Minute
	channelApprovalMaxAttempts = 3
	channelApprovalBatch       = 5
	maxChannelApprovalTextRune = 8000
)

// ChannelApprovalJob holds everything needed to resume a pending approval
// from a channel reply after the provider has been acknowledged. Its ID is the
// inbound event ID, so a provider retry cannot queue the same reply twice.
type ChannelApprovalJob struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	WorkspaceID    uuid.UUID `gorm:"type:uuid;not null"`
	InstallationID uuid.UUID `gorm:"type:uuid;not null"`
	InboundEventID uuid.UUID `gorm:"type:uuid;not null"`
	ActorID        uuid.UUID `gorm:"type:uuid;not null"`
	DigitalHumanID uuid.UUID `gorm:"column:digital_human_id;type:uuid;not null"`
	ConversationID uuid.UUID `gorm:"type:uuid;not null"`
	// SenderExternalID and ConversationType identify who answered, so the
	// worker can reject a group member answering someone else's approval.
	SenderExternalID string                          `gorm:"column:sender_external_id;type:varchar(255);not null"`
	ConversationType channelruntime.ConversationType `gorm:"type:varchar(16);not null"`
	Text             string                          `gorm:"type:text;not null"`
	Status           ChannelApprovalJobStatus        `gorm:"type:varchar(16);not null"`
	AttemptCount     int                             `gorm:"not null"`
	AvailableAt      time.Time                       `gorm:"not null"`
	ClaimedAt        *time.Time
	CompletedAt      *time.Time
	ErrorCode        string    `gorm:"type:varchar(128);not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (ChannelApprovalJob) TableName() string { return "digital_human_channel_approval_jobs" }

// ChannelApprovalJobStore persists channel approval replies so their
// execution survives process restarts and lost queue wake-ups.
type ChannelApprovalJobStore interface {
	CreateChannelApprovalJob(context.Context, *ChannelApprovalJob) (bool, error)
	ClaimChannelApprovalJobs(context.Context, int, time.Time) ([]*ChannelApprovalJob, error)
	ListDueChannelApprovalJobs(context.Context, int, time.Time) ([]*ChannelApprovalJob, error)
	FinishChannelApprovalJob(context.Context, uuid.UUID, ChannelApprovalJobStatus, string, time.Time) error
	RetryChannelApprovalJob(context.Context, uuid.UUID, time.Time, string, time.Time) error
}

// ChannelApprovalDispatcher is the low-latency wake-up for a persisted
// approval job. Correctness comes from the durable sweep.
type ChannelApprovalDispatcher interface {
	EnqueueChannelApproval(context.Context, uuid.UUID, int) error
}

var (
	_ ChannelApprovalJobStore   = (*GormRepository)(nil)
	_ ChannelApprovalDispatcher = (*AsynqChannelTransientDispatcher)(nil)
)

func (r *GormRepository) CreateChannelApprovalJob(ctx context.Context, job *ChannelApprovalJob) (bool, error) {
	if job == nil || job.ID == uuid.Nil || job.InboundEventID != job.ID || job.DigitalHumanID == uuid.Nil ||
		job.ConversationID == uuid.Nil || job.ActorID == uuid.Nil || strings.TrimSpace(job.Text) == "" {
		return false, ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, DoNothing: true,
	}).Create(job)
	if result.Error != nil {
		return false, fmt.Errorf("create DigitalHuman channel approval job: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ClaimChannelApprovalJobs leases due jobs. A running job whose lease expired
// (its worker died) is due again.
func (r *GormRepository) ClaimChannelApprovalJobs(ctx context.Context, limit int, now time.Time) ([]*ChannelApprovalJob, error) {
	if limit <= 0 {
		limit = channelApprovalBatch
	}
	claimed := make([]*ChannelApprovalJob, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var due []*ChannelApprovalJob
		query := tx.Where("status IN ? AND available_at <= ?",
			[]ChannelApprovalJobStatus{ChannelApprovalJobPending, ChannelApprovalJobRunning}, now).
			Order("available_at ASC, created_at ASC").Limit(limit)
		if r.db.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&due).Error; err != nil {
			return err
		}
		for _, job := range due {
			job.Status, job.AttemptCount, job.ClaimedAt = ChannelApprovalJobRunning, job.AttemptCount+1, &now
			job.AvailableAt, job.UpdatedAt = now.Add(channelApprovalLease), now
			if err := tx.Model(&ChannelApprovalJob{}).Where("id = ?", job.ID).Updates(map[string]any{
				"status": job.Status, "attempt_count": job.AttemptCount, "claimed_at": now,
				"available_at": job.AvailableAt, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			claimed = append(claimed, job)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim DigitalHuman channel approval jobs: %w", err)
	}
	return claimed, nil
}

func (r *GormRepository) ListDueChannelApprovalJobs(ctx context.Context, limit int, now time.Time) ([]*ChannelApprovalJob, error) {
	if limit <= 0 {
		limit = channelApprovalBatch
	}
	var due []*ChannelApprovalJob
	if err := r.db.WithContext(ctx).Select("id", "attempt_count").Where("status IN ? AND available_at <= ?",
		[]ChannelApprovalJobStatus{ChannelApprovalJobPending, ChannelApprovalJobRunning}, now).
		Order("available_at ASC, created_at ASC").Limit(limit).Find(&due).Error; err != nil {
		return nil, fmt.Errorf("list due DigitalHuman channel approval jobs: %w", err)
	}
	return due, nil
}

func (r *GormRepository) FinishChannelApprovalJob(ctx context.Context, id uuid.UUID, status ChannelApprovalJobStatus, errorCode string, now time.Time) error {
	if id == uuid.Nil || (status != ChannelApprovalJobSucceeded && status != ChannelApprovalJobFailed) {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Model(&ChannelApprovalJob{}).
		Where("id = ? AND status = ?", id, ChannelApprovalJobRunning).
		Updates(map[string]any{
			"status": status, "error_code": boundedRunes(errorCode, 128), "completed_at": now,
			"claimed_at": nil, "updated_at": now,
		}).Error
}

func (r *GormRepository) RetryChannelApprovalJob(ctx context.Context, id uuid.UUID, availableAt time.Time, errorCode string, now time.Time) error {
	if id == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Model(&ChannelApprovalJob{}).
		Where("id = ? AND status = ?", id, ChannelApprovalJobRunning).
		Updates(map[string]any{
			"status": ChannelApprovalJobPending, "available_at": availableAt,
			"error_code": boundedRunes(errorCode, 128), "claimed_at": nil, "updated_at": now,
		}).Error
}

// channelApprovalRetryDelay backs off retryable approval failures.
func channelApprovalRetryDelay(attempt int) time.Duration {
	delay := 30 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

// channelApprovalErrorCode keeps persisted failure codes free of provider or
// database text.
func channelApprovalErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrRunNotFound):
		return "run_not_found"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	default:
		return "continuation_failed"
	}
}

// admitChannelApproval runs inside the provider acknowledgement window. It
// only checks for a pending approval and persists the reply; the approval
// itself (which may resume a workflow or run the model) executes later on a
// worker with its own deadline.
func (service *ChannelService) admitChannelApproval(
	ctx context.Context,
	runtime channelTaskApprovalRuntime,
	installation *channelruntime.Installation,
	route *ChannelRoute,
	conversationID uuid.UUID,
	stored *channelruntime.InboundEvent,
	message *channelruntime.InboundMessage,
	now time.Time,
) (bool, error) {
	scope := Scope{OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID, ActorID: installation.CreatedBy}
	subjectID := route.Binding.SubjectID
	reply := ChannelApprovalReply{
		Text:             channelInvocationQuery(message),
		SenderExternalID: strings.TrimSpace(message.SenderExternalID),
		ConversationType: message.ConversationType,
	}
	if service.approvals == nil {
		// Without a durable job store (minimal deployments and unit tests) the
		// approval can only be resolved inline.
		return runtime.ContinueChannelTaskApproval(ctx, scope, subjectID, conversationID, reply)
	}
	pending, err := runtime.HasPendingChannelApproval(ctx, scope, subjectID, conversationID)
	if err != nil || !pending {
		return false, err
	}
	text := boundedRunes(strings.TrimSpace(reply.Text), maxChannelApprovalTextRune)
	if text == "" {
		return false, nil
	}
	job := &ChannelApprovalJob{
		ID: stored.ID, InboundEventID: stored.ID,
		OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, ActorID: installation.CreatedBy,
		DigitalHumanID: subjectID, ConversationID: conversationID, Text: text,
		SenderExternalID: reply.SenderExternalID, ConversationType: reply.ConversationType,
		Status: ChannelApprovalJobPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	created, err := service.approvals.CreateChannelApprovalJob(ctx, job)
	if err != nil {
		return false, err
	}
	if created {
		service.wakeChannelApproval(ctx, job.ID, job.AttemptCount)
	}
	return true, nil
}

// DispatchChannelApprovals claims and executes due approval jobs.
func (service *ChannelService) DispatchChannelApprovals(ctx context.Context, limit int) error {
	_, err := service.dispatchChannelApprovals(ctx, limit)
	return err
}

func (service *ChannelService) dispatchChannelApprovals(ctx context.Context, limit int) (int, error) {
	if service == nil || service.approvals == nil {
		return 0, nil
	}
	runtime, ok := service.runtime.(channelTaskApprovalRuntime)
	if !ok {
		return 0, nil
	}
	jobs, err := service.approvals.ClaimChannelApprovalJobs(ctx, limit, service.now())
	if err != nil {
		return 0, err
	}
	var combined error
	for _, job := range jobs {
		combined = errors.Join(combined, service.runChannelApprovalJob(ctx, runtime, job))
	}
	return len(jobs), combined
}

func (service *ChannelService) runChannelApprovalJob(ctx context.Context, runtime channelTaskApprovalRuntime, job *ChannelApprovalJob) error {
	execCtx, cancel := context.WithTimeout(ctx, channelApprovalExecutionTimeout)
	defer cancel()
	// The approval re-checks the pending message, so a retry after a partial
	// attempt that already resolved it is a no-op rather than a duplicate.
	_, err := runtime.ContinueChannelTaskApproval(
		execCtx,
		Scope{OrganizationID: job.OrganizationID, WorkspaceID: job.WorkspaceID, ActorID: job.ActorID},
		job.DigitalHumanID, job.ConversationID,
		ChannelApprovalReply{Text: job.Text, SenderExternalID: job.SenderExternalID, ConversationType: job.ConversationType},
	)
	persistCtx := context.WithoutCancel(ctx)
	now := service.now()
	if err == nil {
		return service.approvals.FinishChannelApprovalJob(persistCtx, job.ID, ChannelApprovalJobSucceeded, "", now)
	}
	code := channelApprovalErrorCode(err)
	logger.WarnContext(ctx, "DigitalHuman channel approval failed",
		"job_id", job.ID.String(), "attempt", job.AttemptCount, "error_code", code, err)
	if job.AttemptCount < channelApprovalMaxAttempts {
		return service.approvals.RetryChannelApprovalJob(persistCtx, job.ID, now.Add(channelApprovalRetryDelay(job.AttemptCount)), code, now)
	}
	return service.approvals.FinishChannelApprovalJob(persistCtx, job.ID, ChannelApprovalJobFailed, code, now)
}

// wakeChannelApproval asks a worker to run the job now. A lost wake-up only
// delays the job until the delivery sweep re-wakes it.
func (service *ChannelService) wakeChannelApproval(ctx context.Context, id uuid.UUID, attempt int) {
	dispatcher, ok := service.transient.(ChannelApprovalDispatcher)
	if !ok {
		service.runChannelApprovalsInProcess()
		return
	}
	if err := dispatcher.EnqueueChannelApproval(ctx, id, attempt); err != nil {
		logger.WarnContext(ctx, "DigitalHuman channel approval wake-up was delayed", "job_id", id.String(), err)
	}
}

// wakeDueChannelApprovals is the durable recovery path: it re-wakes jobs whose
// wake-up was lost, whose retry delay elapsed, or whose worker lease expired.
func (service *ChannelService) wakeDueChannelApprovals(ctx context.Context, now time.Time) error {
	if service == nil || service.approvals == nil {
		return nil
	}
	if _, ok := service.transient.(ChannelApprovalDispatcher); !ok {
		service.runChannelApprovalsInProcess()
		return nil
	}
	due, err := service.approvals.ListDueChannelApprovalJobs(ctx, channelApprovalBatch, now)
	if err != nil {
		return err
	}
	for _, job := range due {
		service.wakeChannelApproval(ctx, job.ID, job.AttemptCount)
	}
	return nil
}

// runChannelApprovalsInProcess is the fallback when no queue dispatcher is
// configured: one background loop drains due jobs outside the request.
func (service *ChannelService) runChannelApprovalsInProcess() {
	if !service.approvalWorker.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer service.approvalWorker.Store(false)
		ctx := context.Background()
		for {
			claimed, err := service.dispatchChannelApprovals(ctx, channelApprovalBatch)
			if err != nil {
				logger.WarnContext(ctx, "DigitalHuman channel approval worker stopped", err)
				return
			}
			if claimed == 0 {
				return
			}
		}
	}()
}
