package digitalhuman

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	restoreBindingAdded   = "added"
	restoreBindingRemoved = "removed"
	restoreBindingUpdated = "updated"
)

// PreviewVersionRestore computes a stable, actor-bound restore confirmation.
func (s *Service) PreviewVersionRestore(ctx context.Context, scope Scope, id, versionID uuid.UUID) (*VersionRestorePreview, error) {
	if !validScope(scope) || id == uuid.Nil || versionID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.version.restore_preview")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.version.restore_preview")
	}
	draft, err := s.repo.Draft(ctx, scope, definition, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.version.restore_preview")
	}
	target, err := s.repo.PublishedVersion(ctx, scope, id, versionID)
	if err != nil {
		return nil, applicationError(err, "digital_human.version.restore_preview")
	}
	preview, err := s.versionRestorePreview(ctx, scope, definition, draft, target)
	if err != nil {
		return nil, applicationError(err, "digital_human.version.restore_preview")
	}
	return preview, nil
}

// RestoreVersion replaces only the mutable versioned configuration and resource bindings.
func (s *Service) RestoreVersion(ctx context.Context, scope Scope, id uuid.UUID, request RestoreVersionRequest) (*Detail, error) {
	if !validScope(scope) || id == uuid.Nil || request.VersionID == uuid.Nil || request.ExpectedRevision < 1 ||
		request.ExpectedDraftLockVersion < 1 || strings.TrimSpace(request.ImpactToken) == "" {
		return nil, applicationError(ErrInvalidRequest, "digital_human.version.restore")
	}
	var result Detail
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		definition, err := tx.LockDefinition(ctx, scope, id)
		if err != nil {
			return err
		}
		if definition.Revision != request.ExpectedRevision {
			return ErrRevisionConflict
		}
		draft, err := tx.Draft(ctx, scope, definition, true)
		if err != nil {
			return err
		}
		if draft.LockVersion != request.ExpectedDraftLockVersion {
			return ErrRevisionConflict
		}
		target, err := tx.PublishedVersion(ctx, scope, id, request.VersionID)
		if err != nil {
			return err
		}
		preview, err := s.versionRestorePreviewWithRepository(ctx, tx, scope, definition, draft, target)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(preview.ImpactToken), []byte(strings.TrimSpace(request.ImpactToken))) != 1 {
			return ErrVersionRestoreImpactChanged
		}

		draft.IdentityName = target.IdentityName
		draft.Persona = target.Persona
		draft.Goals = cloneJSON(target.Goals)
		draft.Rules = cloneJSON(target.Rules)
		draft.DefaultModel = target.DefaultModel
		draft.MemoryPolicy = cloneJSON(target.MemoryPolicy)
		draft.TriggerPolicy = cloneJSON(target.TriggerPolicy)
		draft.IdentitySnapshot = datatypes.JSON([]byte(`{}`))
		draft.UpdatedAt = s.now()
		draft.Revision, err = configRevision(draft)
		if err != nil {
			return err
		}
		if err := tx.UpdateDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}
		if _, err := tx.RestoreDraftResourceBindings(ctx, scope, id, target.ID, draft.ID, scope.ActorID, draft.UpdatedAt); err != nil {
			return err
		}
		published, err := tx.Published(ctx, scope, definition)
		if err != nil {
			return err
		}
		result = Detail{Definition: definition, Draft: draft, Published: published}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.version.restore")
	}
	return &result, nil
}

func (s *Service) versionRestorePreview(ctx context.Context, scope Scope, definition *Definition, draft, target *Version) (*VersionRestorePreview, error) {
	return s.versionRestorePreviewWithRepository(ctx, s.repo, scope, definition, draft, target)
}

