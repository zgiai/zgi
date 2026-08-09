package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const (
	workspaceRoleNameMaxRunes        = 30
	workspaceRoleDescriptionMaxRunes = 200
)

var reservedWorkspaceRoleNames = map[string]struct{}{
	"owner":  {},
	"admin":  {},
	"member": {},
	"viewer": {},
}

func normalizeWorkspaceRoleName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrWorkspaceRoleNameRequired
	}
	if utf8.RuneCountInString(name) > workspaceRoleNameMaxRunes {
		return "", ErrWorkspaceRoleNameTooLong
	}
	if _, reserved := reservedWorkspaceRoleNames[strings.ToLower(name)]; reserved {
		return "", ErrWorkspaceRoleNameReserved
	}
	return name, nil
}

func normalizeWorkspaceRoleDescription(description *string) (*string, error) {
	if description == nil {
		return nil, nil
	}

	normalized := strings.TrimSpace(*description)
	if utf8.RuneCountInString(normalized) > workspaceRoleDescriptionMaxRunes {
		return nil, ErrWorkspaceRoleDescriptionTooLong
	}
	return &normalized, nil
}

func workspaceRoleNameExists(ctx context.Context, db *gorm.DB, organizationID, name, excludeRoleID string) (bool, error) {
	query := db.WithContext(ctx).
		Model(&model.WorkspaceCustomRole{}).
		Where("group_id = ? AND name = ? AND status != ?", organizationID, name, model.WorkspaceCustomRoleStatusDeleted)
	if excludeRoleID != "" {
		query = query.Where("id != ?", excludeRoleID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check role name existence: %w", err)
	}
	return count > 0, nil
}

func isWorkspaceRoleUniqueConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && (pgErr.ConstraintName == "uk_roles_group_name" || pgErr.ConstraintName == "uk_roles_group_system_key")
	}

	message := strings.ToLower(err.Error())
	return (strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint failed")) &&
		strings.Contains(message, "roles")
}

func ensureDefaultWorkspaceRoleTemplates(ctx context.Context, db *gorm.DB, organizationID, createdBy, language string) error {
	if db == nil || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(createdBy) == "" {
		return fmt.Errorf("invalid default workspace role template owner")
	}

	definitions := model.DefaultWorkspaceRoleTemplateDefinitions()
	systemKeys := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		systemKeys = append(systemKeys, definition.SystemKey)
	}

	var existingRoles []model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("group_id = ? AND system_key IN ?", organizationID, systemKeys).
		Find(&existingRoles).Error; err != nil {
		return fmt.Errorf("failed to list default workspace role templates: %w", err)
	}

	existingKeys := make(map[string]struct{}, len(existingRoles))
	for _, role := range existingRoles {
		if role.SystemKey != nil {
			existingKeys[*role.SystemKey] = struct{}{}
		}
	}

	for _, definition := range definitions {
		if _, exists := existingKeys[definition.SystemKey]; exists {
			continue
		}

		name := defaultWorkspaceRoleTemplateName(definition, language)
		uniqueName, err := availableDefaultWorkspaceRoleTemplateName(ctx, db, organizationID, name, language)
		if err != nil {
			return err
		}

		description := defaultWorkspaceRoleTemplateDescription(definition, language)
		systemKey := definition.SystemKey
		role := &model.WorkspaceCustomRole{
			OrganizationID: organizationID,
			Name:           uniqueName,
			NameI18n: map[string]string{
				"zh_Hans": definition.NameZhHans,
				"en_US":   definition.NameEnUS,
			},
			Description: &description,
			DescriptionI18n: map[string]string{
				"zh_Hans": definition.DescZhHans,
				"en_US":   definition.DescEnUS,
			},
			Status:         model.WorkspaceCustomRoleStatusActive,
			Permissions:    model.CanonicalAssignableWorkspacePermissionSnapshotStrings(definition.Permissions),
			SystemKey:      &systemKey,
			TemplateOrigin: model.WorkspaceRoleTemplateOriginSystemDefault,
			CreatedBy:      createdBy,
		}
		if err := db.WithContext(ctx).Create(role).Error; err != nil {
			if isWorkspaceRoleUniqueConstraintViolation(err) {
				continue
			}
			return fmt.Errorf("failed to create default workspace role template %s: %w", definition.SystemKey, err)
		}
	}

	return nil
}

func defaultWorkspaceRoleTemplateName(definition model.WorkspaceDefaultRoleTemplateDefinition, language string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		return definition.NameEnUS
	}
	return definition.NameZhHans
}

func defaultWorkspaceRoleTemplateDescription(definition model.WorkspaceDefaultRoleTemplateDefinition, language string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		return definition.DescEnUS
	}
	return definition.DescZhHans
}

func availableDefaultWorkspaceRoleTemplateName(ctx context.Context, db *gorm.DB, organizationID, preferredName, language string) (string, error) {
	base := strings.TrimSpace(preferredName)
	if base == "" {
		base = "Default Member"
	}

	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt == 1 {
			candidate = base + "（默认）"
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
				candidate = base + " (Default)"
			}
		} else if attempt > 1 {
			candidate = fmt.Sprintf("%s %d", base, attempt)
		}

		exists, err := workspaceRoleNameExists(ctx, db, organizationID, candidate, "")
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique workspace role template name")
}
