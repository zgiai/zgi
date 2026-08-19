package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) CreateRoleTemplate(ctx context.Context, template *RoleTemplate) error {
	if template == nil || template.ID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(template).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRoleNameConflict
		}
		return fmt.Errorf("create DigitalHuman role template: %w", err)
	}
	return nil
}

func (r *GormRepository) CreateRoleTemplateVersion(ctx context.Context, version *RoleTemplateVersion) error {
	if version == nil || version.ID == uuid.Nil || version.RoleTemplateID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(version).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRevisionConflict
		}
		return fmt.Errorf("create DigitalHuman role template version: %w", err)
	}
	return nil
}

func (r *GormRepository) AttachInitialRoleDraft(ctx context.Context, scope Scope, templateID, draftID uuid.UUID) error {
	result := r.scopedRoleTemplates(ctx, scope).
		Where("id = ? AND draft_version_id IS NULL", templateID).
		Update("draft_version_id", draftID)
	if result.Error != nil {
		return fmt.Errorf("attach DigitalHuman role template draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) RoleTemplate(ctx context.Context, scope Scope, id uuid.UUID, lock bool) (*RoleTemplate, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	query := r.scopedRoleTemplates(ctx, scope).Where("id = ?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var template RoleTemplate
	if err := query.Take(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleTemplateNotFound
		}
		return nil, fmt.Errorf("get DigitalHuman role template: %w", err)
	}
	return &template, nil
}

func (r *GormRepository) RoleTemplateDraft(ctx context.Context, scope Scope, template *RoleTemplate, lock bool) (*RoleTemplateVersion, error) {
	if template == nil || template.DraftVersionID == nil || *template.DraftVersionID == uuid.Nil {
		return nil, ErrRoleVersionNotFound
	}
	query := r.scopedRoleTemplateVersions(ctx, scope, template.ID).
		Where("id = ? AND state = ?", *template.DraftVersionID, VersionStateDraft)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var version RoleTemplateVersion
	if err := query.Take(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleVersionNotFound
		}
		return nil, fmt.Errorf("get DigitalHuman role template draft: %w", err)
	}
	return &version, nil
}

func (r *GormRepository) PublishedRoleTemplateVersion(ctx context.Context, scope Scope, versionID uuid.UUID) (*RoleTemplateVersion, error) {
	if !validScopeBoundary(scope) || versionID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var version RoleTemplateVersion
	err := r.db.WithContext(ctx).Model(&RoleTemplateVersion{}).
		Joins("JOIN digital_human_role_templates template ON template.id = digital_human_role_template_versions.role_template_id AND template.deleted_at IS NULL").
		Where("digital_human_role_template_versions.id = ? AND digital_human_role_template_versions.organization_id = ? AND digital_human_role_template_versions.workspace_id = ? AND digital_human_role_template_versions.state = ?",
			versionID, scope.OrganizationID, scope.WorkspaceID, VersionStatePublished).
		Take(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleVersionNotFound
		}
		return nil, fmt.Errorf("get published DigitalHuman role template version: %w", err)
	}
	return &version, nil
}

func (r *GormRepository) RoleTemplateDetail(ctx context.Context, scope Scope, id uuid.UUID) (*RoleTemplateDetail, error) {
	template, err := r.RoleTemplate(ctx, scope, id, false)
	if err != nil {
		return nil, err
	}
	draft, err := r.RoleTemplateDraft(ctx, scope, template, false)
	if err != nil {
		return nil, err
	}
	var published *RoleTemplateVersion
	if template.PublishedVersionID != nil {
		version, versionErr := r.PublishedRoleTemplateVersion(ctx, scope, *template.PublishedVersionID)
		if versionErr != nil {
			return nil, versionErr
		}
		published = version
	}
	return &RoleTemplateDetail{Template: template, Draft: draft, Published: published}, nil
}

// RuntimePublishedVersion resolves the exact immutable identity used by an invocation.
func (r *GormRepository) RuntimePublishedVersion(ctx context.Context, subject runtimeidentity.SubjectRef, versionID *uuid.UUID) (*Definition, *Version, error) {
	if subject.Type != runtimeidentity.SubjectTypeDigitalHuman || subject.ID == uuid.Nil || subject.OrganizationID == uuid.Nil || subject.WorkspaceID == uuid.Nil {
		return nil, nil, ErrInvalidRequest
	}
	var definition Definition
	if err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND workspace_id = ? AND deleted_at IS NULL", subject.ID, subject.OrganizationID, subject.WorkspaceID).
		Take(&definition).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("resolve DigitalHuman runtime definition: %w", err)
	}
	selectedID := definition.PublishedVersionID
	if versionID != nil {
		selectedID = versionID
	}
	if selectedID == nil || *selectedID == uuid.Nil {
		return nil, nil, ErrVersionNotFound
	}
	var version Version
	if err := r.db.WithContext(ctx).Where(
		"id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND state = ?",
		*selectedID, subject.ID, subject.OrganizationID, subject.WorkspaceID, VersionStatePublished,
	).Take(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrVersionNotFound
		}
		return nil, nil, fmt.Errorf("resolve published DigitalHuman runtime version: %w", err)
	}
	return &definition, &version, nil
}

