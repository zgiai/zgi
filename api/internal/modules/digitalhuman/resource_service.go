package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
)

const maxResourceBindings = 512

var _ resourcebindings.Reader = (*GormRepository)(nil)

func (s *Service) ListDraftResourceBindings(ctx context.Context, scope Scope, id uuid.UUID) (*DraftResourceBindings, error) {
	definition, draft, err := s.loadDigitalHumanDraft(ctx, scope, id, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.bindings.list")
	}
	items, err := s.repo.ListResourceBindingRows(ctx, scope, definition.ID, draft.ID)
	if err != nil {
		return nil, applicationError(err, "digital_human.bindings.list")
	}
	return &DraftResourceBindings{DraftLockVersion: draft.LockVersion, Items: items}, nil
}

func (s *Service) ReplaceDraftResourceBindings(ctx context.Context, scope Scope, id uuid.UUID, request ReplaceResourceBindingsRequest) (*DraftResourceBindings, error) {
	if request.ExpectedDraftLockVersion < 1 || len(request.Items) > maxResourceBindings {
		return nil, applicationError(ErrResourceBindingInvalid, "digital_human.bindings.replace")
	}
	var result DraftResourceBindings
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		definition, err := tx.LockDefinition(ctx, scope, id)
		if err != nil {
			return err
		}
		draft, err := tx.Draft(ctx, scope, definition, true)
		if err != nil {
			return err
		}
		if draft.LockVersion != request.ExpectedDraftLockVersion {
			return ErrRevisionConflict
		}
		items, err := buildResourceBindings(scope, definition.ID, draft.ID, request.Items, s.now())
		if err != nil {
			return err
		}
		if err := tx.ReplaceDraftResourceBindings(ctx, scope, definition.ID, draft.ID, items); err != nil {
			return err
		}
		draft.UpdatedAt = s.now()
		if err := tx.UpdateDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}
		result = DraftResourceBindings{DraftLockVersion: draft.LockVersion, Items: items}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.bindings.replace")
	}
	return &result, nil
}

func (s *Service) loadDigitalHumanDraft(ctx context.Context, scope Scope, id uuid.UUID, lock bool) (*Definition, *Version, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, nil, ErrInvalidRequest
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if lock {
		definition, err = s.repo.LockDefinition(ctx, scope, id)
	}
	if err != nil {
		return nil, nil, err
	}
	draft, err := s.repo.Draft(ctx, scope, definition, lock)
	return definition, draft, err
}

func buildResourceBindings(scope Scope, digitalHumanID, draftID uuid.UUID, inputs []ResourceBindingInput, now time.Time) ([]*ResourceBinding, error) {
	items := make([]*ResourceBinding, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	actorID := scope.ActorID
	for _, input := range inputs {
		match, err := resourcebindings.NormalizeMatch(resourcebindings.Match{
			Type: input.Type, ResourceID: input.ResourceID, ParentResourceID: input.ParentResourceID, AccessMode: input.AccessMode,
		})
		if err != nil || strings.TrimSpace(input.ResourceRevision) == "" || len(input.ResourceRevision) > 255 || len(input.CredentialRef) > 512 || !validBindingMode(match.Type, match.AccessMode) {
			return nil, ErrResourceBindingInvalid
		}
		input.DisplayName, input.ResourceRevision, input.CredentialRef = strings.TrimSpace(input.DisplayName), strings.TrimSpace(input.ResourceRevision), strings.TrimSpace(input.CredentialRef)
		if containsCredentialMaterial(input.Metadata) {
			return nil, ErrResourceBindingInvalid
		}
		metadata, err := safeJSON(input.Metadata)
		if err != nil {
			return nil, ErrResourceBindingInvalid
		}
		key := strings.Join([]string{string(match.Type), match.ParentResourceID, match.ResourceID, string(match.AccessMode)}, "\x00")
		if _, ok := seen[key]; ok {
			return nil, ErrResourceBindingInvalid
		}
		seen[key] = struct{}{}
		revision, err := hashRevision(struct {
			Type             resourcebindings.BindingType `json:"type"`
			ResourceID       string                       `json:"resource_id"`
			ParentResourceID string                       `json:"parent_resource_id"`
			AccessMode       resourcebindings.AccessMode  `json:"access_mode"`
			ResourceRevision string                       `json:"resource_revision"`
			CredentialRef    string                       `json:"credential_ref,omitempty"`
			Metadata         json.RawMessage              `json:"metadata"`
		}{match.Type, match.ResourceID, match.ParentResourceID, match.AccessMode, input.ResourceRevision, input.CredentialRef, json.RawMessage(metadata)})
		if err != nil {
			return nil, err
		}
		items = append(items, &ResourceBinding{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, DigitalHumanVersionID: draftID,
			BindingType: match.Type, ResourceID: match.ResourceID, ParentResourceID: match.ParentResourceID,
			DisplayName: input.DisplayName, AccessMode: match.AccessMode, ResourceRevision: input.ResourceRevision,
			BindingRevision: revision, CredentialRef: input.CredentialRef, Metadata: metadata,
			AuthorizedBy: &actorID, AuthorizedAt: &now, CreatedAt: now, UpdatedAt: now,
		})
	}
	return items, nil
}

func validBindingMode(bindingType resourcebindings.BindingType, mode resourcebindings.AccessMode) bool {
	switch bindingType {
	case resourcebindings.BindingTypeSkill, resourcebindings.BindingTypeWorkflow, resourcebindings.BindingTypeIntegrationConnection:
		return mode == resourcebindings.AccessModeExecute
	case resourcebindings.BindingTypeKnowledgeDataset:
		return mode == resourcebindings.AccessModeRead
	case resourcebindings.BindingTypeDatabase, resourcebindings.BindingTypeDatabaseTable:
		return mode == resourcebindings.AccessModeRead || mode == resourcebindings.AccessModeWrite
	default:
		return false
	}
}

func containsCredentialMaterial(value any) bool {
	forbidden := map[string]struct{}{"secret": {}, "password": {}, "api_key": {}, "apikey": {}, "access_token": {}, "refresh_token": {}, "cookie": {}, "token": {}}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if _, blocked := forbidden[normalized]; blocked || containsCredentialMaterial(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsCredentialMaterial(item) {
				return true
			}
		}
	}
	return false
}

func safeJSON(value any) (datatypes.JSON, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > maxPolicyBytes {
		return nil, ErrInvalidRequest
	}
	return datatypes.JSON(encoded), nil
}
