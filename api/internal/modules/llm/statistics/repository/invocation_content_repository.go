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
	ViewedAt       time.Time `gorm:"column:viewed_at"`
}

func (invocationContentViewRow) TableName() string { return "llm_invocation_content_views" }

func (r *statisticsRepositoryImpl) GetInvocationContentSettings(ctx context.Context, organizationID string) (bool, error) {
	var enabled bool
	err := r.db.WithContext(ctx).Table("organizations").Where("id = ?", organizationID).
		Pluck("llm_content_capture_enabled", &enabled).Error
	return enabled, err
}

func (r *statisticsRepositoryImpl) UpdateInvocationContentSettings(ctx context.Context, organizationID string, enabled bool) error {
	result := r.db.WithContext(ctx).Table("organizations").Where("id = ?", organizationID).
		Update("llm_content_capture_enabled", enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
			AccountID: accountID, ViewedAt: time.Now().UTC(),
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