func (r *GormRepository) ListRoleTemplates(ctx context.Context, scope Scope, query ListQuery) ([]*RoleTemplate, int64, error) {
	if !validScope(scope) || query.Page < 1 || query.PageSize < 1 {
		return nil, 0, ErrInvalidRequest
	}
	db := r.scopedRoleTemplates(ctx, scope)
	if query.Search != "" {
		db = db.Where(`LOWER(name) LIKE LOWER(?) ESCAPE '\'`, "%"+escapeSearch(query.Search)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count DigitalHuman role templates: %w", err)
	}
	items := make([]*RoleTemplate, 0, query.PageSize)
	if err := db.Order("updated_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list DigitalHuman role templates: %w", err)
	}
	return items, total, nil
}

func (r *GormRepository) RoleTemplateNameExists(ctx context.Context, scope Scope, name string, excludeID uuid.UUID) (bool, error) {
	query := r.scopedRoleTemplates(ctx, scope).Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name))
	if excludeID != uuid.Nil {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check DigitalHuman role template name: %w", err)
	}
	return count > 0, nil
}

func (r *GormRepository) UpdateRoleTemplateCAS(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64, updates map[string]any) error {
	updates["revision"] = gorm.Expr("revision + 1")
	updates["updated_by"] = scope.ActorID
	updates["updated_at"] = time.Now().UTC()
	result := r.scopedRoleTemplates(ctx, scope).Where("id = ? AND revision = ?", id, expectedRevision).Updates(updates)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return ErrRoleNameConflict
		}
		return fmt.Errorf("update DigitalHuman role template: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) UpdateRoleTemplateDraftCAS(ctx context.Context, scope Scope, version *RoleTemplateVersion, expectedLockVersion int64) error {
	result := r.db.WithContext(ctx).Model(&RoleTemplateVersion{}).
		Where("id = ? AND role_template_id = ? AND organization_id = ? AND workspace_id = ? AND state = ? AND lock_version = ?",
			version.ID, version.RoleTemplateID, scope.OrganizationID, scope.WorkspaceID, VersionStateDraft, expectedLockVersion).
		Updates(map[string]any{
			"goals": version.Goals, "responsibilities": version.Responsibilities,
			"capabilities": version.Capabilities, "sop_refs": version.SOPRefs,
			"knowledge_refs": version.KnowledgeRefs, "policy": version.Policy,
			"evaluation_refs": version.EvaluationRefs, "revision": version.Revision,
			"lock_version": gorm.Expr("lock_version + 1"), "updated_by": scope.ActorID, "updated_at": version.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update DigitalHuman role template draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	version.LockVersion = expectedLockVersion + 1
	return nil
}

func (r *GormRepository) NextPublishedRoleTemplateVersion(ctx context.Context, templateID uuid.UUID) (int, error) {
	var current int
	err := r.db.WithContext(ctx).Model(&RoleTemplateVersion{}).
		Where("role_template_id = ? AND state = ?", templateID, VersionStatePublished).
		Select("COALESCE(MAX(version_number), 0)").Scan(&current).Error
	if err != nil {
		return 0, fmt.Errorf("get next DigitalHuman role template version: %w", err)
	}
	return current + 1, nil
}

func (r *GormRepository) PromoteRoleTemplateDraftCAS(ctx context.Context, scope Scope, draft *RoleTemplateVersion, expectedLockVersion int64) error {
	result := r.db.WithContext(ctx).Model(&RoleTemplateVersion{}).
		Where("id = ? AND role_template_id = ? AND organization_id = ? AND workspace_id = ? AND state = ? AND lock_version = ?",
			draft.ID, draft.RoleTemplateID, scope.OrganizationID, scope.WorkspaceID, VersionStateDraft, expectedLockVersion).
		Updates(map[string]any{
			"state": VersionStatePublished, "version_number": draft.VersionNumber, "revision": draft.Revision,
			"published_by": draft.PublishedBy, "published_at": draft.PublishedAt,
			"updated_by": scope.ActorID, "updated_at": draft.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("publish DigitalHuman role template draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) AdvanceRoleTemplatePublicationCAS(ctx context.Context, scope Scope, template *RoleTemplate, expectedRevision int64) error {
	result := r.scopedRoleTemplates(ctx, scope).Where("id = ? AND revision = ?", template.ID, expectedRevision).
		Updates(map[string]any{
			"draft_version_id": template.DraftVersionID, "published_version_id": template.PublishedVersionID,
			"revision": gorm.Expr("revision + 1"), "updated_by": scope.ActorID, "updated_at": template.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("advance DigitalHuman role template publication: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	template.Revision = expectedRevision + 1
	return nil
}

func (r *GormRepository) ListRoleAssignments(ctx context.Context, scope Scope, digitalHumanID, versionID uuid.UUID) ([]*RoleAssignment, error) {
	items := make([]*RoleAssignment, 0)
	err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, versionID,
	).Order("role_template_id ASC, role_template_version_id ASC").Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list DigitalHuman role assignments: %w", err)
	}
	return items, nil
}

func (r *GormRepository) ReplaceDraftRoleAssignments(ctx context.Context, scope Scope, digitalHumanID, draftID uuid.UUID, items []*RoleAssignment) error {
	query := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, draftID,
	)
	if err := query.Delete(&RoleAssignment{}).Error; err != nil {
		return fmt.Errorf("replace DigitalHuman role assignments: %w", err)
	}
	if len(items) > 0 {
		if err := r.db.WithContext(ctx).Create(&items).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrInvalidRequest
			}
			return fmt.Errorf("create DigitalHuman role assignments: %w", err)
		}
	}
	return nil
}

func (r *GormRepository) ListResourceBindingRows(ctx context.Context, scope Scope, digitalHumanID, versionID uuid.UUID) ([]*ResourceBinding, error) {
	items := make([]*ResourceBinding, 0)
	err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, versionID,
	).Order("binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC").Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list DigitalHuman resource bindings: %w", err)
	}
	return items, nil
}

func (r *GormRepository) ReplaceDraftResourceBindings(ctx context.Context, scope Scope, digitalHumanID, draftID uuid.UUID, items []*ResourceBinding) error {
	query := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, draftID,
	)
	if err := query.Delete(&ResourceBinding{}).Error; err != nil {
		return fmt.Errorf("replace DigitalHuman resource bindings: %w", err)
	}
	if len(items) > 0 {
		if err := r.db.WithContext(ctx).Create(&items).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrResourceBindingInvalid
			}
			return fmt.Errorf("create DigitalHuman resource bindings: %w", err)
		}
	}
	return nil
}

