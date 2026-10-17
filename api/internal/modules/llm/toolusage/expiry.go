package toolusage

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// ExpirePending records the absence of facts, never a quantity or a charge.
// A later trusted fact can still resolve the same dedupe identity.
func (r *Repository) ExpirePending(ctx context.Context, cutoff time.Time) (int64, error) {
	var events []Event
	if err := r.db.WithContext(ctx).Where("billing_lane = ? AND status = ? AND quantity = 0 AND first_observed_at < ?", ownedLane, StatusPending, cutoff).Limit(500).Find(&events).Error; err != nil {
		return 0, err
	}
	var changed int64
	for _, e := range events {
		details := map[string]any{}
		_ = json.Unmarshal(e.Details, &details)
		details["free_reason"] = "usage_timeout"
		encoded, err := json.Marshal(details)
		if err != nil {
			return changed, err
		}
		result := r.db.WithContext(ctx).Model(&Event{}).Where("id = ? AND status = ? AND quantity = 0", e.ID, StatusPending).Updates(map[string]any{"status": StatusFailed, "details": string(encoded), "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return changed, result.Error
		}
		changed += result.RowsAffected
	}
	return changed, nil
}
func RunExpiryWorker(ctx context.Context, db *gorm.DB) {
	wait := 5 * time.Minute
	if seconds, err := strconv.Atoi(os.Getenv("LLM_TOOL_USAGE_WAIT_SECONDS")); err == nil && seconds > 0 && seconds <= 86400 {
		wait = time.Duration(seconds) * time.Second
	}
	interval := wait / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := NewRepository(db).ExpirePending(sweepCtx, time.Now().UTC().Add(-wait))
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("[ToolUsage] expire pending facts failed: %v", err)
			}
		}
	}
}
