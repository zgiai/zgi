package dto

import (
	"time"

	"github.com/google/uuid"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
)

// QuotaUsageHistoryDTO Quota usage history DTO
type QuotaUsageHistoryDTO struct {
	ID                string                    `json:"id"`
	GroupID           uuid.UUID                 `json:"group_id"`
	AccountID         uuid.UUID                 `json:"account_id"`
	TenantID          *uuid.UUID                `json:"tenant_id,omitempty"`
	ResourceType      quota_model.ResourceType  `json:"resource_type"`
	ResourceTypeName  string                    `json:"resource_type_name"`
	OperationType     quota_model.OperationType `json:"operation_type"`
	OperationTypeName string                    `json:"operation_type_name"`
	Delta             int64                     `json:"delta"`
	ValueBefore       int64                     `json:"value_before"`
	ValueAfter        int64                     `json:"value_after"`
	ResourceID        *string                   `json:"resource_id,omitempty"`
	ResourceName      *string                   `json:"resource_name,omitempty"`
	Metadata          map[string]interface{}    `json:"metadata,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
}

// QuotaStatusDTO Quota status DTO
type QuotaStatusDTO struct {
	GroupID   uuid.UUID                    `json:"group_id"`
	PlanCode  string                       `json:"plan_code"`
	Resources map[string]*ResourceQuotaDTO `json:"resources"`
	UpdatedAt time.Time                    `json:"updated_at"`
}

// ResourceQuotaDTO Resource quota DTO
type ResourceQuotaDTO struct {
	Used      int64   `json:"used"`      // Used amount
	Limit     int64   `json:"limit"`     // Limit (-1 means unlimited)
	Unlimited bool    `json:"unlimited"` // Whether unlimited
	Usage     float64 `json:"usage"`     // Usage rate (0-100)
	Unit      string  `json:"unit"`      // Unit (e.g.: count, GB, rows)
}

// QuotaUsageHistoryFilterDTO Quota usage history query filter DTO
type QuotaUsageHistoryFilterDTO struct {
	GroupID       *uuid.UUID                 `json:"group_id,omitempty" form:"group_id"`
	AccountID     *uuid.UUID                 `json:"account_id,omitempty" form:"account_id"`
	TenantID      *uuid.UUID                 `json:"tenant_id,omitempty" form:"tenant_id"`
	ResourceType  *quota_model.ResourceType  `json:"resource_type,omitempty" form:"resource_type"`
	OperationType *quota_model.OperationType `json:"operation_type,omitempty" form:"operation_type"`
	ResourceID    *string                    `json:"resource_id,omitempty" form:"resource_id"`
	StartTime     *time.Time                 `json:"start_time,omitempty" form:"start_time"`
	EndTime       *time.Time                 `json:"end_time,omitempty" form:"end_time"`
	Page          int                        `json:"page" form:"page" binding:"min=1"`
	Limit         int                        `json:"limit" form:"limit" binding:"min=1,max=100"`
	Sort          string                     `json:"sort" form:"sort"` // created_at_desc, created_at_asc
}

// QuotaUsageHistoryListResponseDTO Quota usage history list response DTO
type QuotaUsageHistoryListResponseDTO struct {
	Items []QuotaUsageHistoryDTO `json:"items"`
	Total int64                  `json:"total"`
	Page  int                    `json:"page"`
	Limit int                    `json:"limit"`
}

// QuotaCheckRequestDTO Quota check request DTO
type QuotaCheckRequestDTO struct {
	GroupID      uuid.UUID                `json:"group_id" binding:"required"`
	ResourceType quota_model.ResourceType `json:"resource_type" binding:"required"`
	Amount       int64                    `json:"amount" binding:"required,min=1"`
}

// QuotaCheckResponseDTO Quota check response DTO
type QuotaCheckResponseDTO struct {
	CanProceed   bool  `json:"can_proceed"`
	CurrentUsage int64 `json:"current_usage"`
	Limit        int64 `json:"limit"`
	Remaining    int64 `json:"remaining"`
	Unlimited    bool  `json:"unlimited"`
}

// QuotaUpdateRequestDTO Quota update request DTO
type QuotaUpdateRequestDTO struct {
	GroupID      uuid.UUID                `json:"group_id" binding:"required"`
	AccountID    uuid.UUID                `json:"account_id" binding:"required"`
	TenantID     *uuid.UUID               `json:"tenant_id,omitempty"`
	ResourceType quota_model.ResourceType `json:"resource_type" binding:"required"`
	Delta        int64                    `json:"delta" binding:"required"`
	ResourceID   *string                  `json:"resource_id,omitempty"`
	ResourceName *string                  `json:"resource_name,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
}

// ToQuotaUsageHistoryDTO Convert model to DTO
func ToQuotaUsageHistoryDTO(history *quota_model.QuotaUsageHistory) *QuotaUsageHistoryDTO {
	dto := &QuotaUsageHistoryDTO{
		ID:                history.ID,
		GroupID:           history.GroupID,
		AccountID:         history.AccountID,
		TenantID:          history.TenantID,
		ResourceType:      history.ResourceType,
		ResourceTypeName:  history.ResourceType.GetDisplayName(),
		OperationType:     history.OperationType,
		OperationTypeName: history.OperationType.GetDisplayName(),
		Delta:             history.Delta,
		ValueBefore:       history.ValueBefore,
		ValueAfter:        history.ValueAfter,
		ResourceID:        history.ResourceID,
		ResourceName:      history.ResourceName,
		CreatedAt:         history.CreatedAt,
	}

	// Parse metadata
	if history.Metadata != nil {
		dto.Metadata = map[string]interface{}(*history.Metadata)
	}

	return dto
}
