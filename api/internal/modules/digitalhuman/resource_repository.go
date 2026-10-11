package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

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
	return r.replaceDraftResourceBindings(ctx, scope, digitalHumanID, draftID, items, false)
}

// preserveUnchanged is used only for an explicit read/modify/write of this draft.
// Version restoration must validate every new binding instance instead.
func (r *GormRepository) replaceDraftResourceBindings(ctx context.Context, scope Scope, digitalHumanID, draftID uuid.UUID, items []*ResourceBinding, preserveUnchanged bool) error {
	existing, err := r.ListResourceBindingRows(ctx, scope, digitalHumanID, draftID)
	if err != nil {
		return err
	}
	keepOnly := make(map[uuid.UUID]bool)
	for _, item := range items {
		for _, old := range existing {
			if preserveUnchanged && old.ID == item.ID && sameResourceBindingContent(old, item) {
				keepOnly[item.ID] = true
				item.ID, item.AuthorizedBy, item.AuthorizedAt, item.CreatedAt = old.ID, old.AuthorizedBy, old.AuthorizedAt, old.CreatedAt
				break
			}
		}
	}
	if err := r.syncConnectorBindings(ctx, scope, digitalHumanID, draftID, core.ScopeDraft, items, keepOnly); err != nil {
		return err
	}
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
	bindings, err := r.ListResourceBindingRows(ctx, scope, digitalHumanID, fromVersionID)
	if err != nil {
		return err
	}
	if err := validateVersionResourceBindings(bindings); err != nil {
		return err
	}
	if err := r.syncConnectorBindings(ctx, scope, digitalHumanID, fromVersionID, core.ScopePublished, bindings, nil); err != nil {
		return err
	}
	for _, item := range bindings {
		item.ID = uuid.New()
		item.DigitalHumanVersionID = toVersionID
		item.CreatedAt, item.UpdatedAt = now, now
	}
	if err := r.syncConnectorBindings(ctx, scope, digitalHumanID, toVersionID, core.ScopeDraft, bindings, nil); err != nil {
		return err
	}
	if len(bindings) > 0 {
		if err := r.db.WithContext(ctx).Create(&bindings).Error; err != nil {
			return fmt.Errorf("clone DigitalHuman resource bindings: %w", err)
		}
	}
	return nil
}

// RestoreDraftResourceBindings replaces the mutable draft bindings with copies from a publication.
func (r *GormRepository) RestoreDraftResourceBindings(ctx context.Context, scope Scope, digitalHumanID, fromVersionID, draftID uuid.UUID, actorID uuid.UUID, now time.Time) ([]*ResourceBinding, error) {
	bindings, err := r.ListResourceBindingRows(ctx, scope, digitalHumanID, fromVersionID)
	if err != nil {
		return nil, err
	}
	if err := validateVersionResourceBindings(bindings); err != nil {
		return nil, err
	}
	for _, item := range bindings {
		item.ID = uuid.New()
		item.DigitalHumanVersionID = draftID
		if item.BindingType != resourcebindings.BindingTypeConnector && item.BindingType != resourcebindings.BindingTypeIntegrationConnection {
			item.AuthorizedBy = &actorID
			item.AuthorizedAt = &now
		}
		item.CreatedAt, item.UpdatedAt = now, now
	}
	if err := r.ReplaceDraftResourceBindings(ctx, scope, digitalHumanID, draftID, bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

// Historical integration references can only be retained in their existing
// draft or read from history. Publishing/restoring must not create new instances.
func validateVersionResourceBindings(rows []*ResourceBinding) error {
	for _, row := range rows {
		if row.BindingType == resourcebindings.BindingTypeIntegrationConnection {
			return ErrResourceBindingInvalid
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
