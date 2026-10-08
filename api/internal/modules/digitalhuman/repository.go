package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormRepository persists DigitalHuman aggregates and their versions.
type GormRepository struct {
	db               *gorm.DB
	connectorCatalog ConnectorCatalog
}

// NewRepository constructs a DigitalHuman repository.
func NewRepository(db *gorm.DB) *GormRepository {
	if db == nil {
		panic("digital human repository requires db")
	}
	return &GormRepository{db: db}
}

// Transaction runs fn atomically with a transaction-bound repository.
func (r *GormRepository) Transaction(ctx context.Context, fn func(*GormRepository) error) error {
	if fn == nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx, connectorCatalog: r.connectorCatalog})
	})
}

// CreateDefinition inserts aggregate metadata.
func (r *GormRepository) CreateDefinition(ctx context.Context, definition *Definition) error {
	if definition == nil || definition.ID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(definition).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrNameConflict
		}
		return fmt.Errorf("create digital human definition: %w", err)
	}
	return nil
}

// CreateVersion inserts a draft or published configuration version.
func (r *GormRepository) CreateVersion(ctx context.Context, version *Version) error {
	if version == nil || version.ID == uuid.Nil || version.DigitalHumanID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(version).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRevisionConflict
		}
		return fmt.Errorf("create digital human version: %w", err)
	}
	return nil
}

