package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type accountSkillPreferenceRepository struct {
	db *gorm.DB
}

func NewAccountSkillPreferenceRepository(db *gorm.DB) AccountSkillPreferenceRepository {
	return &accountSkillPreferenceRepository{db: db}
}

func (r *accountSkillPreferenceRepository) Get(ctx context.Context, organizationID, accountID uuid.UUID, callerType string) (*runtimemodel.AccountSkillPreference, error) {
	var pref runtimemodel.AccountSkillPreference
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND account_id = ? AND caller_type = ?", organizationID, accountID, callerType).
		Take(&pref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get account skill preference: %w", err)
	}
	return &pref, nil
}

func (r *accountSkillPreferenceRepository) Upsert(ctx context.Context, pref *runtimemodel.AccountSkillPreference) error {
	if pref == nil {
		return fmt.Errorf("account skill preference is required")
	}
	now := time.Now()
	pref.UpdatedAt = now
	enabledSkillIDs, err := json.Marshal(pref.EnabledSkillIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal account skill preference enabled skill ids: %w", err)
	}
	result := r.db.WithContext(ctx).
		Model(&runtimemodel.AccountSkillPreference{}).
		Where("organization_id = ? AND account_id = ? AND caller_type = ?", pref.OrganizationID, pref.AccountID, pref.CallerType).
		Updates(map[string]interface{}{
			"enabled_skill_ids": datatypes.JSON(enabledSkillIDs),
			"updated_at":        now,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update account skill preference: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}
	pref.CreatedAt = now
	if err := r.db.WithContext(ctx).Create(pref).Error; err != nil {
		return fmt.Errorf("failed to create account skill preference: %w", err)
	}
	return nil
}