// CloneVersionResources creates mutable copies for the next draft while preserving published rows.
func (r *GormRepository) CloneVersionResources(ctx context.Context, scope Scope, digitalHumanID, fromVersionID, toVersionID uuid.UUID, actorID uuid.UUID, now time.Time) error {
	assignments, err := r.ListRoleAssignments(ctx, scope, digitalHumanID, fromVersionID)
	if err != nil {
		return err
	}
	for _, item := range assignments {
		item.ID = uuid.New()
		item.DigitalHumanVersionID = toVersionID
		item.CreatedBy, item.UpdatedBy = &actorID, &actorID
		item.CreatedAt, item.UpdatedAt = now, now
	}
	if len(assignments) > 0 {
		if err := r.db.WithContext(ctx).Create(&assignments).Error; err != nil {
			return fmt.Errorf("clone DigitalHuman role assignments: %w", err)
		}
	}
	bindings, err := r.ListResourceBindingRows(ctx, scope, digitalHumanID, fromVersionID)
	if err != nil {
		return err
	}
	for _, item := range bindings {
		item.ID = uuid.New()
		item.DigitalHumanVersionID = toVersionID
		item.CreatedAt, item.UpdatedAt = now, now
	}
	if len(bindings) > 0 {
		if err := r.db.WithContext(ctx).Create(&bindings).Error; err != nil {
			return fmt.Errorf("clone DigitalHuman resource bindings: %w", err)
		}
	}
	return nil
}

func (r *GormRepository) ListSnapshot(ctx context.Context, ref resourcebindings.SnapshotRef) ([]resourcebindings.Binding, error) {
	ref, version, err := r.digitalHumanSnapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	scope := Scope{OrganizationID: ref.Subject.OrganizationID, WorkspaceID: ref.Subject.WorkspaceID, ActorID: ref.Subject.ID}
	rows, err := r.ListResourceBindingRows(ctx, scope, ref.Subject.ID, version.ID)
	if err != nil {
		return nil, err
	}
	bindings := make([]resourcebindings.Binding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, projectDigitalHumanBinding(ref, row))
	}
	return bindings, nil
}