// AttachInitialDraft records the draft created in the same transaction as the definition.
func (r *GormRepository) AttachInitialDraft(ctx context.Context, scope Scope, id, draftID uuid.UUID) error {
	result := r.scopedDefinitions(ctx, scope).
		Where("id = ? AND draft_version_id IS NULL", id).
		Update("draft_version_id", draftID)
	if result.Error != nil {
		return fmt.Errorf("attach initial digital human draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// Definition returns scoped aggregate metadata.
func (r *GormRepository) Definition(ctx context.Context, scope Scope, id uuid.UUID) (*Definition, error) {
	return r.definition(ctx, scope, id, false)
}

// LockDefinition returns scoped metadata locked for a lifecycle transaction.
func (r *GormRepository) LockDefinition(ctx context.Context, scope Scope, id uuid.UUID) (*Definition, error) {
	return r.definition(ctx, scope, id, true)
}

func (r *GormRepository) definition(ctx context.Context, scope Scope, id uuid.UUID, lock bool) (*Definition, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	query := r.scopedDefinitions(ctx, scope).Where("id = ?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var definition Definition
	if err := query.Take(&definition).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get digital human definition: %w", err)
	}
	return &definition, nil
}

// Draft returns the current mutable configuration.
func (r *GormRepository) Draft(ctx context.Context, scope Scope, definition *Definition, lock bool) (*Version, error) {
	if definition == nil || definition.DraftVersionID == nil || *definition.DraftVersionID == uuid.Nil {
		return nil, ErrVersionNotFound
	}
	query := r.scopedVersions(ctx, scope, definition.ID).
		Where("id = ? AND state = ?", *definition.DraftVersionID, VersionStateDraft)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var version Version
	if err := query.Take(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("get digital human draft: %w", err)
	}
	return &version, nil
}

// Published returns the current published configuration, if one exists.
func (r *GormRepository) Published(ctx context.Context, scope Scope, definition *Definition) (*Version, error) {
	if definition == nil || definition.PublishedVersionID == nil || *definition.PublishedVersionID == uuid.Nil {
		return nil, nil
	}
	var version Version
	err := r.scopedVersions(ctx, scope, definition.ID).
		Where("id = ? AND state = ?", *definition.PublishedVersionID, VersionStatePublished).
		Take(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("get published digital human version: %w", err)
	}
	return &version, nil
}

// PublishedVersion returns one immutable publication under the scoped aggregate.
func (r *GormRepository) PublishedVersion(ctx context.Context, scope Scope, definitionID, versionID uuid.UUID) (*Version, error) {
	if !validScope(scope) || definitionID == uuid.Nil || versionID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var version Version
	err := r.scopedVersions(ctx, scope, definitionID).
		Where("id = ? AND state = ?", versionID, VersionStatePublished).
		Take(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get published digital human version: %w", err)
	}
	return &version, nil
}

// ListDefinitions returns one newest-first workspace page.
func (r *GormRepository) ListDefinitions(ctx context.Context, scope Scope, query ListQuery) ([]*Definition, int64, error) {
	if !validScope(scope) || query.Page < 1 || query.PageSize < 1 {
		return nil, 0, ErrInvalidRequest
	}
	db := r.scopedDefinitions(ctx, scope)
	if query.Search != "" {
		db = db.Where(`LOWER(name) LIKE LOWER(?) ESCAPE '\'`, "%"+escapeSearch(query.Search)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count digital humans: %w", err)
	}
	items := make([]*Definition, 0, query.PageSize)
	// State transitions update updated_at. Keep card order stable across
	// publish/unpublish/status edits by sorting on creation time instead of the
	// mutable audit timestamp.
	if err := db.Order("created_at DESC, id DESC").
		Offset((query.Page - 1) * query.PageSize).
		Limit(query.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list digital humans: %w", err)
	}
	return items, total, nil
}

// NameExists reports whether another live definition uses name in the scope.
func (r *GormRepository) NameExists(ctx context.Context, scope Scope, name string, excludeID uuid.UUID) (bool, error) {
	if !validScope(scope) || strings.TrimSpace(name) == "" {
		return false, ErrInvalidRequest
	}
	query := r.scopedDefinitions(ctx, scope).Where("LOWER(name) = LOWER(?)", strings.TrimSpace(name))
	if excludeID != uuid.Nil {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check digital human name: %w", err)
	}
	return count > 0, nil
}

// UpdateDefinitionCAS updates metadata only when revision still matches.
func (r *GormRepository) UpdateDefinitionCAS(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64, updates map[string]any) error {
	if !validScope(scope) || id == uuid.Nil || expectedRevision < 1 || len(updates) == 0 {
		return ErrInvalidRequest
	}
	updates["revision"] = gorm.Expr("revision + 1")
	updates["updated_by"] = scope.ActorID
	updates["updated_at"] = time.Now().UTC()
	result := r.scopedDefinitions(ctx, scope).
		Where("id = ? AND revision = ?", id, expectedRevision).
		Updates(updates)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return ErrNameConflict
		}
		return fmt.Errorf("update digital human definition: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// UpdateDraftCAS updates only the current mutable draft.
func (r *GormRepository) UpdateDraftCAS(ctx context.Context, scope Scope, version *Version, expectedLockVersion int64) error {
	if version == nil || !validScope(scope) || version.ID == uuid.Nil || expectedLockVersion < 1 {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&Version{}).
		Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND state = ? AND lock_version = ?",
			version.ID, version.DigitalHumanID, scope.OrganizationID, scope.WorkspaceID, VersionStateDraft, expectedLockVersion).
		Updates(map[string]any{
			"identity_name":       version.IdentityName,
			"persona":             version.Persona,
			"goals":               version.Goals,
			"rules":               version.Rules,
			"default_model":       version.DefaultModel,
			"memory_policy":       version.MemoryPolicy,
			"trigger_policy":      version.TriggerPolicy,
			"presentation_policy": version.PresentationPolicy,
			"execution_runtime":   executionRuntimeJSON(version.ExecutionRuntime),
			"revision":            version.Revision,
			"lock_version":        gorm.Expr("lock_version + 1"),
			"updated_by":          scope.ActorID,
			"updated_at":          version.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update digital human draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	version.LockVersion = expectedLockVersion + 1
	return nil
}

// NextPublishedVersionNumber returns the next version number under a locked definition.
func (r *GormRepository) NextPublishedVersionNumber(ctx context.Context, definitionID uuid.UUID) (int, error) {
	var current int
	if err := r.db.WithContext(ctx).Model(&Version{}).
		Where("digital_human_id = ? AND state = ?", definitionID, VersionStatePublished).
		Select("COALESCE(MAX(version_number), 0)").Scan(&current).Error; err != nil {
		return 0, fmt.Errorf("get next digital human version number: %w", err)
	}
	return current + 1, nil
}

// PromoteDraftCAS converts a locked draft row into an immutable publication.
func (r *GormRepository) PromoteDraftCAS(ctx context.Context, scope Scope, draft *Version, expectedLockVersion int64) error {
	result := r.db.WithContext(ctx).Model(&Version{}).
		Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND state = ? AND lock_version = ?",
			draft.ID, draft.DigitalHumanID, scope.OrganizationID, scope.WorkspaceID, VersionStateDraft, expectedLockVersion).
		Updates(map[string]any{
			"state":             VersionStatePublished,
			"version_number":    draft.VersionNumber,
			"name":              draft.Name,
			"description":       draft.Description,
			"identity_snapshot": draft.IdentitySnapshot,
			"revision":          draft.Revision,
			"published_by":      draft.PublishedBy,
			"published_at":      draft.PublishedAt,
			"updated_by":        scope.ActorID,
			"updated_at":        draft.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("publish digital human draft: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// AdvancePublicationCAS points the aggregate at its new draft and publication.
func (r *GormRepository) AdvancePublicationCAS(ctx context.Context, scope Scope, definition *Definition, expectedRevision int64) error {
	result := r.scopedDefinitions(ctx, scope).
		Where("id = ? AND revision = ?", definition.ID, expectedRevision).
		Updates(map[string]any{
			"status":               StatusActive,
			"draft_version_id":     definition.DraftVersionID,
			"published_version_id": definition.PublishedVersionID,
			"revision":             gorm.Expr("revision + 1"),
			"updated_by":           scope.ActorID,
			"updated_at":           definition.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("advance digital human publication: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	definition.Revision = expectedRevision + 1
	return nil
}

// ListPublishedVersions returns one newest-first publication page.
func (r *GormRepository) ListPublishedVersions(ctx context.Context, scope Scope, definitionID uuid.UUID, query ListQuery) ([]*Version, int64, error) {
	if !validScope(scope) || definitionID == uuid.Nil || query.Page < 1 || query.PageSize < 1 {
		return nil, 0, ErrInvalidRequest
	}
	db := r.scopedVersions(ctx, scope, definitionID).Where("state = ?", VersionStatePublished)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count published digital human versions: %w", err)
	}
	items := make([]*Version, 0, query.PageSize)
	if err := db.Order("version_number DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list published digital human versions: %w", err)
	}
	return items, total, nil
}

// PublishedVersionCount counts deletion blockers owned by the version store.
func (r *GormRepository) PublishedVersionCount(ctx context.Context, scope Scope, definitionID uuid.UUID) (int64, error) {
	var count int64
	if err := r.scopedVersions(ctx, scope, definitionID).
		Where("state = ?", VersionStatePublished).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count digital human deletion impact: %w", err)
	}
	return count, nil
}

// SoftDeleteCAS soft-deletes an aggregate at the expected revision. Published
// versions are immutable, so an aggregate that still points at a published
// version must be taken offline (status=disabled) before it can be removed.
func (r *GormRepository) SoftDeleteCAS(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64, now time.Time) error {
	result := r.scopedDefinitions(ctx, scope).
		Where(
			"id = ? AND revision = ? AND (published_version_id IS NULL OR status = ?)",
			id,
			expectedRevision,
			StatusDisabled,
		).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
			"updated_by": scope.ActorID,
			"revision":   gorm.Expr("revision + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("delete digital human: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func (r *GormRepository) scopedDefinitions(ctx context.Context, scope Scope) *gorm.DB {
	return r.db.WithContext(ctx).Model(&Definition{}).
		Where("organization_id = ? AND workspace_id = ? AND deleted_at IS NULL", scope.OrganizationID, scope.WorkspaceID)
}

func (r *GormRepository) scopedVersions(ctx context.Context, scope Scope, definitionID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).Model(&Version{}).
		Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ?", scope.OrganizationID, scope.WorkspaceID, definitionID)
}

func validScope(scope Scope) bool {
	return validScopeBoundary(scope) && scope.ActorID != uuid.Nil
}

func validScopeBoundary(scope Scope) bool {
	return scope.OrganizationID != uuid.Nil && scope.WorkspaceID != uuid.Nil
}

func escapeSearch(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") || strings.Contains(message, "unique violation")
}
