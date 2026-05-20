package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	"gorm.io/gorm"
)

// Compile-time interface check
var _ types.HealthTracker = (*ChannelHealthTracker)(nil)

// ChannelHealthTracker tracks channel health and manages auto-ban
type ChannelHealthTracker struct {
	db               *gorm.DB
	mu               sync.RWMutex
	failureWindows   map[uuid.UUID]*FailureWindow
	windowDuration   time.Duration
	failureThreshold int // Percentage threshold for auto-ban (e.g., 80 means 80%)
}

// FailureWindow tracks failures within a time window
type FailureWindow struct {
	TotalRequests   int
	FailedRequests  int
	WindowStartTime time.Time
}

// NewChannelHealthTracker creates a new channel health tracker
func NewChannelHealthTracker(db *gorm.DB) *ChannelHealthTracker {
	return &ChannelHealthTracker{
		db:               db,
		failureWindows:   make(map[uuid.UUID]*FailureWindow),
		windowDuration:   5 * time.Minute, // 5-minute sliding window
		failureThreshold: 80,              // 80% failure rate triggers auto-ban
	}
}

// RecordSuccess records a successful request for a channel
func (t *ChannelHealthTracker) RecordSuccess(channelID uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	window := t.getOrCreateWindow(channelID)
	window.TotalRequests++
}

// RecordFailure records a failed request for a channel
func (t *ChannelHealthTracker) RecordFailure(ctx context.Context, channelID uuid.UUID, autoBanEnabled bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	window := t.getOrCreateWindow(channelID)
	window.TotalRequests++
	window.FailedRequests++

	// Check if auto-ban should be triggered
	if autoBanEnabled && window.TotalRequests >= 10 { // Minimum 10 requests before considering auto-ban
		failureRate := (window.FailedRequests * 100) / window.TotalRequests
		if failureRate >= t.failureThreshold {
			// Auto-ban the channel
			if err := t.banChannel(ctx, channelID); err != nil {
				return fmt.Errorf("failed to auto-ban channel: %w", err)
			}
			// Reset window after banning
			delete(t.failureWindows, channelID)
		}
	}

	return nil
}

// getOrCreateWindow gets or creates a failure window for a channel
func (t *ChannelHealthTracker) getOrCreateWindow(channelID uuid.UUID) *FailureWindow {
	now := time.Now()

	window, exists := t.failureWindows[channelID]
	if !exists || now.Sub(window.WindowStartTime) > t.windowDuration {
		// Create new window
		window = &FailureWindow{
			TotalRequests:   0,
			FailedRequests:  0,
			WindowStartTime: now,
		}
		t.failureWindows[channelID] = window
	}

	return window
}

// banChannel disables a channel in the database (V2: uses llm_routes table)
func (t *ChannelHealthTracker) banChannel(ctx context.Context, channelID uuid.UUID) error {
	return t.db.WithContext(ctx).
		Model(&channelmodel.LLMRoute{}).
		Where("id = ?", channelID).
		Update("is_enabled", false).Error
}

// GetFailureRate returns the current failure rate for a channel
func (t *ChannelHealthTracker) GetFailureRate(channelID uuid.UUID) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	window, exists := t.failureWindows[channelID]
	if !exists || window.TotalRequests == 0 {
		return 0
	}

	return (window.FailedRequests * 100) / window.TotalRequests
}

// CleanupExpiredWindows removes expired failure windows
func (t *ChannelHealthTracker) CleanupExpiredWindows() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for channelID, window := range t.failureWindows {
		if now.Sub(window.WindowStartTime) > t.windowDuration {
			delete(t.failureWindows, channelID)
		}
	}
}

// StartCleanupRoutine starts a background routine to cleanup expired windows
func (t *ChannelHealthTracker) StartCleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.CleanupExpiredWindows()
			}
		}
	}()
}
