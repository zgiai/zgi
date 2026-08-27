package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errDirectSkillCredentialNotFound = errors.New("digital human direct skill credential not found")

func (r *GormRepository) DirectSkillCredential(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	skillID string,
	lock bool,
) (*DirectSkillCredential, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || strings.TrimSpace(skillID) == "" {
		return nil, ErrInvalidRequest
	}
	query := r.db.WithContext(ctx).Model(&DirectSkillCredential{}).
		Where(
			"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND skill_id = ?",
			scope.OrganizationID, scope.WorkspaceID, digitalHumanID, strings.ToLower(strings.TrimSpace(skillID)),
		)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var credential DirectSkillCredential
	if err := query.Take(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errDirectSkillCredentialNotFound
		}
		return nil, fmt.Errorf("get digital human direct skill credential: %w", err)
	}
	return &credential, nil
}

func (r *GormRepository) CreateDirectSkillCredential(ctx context.Context, credential *DirectSkillCredential) error {
	if credential == nil || credential.ID == uuid.Nil || credential.DigitalHumanID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(credential).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRevisionConflict
		}
		return fmt.Errorf("create digital human direct skill credential: %w", err)
	}
	return nil
}

func (r *GormRepository) UpdateDirectSkillCredential(ctx context.Context, credential *DirectSkillCredential, expectedRevision int64) error {
	if credential == nil || credential.ID == uuid.Nil || expectedRevision < 1 {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&DirectSkillCredential{}).
		Where("id = ? AND revision = ?", credential.ID, expectedRevision).
		Updates(map[string]any{
			"encrypted_credentials": credential.EncryptedCredentials,
			"credential_version":    credential.CredentialVersion,
			"enabled":               credential.Enabled,
			"revision":              credential.Revision,
			"updated_by":            credential.UpdatedBy,
			"updated_at":            credential.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update digital human direct skill credential: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}
