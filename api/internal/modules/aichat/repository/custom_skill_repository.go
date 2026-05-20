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

type customSkillRepository struct {
	db *gorm.DB
}

func NewCustomSkillRepository(db *gorm.DB) CustomSkillRepository {
	return &customSkillRepository{db: db}
}

func (r *customSkillRepository) ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*aichatmodel.CustomSkill, error) {
	var skills []*aichatmodel.CustomSkill
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND status = ?", organizationID, aichatmodel.CustomSkillStatusActive).
		Order("skill_id ASC").
		Find(&skills).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list custom skills: %w", err)
	}
	return skills, nil
}

func (r *customSkillRepository) GetBySkillID(ctx context.Context, organizationID uuid.UUID, skillID string) (*aichatmodel.CustomSkill, error) {
	var skill aichatmodel.CustomSkill
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND skill_id = ? AND status = ?", organizationID, normalizeRepositorySkillID(skillID), aichatmodel.CustomSkillStatusActive).
		First(&skill).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get custom skill: %w", err)
	}
	return &skill, nil
}

func (r *customSkillRepository) Upsert(ctx context.Context, skill *aichatmodel.CustomSkill) error {
	if skill == nil {
		return fmt.Errorf("custom skill is required")
	}
	now := time.Now()
	skill.SkillID = normalizeRepositorySkillID(skill.SkillID)
	if skill.ID == uuid.Nil {
		skill.ID = uuid.New()
	}
	if skill.Status == "" {
		skill.Status = aichatmodel.CustomSkillStatusActive
	}
	if skill.Display == nil {
		skill.Display = map[string]interface{}{}
	}
	if skill.Manifest == nil {
		skill.Manifest = map[string]interface{}{}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing aichatmodel.CustomSkill
		err := tx.Where("organization_id = ? AND skill_id = ? AND deleted_at IS NULL", skill.OrganizationID, skill.SkillID).
			First(&existing).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to find custom skill for upsert: %w", err)
		}
		if err == gorm.ErrRecordNotFound {
			skill.CreatedAt = now
			skill.UpdatedAt = now
			if err := tx.Create(skill).Error; err != nil {
				return fmt.Errorf("failed to create custom skill: %w", err)
			}
			return nil
		}
		updates := map[string]interface{}{
			"name":             skill.Name,
			"description":      skill.Description,
			"when_to_use":      skill.WhenToUse,
			"runtime_type":     skill.RuntimeType,
			"display":          skill.Display,
			"storage_path":     skill.StoragePath,
			"manifest":         skill.Manifest,
			"status":           skill.Status,
			"validation_error": skill.ValidationError,
			"updated_at":       now,
		}
		if err := tx.Model(&aichatmodel.CustomSkill{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update custom skill: %w", err)
		}
		skill.ID = existing.ID
		skill.CreatedAt = existing.CreatedAt
		skill.UpdatedAt = now
		return nil
	})
}

func (r *customSkillRepository) DeleteBySkillID(ctx context.Context, organizationID uuid.UUID, skillID string) error {
	result := r.db.WithContext(ctx).
		Where("organization_id = ? AND skill_id = ?", organizationID, normalizeRepositorySkillID(skillID)).
		Delete(&aichatmodel.CustomSkill{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete custom skill: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func normalizeRepositorySkillID(skillID string) string {
	return strings.ToLower(strings.TrimSpace(skillID))
}
