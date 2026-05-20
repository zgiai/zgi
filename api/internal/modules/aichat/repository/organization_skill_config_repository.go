package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"gorm.io/gorm"
)

type organizationSkillConfigRepository struct {
	db *gorm.DB
}

func NewOrganizationSkillConfigRepository(db *gorm.DB) OrganizationSkillConfigRepository {
	return &organizationSkillConfigRepository{db: db}
}

func (r *organizationSkillConfigRepository) ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*aichatmodel.OrganizationSkillConfig, error) {
	var configs []*aichatmodel.OrganizationSkillConfig
	if err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID.String()).
		Order("skill_id ASC").
		Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list aichat organization skill configs: %w", err)
	}
	return configs, nil
}

func (r *organizationSkillConfigRepository) ReplaceForOrganization(ctx context.Context, organizationID uuid.UUID, configs []*aichatmodel.OrganizationSkillConfig) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ?", organizationID.String()).Delete(&aichatmodel.OrganizationSkillConfig{}).Error; err != nil {
			return fmt.Errorf("failed to delete aichat organization skill configs: %w", err)
		}
		if len(configs) == 0 {
			return nil
		}
		for _, config := range configs {
			if config == nil {
				continue
			}
			if err := tx.Exec(
				`INSERT INTO aichat_organization_skill_configs (organization_id, skill_id, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
				organizationID.String(),
				config.SkillID,
				config.Enabled,
				now,
				now,
			).Error; err != nil {
				return fmt.Errorf("failed to create aichat organization skill config: %w", err)
			}
		}
		return nil
	})
}

func (r *organizationSkillConfigRepository) DeleteByOrganizationAndSkill(ctx context.Context, organizationID uuid.UUID, skillID string) error {
	id := strings.ToLower(strings.TrimSpace(skillID))
	if id == "" {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND skill_id = ?", organizationID.String(), id).
		Delete(&aichatmodel.OrganizationSkillConfig{}).Error; err != nil {
		return fmt.Errorf("failed to delete aichat organization skill config: %w", err)
	}
	return nil
}