func (s *Service) versionRestorePreviewWithRepository(ctx context.Context, repo *GormRepository, scope Scope, definition *Definition, draft, target *Version) (*VersionRestorePreview, error) {
	currentBindings, err := repo.ListResourceBindingRows(ctx, scope, definition.ID, draft.ID)
	if err != nil {
		return nil, err
	}
	targetBindings, err := repo.ListResourceBindingRows(ctx, scope, definition.ID, target.ID)
	if err != nil {
		return nil, err
	}
	changedFields := versionChangedFields(draft, target)
	bindingChanges := versionBindingChanges(currentBindings, targetBindings)
	token, err := hashRevision(struct {
		ActorID            uuid.UUID                     `json:"actor_id"`
		DigitalHumanID     uuid.UUID                     `json:"digital_human_id"`
		VersionID          uuid.UUID                     `json:"version_id"`
		DefinitionRevision int64                         `json:"definition_revision"`
		DraftLockVersion   int64                         `json:"draft_lock_version"`
		DraftRevision      string                        `json:"draft_revision"`
		TargetRevision     string                        `json:"target_revision"`
		ChangedFields      []string                      `json:"changed_fields"`
		BindingChanges     []VersionRestoreBindingImpact `json:"binding_changes"`
	}{scope.ActorID, definition.ID, target.ID, definition.Revision, draft.LockVersion, draft.Revision, target.Revision, changedFields, bindingChanges})
	if err != nil {
		return nil, err
	}
	return &VersionRestorePreview{
		VersionID: target.ID, VersionNumber: target.VersionNumber,
		IsCurrent:                     definition.PublishedVersionID != nil && *definition.PublishedVersionID == target.ID,
		WillDiscardUnpublishedChanges: len(changedFields) > 0 || len(bindingChanges) > 0,
		ChangedFields:                 changedFields, BindingChanges: bindingChanges, ImpactToken: token,
	}, nil
}

func versionChangedFields(current, target *Version) []string {
	fields := make([]string, 0, 7)
	if current.IdentityName != target.IdentityName {
		fields = append(fields, "identity_name")
	}
	if current.Persona != target.Persona {
		fields = append(fields, "persona")
	}
	if !bytes.Equal(current.Goals, target.Goals) {
		fields = append(fields, "goals")
	}
	if !bytes.Equal(current.Rules, target.Rules) {
		fields = append(fields, "rules")
	}
	if current.DefaultModel != target.DefaultModel {
		fields = append(fields, "default_model")
	}
	if !bytes.Equal(current.MemoryPolicy, target.MemoryPolicy) {
		fields = append(fields, "memory_policy")
	}
	if !bytes.Equal(current.TriggerPolicy, target.TriggerPolicy) {
		fields = append(fields, "trigger_policy")
	}
	return fields
}

func versionBindingChanges(current, target []*ResourceBinding) []VersionRestoreBindingImpact {
	currentByKey := make(map[string]*ResourceBinding, len(current))
	targetByKey := make(map[string]*ResourceBinding, len(target))
	for _, item := range current {
		currentByKey[restoreBindingKey(item)] = item
	}
	for _, item := range target {
		targetByKey[restoreBindingKey(item)] = item
	}
	changes := make([]VersionRestoreBindingImpact, 0)
	for key, item := range targetByKey {
		change := restoreBindingAdded
		if existing, ok := currentByKey[key]; ok {
			if existing.BindingRevision == item.BindingRevision {
				continue
			}
			change = restoreBindingUpdated
		}
		changes = append(changes, restoreBindingImpact(change, item))
	}
	for key, item := range currentByKey {
		if _, ok := targetByKey[key]; !ok {
			changes = append(changes, restoreBindingImpact(restoreBindingRemoved, item))
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		left := fmt.Sprintf("%s\x00%s\x00%s\x00%s", changes[i].Change, changes[i].Type, changes[i].ParentResourceID, changes[i].ResourceID)
		right := fmt.Sprintf("%s\x00%s\x00%s\x00%s", changes[j].Change, changes[j].Type, changes[j].ParentResourceID, changes[j].ResourceID)
		return left < right
	})
	return changes
}

func restoreBindingKey(item *ResourceBinding) string {
	if item == nil {
		return ""
	}
	return strings.Join([]string{string(item.BindingType), item.ParentResourceID, item.ResourceID, string(item.AccessMode)}, "\x00")
}

func restoreBindingImpact(change string, item *ResourceBinding) VersionRestoreBindingImpact {
	return VersionRestoreBindingImpact{Change: change, Type: item.BindingType, ResourceID: item.ResourceID, ParentResourceID: item.ParentResourceID, DisplayName: item.DisplayName}
}
