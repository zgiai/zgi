package APIKey

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIKeyUsageLogRepository defines the interface for API key usage log operations
type APIKeyUsageLogRepository interface {
	Create(ctx context.Context, log *APIKeyUsageLog) error
	GetByAPIKeyID(ctx context.Context, apiKeyID uuid.UUID, page, limit int, startDate, endDate *time.Time) ([]*APIKeyUsageLog, int64, error)
	GetByAgentID(ctx context.Context, agentID uuid.UUID, page, limit int, startDate, endDate *time.Time) ([]*APIKeyUsageLog, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*APIKeyUsageLog, error)
	GetTotalTokensUsed(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate *time.Time) (int64, error)
	GetUsageStats(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate *time.Time) (map[string]interface{}, error)
}

// apiKeyUsageLogRepository implements APIKeyUsageLogRepository
type apiKeyUsageLogRepository struct {
	db *gorm.DB
}

// NewAPIKeyUsageLogRepository creates a new API key usage log repository
func NewAPIKeyUsageLogRepository(db *gorm.DB) APIKeyUsageLogRepository {
	return &apiKeyUsageLogRepository{db: db}
}

// Create creates a new API key usage log
func (r *apiKeyUsageLogRepository) Create(ctx context.Context, log *APIKeyUsageLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("failed to create API key usage log: %w", err)
	}
	return nil
}

// GetByAPIKeyID retrieves usage logs for a specific API key with pagination and date filtering
func (r *apiKeyUsageLogRepository) GetByAPIKeyID(ctx context.Context, apiKeyID uuid.UUID, page, limit int, startDate, endDate *time.Time) ([]*APIKeyUsageLog, int64, error) {
	var logs []*APIKeyUsageLog
	var total int64

	query := r.db.WithContext(ctx).Model(&APIKeyUsageLog{}).Where("api_key_id = ?", apiKeyID)

	// Apply date filters
	if startDate != nil {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", endDate)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count API key usage logs: %w", err)
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get API key usage logs: %w", err)
	}

	return logs, total, nil
}

// GetByAgentID retrieves usage logs for a specific agent with pagination and date filtering
func (r *apiKeyUsageLogRepository) GetByAgentID(ctx context.Context, agentID uuid.UUID, page, limit int, startDate, endDate *time.Time) ([]*APIKeyUsageLog, int64, error) {
	var logs []*APIKeyUsageLog
	var total int64

	query := r.db.WithContext(ctx).Model(&APIKeyUsageLog{}).Where("agent_id = ?", agentID)

	// Apply date filters
	if startDate != nil {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", endDate)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count API key usage logs: %w", err)
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get API key usage logs: %w", err)
	}

	return logs, total, nil
}

// GetByID retrieves a specific usage log by ID
func (r *apiKeyUsageLogRepository) GetByID(ctx context.Context, id uuid.UUID) (*APIKeyUsageLog, error) {
	var log APIKeyUsageLog
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&log).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("API key usage log not found")
		}
		return nil, fmt.Errorf("failed to get API key usage log: %w", err)
	}
	return &log, nil
}

// GetTotalTokensUsed calculates the total tokens used for an API key within a date range
func (r *apiKeyUsageLogRepository) GetTotalTokensUsed(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate *time.Time) (int64, error) {
	var totalTokens int64

	query := r.db.WithContext(ctx).Model(&APIKeyUsageLog{}).
		Where("api_key_id = ?", apiKeyID).
		Select("COALESCE(SUM(tokens_used), 0)")

	// Apply date filters
	if startDate != nil {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", endDate)
	}

	if err := query.Scan(&totalTokens).Error; err != nil {
		return 0, fmt.Errorf("failed to calculate total tokens used: %w", err)
	}

	return totalTokens, nil
}

// GetUsageStats retrieves usage statistics for an API key
func (r *apiKeyUsageLogRepository) GetUsageStats(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate *time.Time) (map[string]interface{}, error) {
	var stats struct {
		TotalRequests   int64   `gorm:"column:total_requests"`
		SuccessfulReqs  int64   `gorm:"column:successful_requests"`
		FailedReqs      int64   `gorm:"column:failed_requests"`
		TotalTokens     int64   `gorm:"column:total_tokens"`
		AvgResponseTime float64 `gorm:"column:avg_response_time"`
	}

	query := r.db.WithContext(ctx).Model(&APIKeyUsageLog{}).
		Where("api_key_id = ?", apiKeyID).
		Select(`
			COUNT(*) as total_requests,
			COUNT(CASE WHEN response_status_code >= 200 AND response_status_code < 300 THEN 1 END) as successful_requests,
			COUNT(CASE WHEN response_status_code >= 400 OR error_message IS NOT NULL THEN 1 END) as failed_requests,
			COALESCE(SUM(tokens_used), 0) as total_tokens,
			COALESCE(AVG(response_time_ms), 0) as avg_response_time
		`)

	// Apply date filters
	if startDate != nil {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", endDate)
	}

	if err := query.Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}

	return map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"successful_requests":  stats.SuccessfulReqs,
		"failed_requests":      stats.FailedReqs,
		"total_tokens":         stats.TotalTokens,
		"avg_response_time_ms": stats.AvgResponseTime,
	}, nil
}
