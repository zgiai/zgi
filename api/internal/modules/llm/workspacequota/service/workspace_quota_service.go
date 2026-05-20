package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/dto"
	"gorm.io/gorm"
)

var (
	ErrWorkspaceNotFound    = errors.New("workspace not found")
	ErrWorkspaceOrgMismatch = errors.New("workspace does not belong to organization")
	ErrInvalidOrganization  = errors.New("invalid organization_id")
	ErrInvalidWorkspaceID   = errors.New("workspace_id is required")
	ErrQuotaAmountRequired  = errors.New("quota_amount is required and must be greater than 0 when quota_type=custom")
	ErrInvalidRemainQuota   = errors.New("remain_quota must be non-negative")
	ErrRemainExceedsLimit   = errors.New("remain_quota cannot exceed quota_limit")
)

type workspaceQuotaServiceImpl struct {
	db *gorm.DB
}

func NewWorkspaceQuotaService(db *gorm.DB) WorkspaceQuotaService {
	return &workspaceQuotaServiceImpl{db: db}
}

func (s *workspaceQuotaServiceImpl) ListWorkspaceQuotas(
	ctx context.Context,
	organizationID string,
	req *dto.ListWorkspaceQuotaRequest,
) (*dto.ListWorkspaceQuotaResponse, error) {
	orgUUID, err := parseOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}

	page := 1
	limit := 20
	if req != nil {
		if req.Page > 0 {
			page = req.Page
		}
		if req.Limit > 0 {
			limit = req.Limit
		}
	}

	var total int64
	query := s.db.WithContext(ctx).Model(&gateway.WorkspaceQuota{}).Where("organization_id = ?", orgUUID)
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count workspace quotas: %w", err)
	}

	var rows []*gateway.WorkspaceQuota
	if err := query.
		Order("updated_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list workspace quotas: %w", err)
	}

	items := make([]*dto.WorkspaceQuotaResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toWorkspaceQuotaResponse(row, true))
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if total == 0 {
		totalPages = 0
	}

	return &dto.ListWorkspaceQuotaResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func (s *workspaceQuotaServiceImpl) GetWorkspaceQuota(
	ctx context.Context,
	organizationID, workspaceID string,
) (*dto.WorkspaceQuotaResponse, error) {
	orgUUID, err := parseOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrInvalidWorkspaceID
	}
	if err := s.assertWorkspaceBelongsOrganization(ctx, workspaceID, organizationID); err != nil {
		return nil, err
	}

	var quota gateway.WorkspaceQuota
	err = s.db.WithContext(ctx).
		Where("workspace_id = ? AND organization_id = ?", workspaceID, orgUUID).
		First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return default unlimited view for an existing workspace without explicit quota config.
			return &dto.WorkspaceQuotaResponse{
				WorkspaceID:    workspaceID,
				OrganizationID: organizationID,
				UsedQuota:      0,
				RemainQuota:    0,
				QuotaLimit:     nil,
				Configured:     false,
			}, nil
		}
		return nil, fmt.Errorf("get workspace quota: %w", err)
	}

	return toWorkspaceQuotaResponse(&quota, true), nil
}

func (s *workspaceQuotaServiceImpl) UpdateWorkspaceQuota(
	ctx context.Context,
	organizationID, workspaceID string,
	req *dto.UpdateWorkspaceQuotaRequest,
) (*dto.WorkspaceQuotaResponse, error) {
	orgUUID, err := parseOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrInvalidWorkspaceID
	}
	if req == nil {
		return nil, fmt.Errorf("update request is required")
	}
	if err := validateUpdateRequest(req); err != nil {
		return nil, err
	}
	if err := s.assertWorkspaceBelongsOrganization(ctx, workspaceID, organizationID); err != nil {
		return nil, err
	}

	var quota gateway.WorkspaceQuota
	now := time.Now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.WithContext(ctx).
			Where("workspace_id = ? AND organization_id = ?", workspaceID, orgUUID).
			First(&quota).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("query workspace quota: %w", err)
			}

			quota = gateway.WorkspaceQuota{
				WorkspaceID:    workspaceID,
				OrganizationID: orgUUID,
				UsedQuota:      0,
				RemainQuota:    0,
				QuotaLimit:     nil,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
		} else {
			quota.UpdatedAt = now
		}

		switch req.QuotaType {
		case dto.QuotaTypeUnlimited:
			quota.QuotaLimit = nil
		case dto.QuotaTypeCustom:
			limit := *req.QuotaAmount
			quota.QuotaLimit = &limit
		}

		if req.RemainQuota != nil {
			quota.RemainQuota = *req.RemainQuota
		} else if req.QuotaType == dto.QuotaTypeCustom && quota.QuotaLimit != nil && quota.RemainQuota > *quota.QuotaLimit {
			quota.RemainQuota = *quota.QuotaLimit
		}

		if quota.QuotaLimit != nil && quota.RemainQuota > *quota.QuotaLimit {
			return ErrRemainExceedsLimit
		}

		if err := tx.WithContext(ctx).Save(&quota).Error; err != nil {
			return fmt.Errorf("save workspace quota: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toWorkspaceQuotaResponse(&quota, true), nil
}

func (s *workspaceQuotaServiceImpl) assertWorkspaceBelongsOrganization(
	ctx context.Context,
	workspaceID string,
	organizationID string,
) error {
	workspaceID = strings.TrimSpace(workspaceID)
	organizationID = strings.TrimSpace(organizationID)

	var row struct {
		OrganizationID *string `gorm:"column:organization_id"`
	}
	err := s.db.WithContext(ctx).
		Table("workspaces").
		Select("organization_id").
		Where("id = ?", workspaceID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("query workspace: %w", err)
	}
	if row.OrganizationID == nil || *row.OrganizationID == "" || *row.OrganizationID != organizationID {
		return ErrWorkspaceOrgMismatch
	}
	return nil
}

func validateUpdateRequest(req *dto.UpdateWorkspaceQuotaRequest) error {
	if req.QuotaType == dto.QuotaTypeCustom {
		if req.QuotaAmount == nil || *req.QuotaAmount <= 0 {
			return ErrQuotaAmountRequired
		}
	}
	if req.RemainQuota != nil && *req.RemainQuota < 0 {
		return ErrInvalidRemainQuota
	}
	return nil
}

func parseOrganizationID(organizationID string) (uuid.UUID, error) {
	orgID := strings.TrimSpace(organizationID)
	if orgID == "" {
		return uuid.Nil, ErrInvalidOrganization
	}
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s", ErrInvalidOrganization, orgID)
	}
	return orgUUID, nil
}

func toWorkspaceQuotaResponse(row *gateway.WorkspaceQuota, configured bool) *dto.WorkspaceQuotaResponse {
	if row == nil {
		return nil
	}
	createdAt := row.CreatedAt
	updatedAt := row.UpdatedAt
	return &dto.WorkspaceQuotaResponse{
		WorkspaceID:    row.WorkspaceID,
		OrganizationID: row.OrganizationID.String(),
		UsedQuota:      row.UsedQuota,
		RemainQuota:    row.RemainQuota,
		QuotaLimit:     row.QuotaLimit,
		Configured:     configured,
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
	}
}