func (r *GormRepository) Verify(ctx context.Context, ref resourcebindings.SnapshotRef, match resourcebindings.Match) (resourcebindings.Decision, error) {
	ref, version, err := r.digitalHumanSnapshot(ctx, ref)
	if err != nil {
		return resourcebindings.Decision{}, err
	}
	match, err = resourcebindings.NormalizeMatch(match)
	if err != nil {
		return resourcebindings.Decision{}, err
	}
	query := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND binding_type = ? AND resource_id = ? AND parent_resource_id = ?",
		ref.Subject.OrganizationID, ref.Subject.WorkspaceID, ref.Subject.ID, version.ID, match.Type, match.ResourceID, match.ParentResourceID,
	)
	if match.AccessMode == resourcebindings.AccessModeRead {
		query = query.Where("access_mode IN ?", []resourcebindings.AccessMode{resourcebindings.AccessModeRead, resourcebindings.AccessModeWrite})
	} else {
		query = query.Where("access_mode = ?", match.AccessMode)
	}
	var row ResourceBinding
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resourcebindings.Decision{Reason: "resource_not_bound"}, nil
		}
		return resourcebindings.Decision{}, fmt.Errorf("verify DigitalHuman resource binding: %w", err)
	}
	binding := projectDigitalHumanBinding(ref, &row)
	if match.ActionID != "" && !resourcebindings.AllowsIntegrationAction(binding.Metadata, match.ActionID) {
		return resourcebindings.Decision{Binding: &binding, Reason: "integration_action_not_allowed"}, nil
	}
	return resourcebindings.Decision{Allowed: true, Binding: &binding}, nil
}

func (r *GormRepository) digitalHumanSnapshot(ctx context.Context, ref resourcebindings.SnapshotRef) (resourcebindings.SnapshotRef, *Version, error) {
	ref, err := resourcebindings.ValidateSnapshotRef(ref)
	if err != nil {
		return resourcebindings.SnapshotRef{}, nil, err
	}
	if ref.Subject.Type != runtimeidentity.SubjectTypeDigitalHuman {
		return resourcebindings.SnapshotRef{}, nil, fmt.Errorf("%w: DigitalHuman repository requires DigitalHuman subject", resourcebindings.ErrInvalidReference)
	}
	query := r.db.WithContext(ctx).Model(&Version{}).
		Joins("JOIN digital_humans definition ON definition.id = digital_human_versions.digital_human_id AND definition.deleted_at IS NULL").
		Where("digital_human_versions.digital_human_id = ? AND digital_human_versions.organization_id = ? AND digital_human_versions.workspace_id = ?",
			ref.Subject.ID, ref.Subject.OrganizationID, ref.Subject.WorkspaceID)
	if ref.VersionID != nil {
		query = query.Where("digital_human_versions.id = ?", *ref.VersionID)
	}
	state := VersionStateDraft
	if ref.Phase == resourcebindings.PhasePublished {
		state = VersionStatePublished
	}
	var version Version
	if err := query.Where("digital_human_versions.state = ?", state).Take(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resourcebindings.SnapshotRef{}, nil, ErrVersionNotFound
		}
		return resourcebindings.SnapshotRef{}, nil, fmt.Errorf("resolve DigitalHuman binding snapshot: %w", err)
	}
	versionID := version.ID
	ref.VersionID = &versionID
	return ref, &version, nil
}

func projectDigitalHumanBinding(ref resourcebindings.SnapshotRef, row *ResourceBinding) resourcebindings.Binding {
	metadata := make(map[string]interface{})
	_ = json.Unmarshal(row.Metadata, &metadata)
	return resourcebindings.Binding{
		ID: row.ID, Snapshot: ref, Type: row.BindingType, ResourceID: row.ResourceID,
		ParentResourceID: row.ParentResourceID, DisplayName: row.DisplayName, AccessMode: row.AccessMode,
		Revision: row.BindingRevision, CredentialRef: row.CredentialRef, Metadata: metadata,
		Authorization: resourcebindings.AuthorizationEvidence{AuthorizedBy: row.AuthorizedBy, AuthorizedAt: row.AuthorizedAt},
	}
}

func (r *GormRepository) scopedRoleTemplates(ctx context.Context, scope Scope) *gorm.DB {
	return r.db.WithContext(ctx).Model(&RoleTemplate{}).
		Where("organization_id = ? AND workspace_id = ? AND deleted_at IS NULL", scope.OrganizationID, scope.WorkspaceID)
}

func (r *GormRepository) scopedRoleTemplateVersions(ctx context.Context, scope Scope, templateID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).Model(&RoleTemplateVersion{}).
		Where("organization_id = ? AND workspace_id = ? AND role_template_id = ?", scope.OrganizationID, scope.WorkspaceID, templateID)
}

func jsonClone(value datatypes.JSON) datatypes.JSON { return append(datatypes.JSON(nil), value...) }
