package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"gorm.io/gorm"
)

type invocationContentDetailRow struct {
	RequestID        string    `gorm:"column:request_id"`
	InputText        string    `gorm:"column:input_text"`
	OutputText       string    `gorm:"column:output_text"`
	InputJSON        string    `gorm:"column:input_json"`
	OutputJSON       string    `gorm:"column:output_json"`
	ContentStatus    string    `gorm:"column:content_status"`
	InputTruncated   bool      `gorm:"column:input_truncated"`
	OutputTruncated  bool      `gorm:"column:output_truncated"`
	RedactionVersion string    `gorm:"column:redaction_version"`
	ExpiresAt        time.Time `gorm:"column:expires_at"`
}

type invocationContentViewRow struct {
	ID             string    `gorm:"column:id"`
	OrganizationID string    `gorm:"column:organization_id"`
	RequestID      string    `gorm:"column:request_id"`
	AccountID      string    `gorm:"column:account_id"`
	Action         string    `gorm:"column:action"`
	ViewedAt       time.Time `gorm:"column:viewed_at"`
}

func (invocationContentViewRow) TableName() string { return "llm_invocation_content_views" }

const (
	invocationContentStoredCountLimit   = 10000
	invocationContentPurgeBatchSize     = 500
	invocationContentPurgeMaxBatchCount = 20
)

func (r *statisticsRepositoryImpl) GetInvocationContentSettings(ctx context.Context, organizationID string) (*InvocationContentSettingsState, error) {
	var state InvocationContentSettingsState
	if err := r.db.WithContext(ctx).Table("organizations").Where("id = ?", organizationID).
		Select("llm_content_capture_enabled AS enabled", "llm_content_retention_days AS retention_days").
		Take(&state).Error; err != nil {
		return nil, err
	}
	var boundedCount int64
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT 1 FROM llm_invocation_contents
			WHERE organization_id = ? LIMIT ?
		) AS bounded_contents`, organizationID, invocationContentStoredCountLimit+1).Scan(&boundedCount).Error; err != nil {
		return nil, err
	}
	state.StoredCountCapped = boundedCount > invocationContentStoredCountLimit
	state.StoredCount = min(boundedCount, int64(invocationContentStoredCountLimit))
	return &state, nil
}

func (r *statisticsRepositoryImpl) UpdateInvocationContentSettings(ctx context.Context, organizationID string, enabled bool, retentionDays int) error {
	result := r.db.WithContext(ctx).Table("organizations").Where("id = ?", organizationID).
		Updates(map[string]any{
			"llm_content_capture_enabled": enabled,
			"llm_content_retention_days":  retentionDays,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// PurgeInvocationContent removes only optional input/output snapshots. Billing
// and invocation metadata live in llm_usage_bills and are intentionally not
// touched. An audit is persisted before bounded deletes begin, so a large
// tenant does not hold one database lock for the whole purge.
func (r *statisticsRepositoryImpl) PurgeInvocationContent(ctx context.Context, organizationID, accountID string) (int64, bool, error) {
	audit := invocationContentViewRow{
		ID: uuid.NewString(), OrganizationID: organizationID, RequestID: "*",
		AccountID: accountID, Action: "purge_all", ViewedAt: time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&audit).Error; err != nil {
		return 0, false, err
	}
	var deleted int64
	for range invocationContentPurgeMaxBatchCount {
		var requestIDs []string
		if err := r.db.WithContext(ctx).Table("llm_invocation_contents").
			Where("organization_id = ?", organizationID).Order("expires_at ASC").Limit(invocationContentPurgeBatchSize).
			Pluck("request_id", &requestIDs).Error; err != nil {
			return deleted, false, err
		}
		if len(requestIDs) == 0 {
			return deleted, false, nil
		}
		result := r.db.WithContext(ctx).Table("llm_invocation_contents").Where("request_id IN ?", requestIDs).
			Delete(&invocationContentDetailRow{})
		if result.Error != nil {
			return deleted, false, result.Error
		}
		deleted += result.RowsAffected
		if len(requestIDs) < invocationContentPurgeBatchSize {
			return deleted, false, nil
		}
	}
	var hasMore bool
	if err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM llm_invocation_contents
			WHERE organization_id = ? LIMIT 1
		)`, organizationID).Scan(&hasMore).Error; err != nil {
		return deleted, false, err
	}
	return deleted, hasMore, nil
}

// GetInvocationContent writes the access audit in the same transaction as the
// sensitive read. If the audit cannot be persisted, content is not returned.
func (r *statisticsRepositoryImpl) GetInvocationContent(ctx context.Context, organizationID, accountID, invocationID string) (*dto.InvocationContentDetail, error) {
	var row invocationContentDetailRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("llm_invocation_contents").
			Where("organization_id = ? AND request_id = ? AND expires_at > ?", organizationID, invocationID, time.Now().UTC()).
			First(&row).Error; err != nil {
			return err
		}
		audit := invocationContentViewRow{
			ID: uuid.NewString(), OrganizationID: organizationID, RequestID: invocationID,
			AccountID: accountID, Action: "view", ViewedAt: time.Now().UTC(),
		}
		return tx.Create(&audit).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &dto.InvocationContentDetail{
		InvocationID: row.RequestID, InputText: row.InputText, OutputText: row.OutputText,
		InputJSON: row.InputJSON, OutputJSON: row.OutputJSON, ContentStatus: row.ContentStatus,
		InputTruncated: row.InputTruncated, OutputTruncated: row.OutputTruncated,
		RedactionVersion: row.RedactionVersion, ExpiresAt: row.ExpiresAt.UnixMilli(),
	}, nil
}
