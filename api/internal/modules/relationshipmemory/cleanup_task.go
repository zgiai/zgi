package relationshipmemory

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const (
	CleanupTaskType = "system:relationship_memory:cleanup"
	// auditRetention is how long memory events stay for audit.
	auditRetention = 180 * 24 * time.Hour
	// extractionJobRetention is how long rows of the retired consolidation
	// queue are kept before they are dropped.
	extractionJobRetention = 30 * 24 * time.Hour
)

// CleanupTask periodically drops expired undo records, old memory events,
// expired history, and the retired consolidation queue's rows.
type CleanupTask struct{}

func NewCleanupTask() *CleanupTask           { return &CleanupTask{} }
func (*CleanupTask) TaskType() string        { return CleanupTaskType }
func (*CleanupTask) CronSpec() string        { return "" }
func (*CleanupTask) Interval() time.Duration { return 10 * time.Minute }
func (*CleanupTask) Payload() []byte         { return nil }
func (*CleanupTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(5 * time.Minute), asynq.Unique(9 * time.Minute)}
}

type CleanupHandler struct{ adapter *AgentAdapter }

func NewCleanupHandler(db *gorm.DB) *CleanupHandler {
	return &CleanupHandler{adapter: NewAgentAdapter(db)}
}

func (h *CleanupHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.adapter == nil {
		return nil
	}
	return h.adapter.CleanupExpiredRelationshipMemory(ctx, time.Now(), auditRetention)
}
