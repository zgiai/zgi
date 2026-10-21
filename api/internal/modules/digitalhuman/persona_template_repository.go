package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *GormRepository) CreatePersonaTemplate(ctx context.Context, template *PersonaTemplate) error {
	if template == nil || template.ID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(template).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrPersonaTemplateNameConflict
		}
		return fmt.Errorf("create DigitalHuman persona template: %w", err)
	}
	return nil
}

func (r *GormRepository) PersonaTemplate(ctx context.Context, scope Scope, id uuid.UUID) (*PersonaTemplate, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var template PersonaTemplate
	if err := r.scopedPersonaTemplates(ctx, scope).Where("id = ?", id).Take(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPersonaTemplateNotFound
		}
		return nil, fmt.Errorf("get DigitalHuman persona template: %w", err)
	}
	return &template, nil
}

func (r *GormRepository) ListPersonaTemplates(ctx context.Context, scope Scope, query ListQuery) ([]*PersonaTemplate, int64, error) {
	if !validScope(scope) || query.Page < 1 || query.PageSize < 1 {
		return nil, 0, ErrInvalidRequest
	}
	db := r.scopedPersonaTemplates(ctx, scope)
	if query.Search != "" {
		db = db.Where(`LOWER(name) LIKE LOWER(?) ESCAPE '\'`, "%"+escapeSearch(query.Search)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count DigitalHuman persona templates: %w", err)
	}
	items := make([]*PersonaTemplate, 0, query.PageSize)
	if err := db.Order("updated_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list DigitalHuman persona templates: %w", err)
	}
	return items, total, nil
}

func (r *GormRepository) PersonaTemplateNameExists(ctx context.Context, scope Scope, name string, excludeID uuid.UUID) (bool, error) {
	query := r.scopedPersonaTemplates(ctx, scope).Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name))
	if excludeID != uuid.Nil {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check DigitalHuman persona template name: %w", err)
	}
	return count > 0, nil
}

func (r *GormRepository) UpdatePersonaTemplateCAS(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64, updates map[string]any) error {
	updates["revision"] = gorm.Expr("revision + 1")
	updates["updated_by"] = scope.ActorID
	updates["updated_at"] = time.Now().UTC()
	result := r.scopedPersonaTemplates(ctx, scope).Where("id = ? AND revision = ?", id, expectedRevision).Updates(updates)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return ErrPersonaTemplateNameConflict
		}
		return fmt.Errorf("update DigitalHuman persona template: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) DeletePersonaTemplateCAS(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64) error {
	result := r.scopedPersonaTemplates(ctx, scope).
		Where("id = ? AND revision = ?", id, expectedRevision).
		Updates(map[string]any{"deleted_at": time.Now().UTC(), "updated_by": scope.ActorID, "revision": gorm.Expr("revision + 1")})
	if result.Error != nil {
		return fmt.Errorf("delete DigitalHuman persona template: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) scopedPersonaTemplates(ctx context.Context, scope Scope) *gorm.DB {
	return r.db.WithContext(ctx).Model(&PersonaTemplate{}).
		Where("organization_id = ? AND workspace_id = ? AND deleted_at IS NULL", scope.OrganizationID, scope.WorkspaceID)
}
