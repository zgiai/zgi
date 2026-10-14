package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"reflect"
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
	return &DraftResourceBindings{DraftVersionID: draft.ID, DraftLockVersion: draft.LockVersion, Items: items}, nil
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
		if draft.LockVersion != request.ExpectedDraftLockVersion || request.ExpectedDraftVersionID != nil && *request.ExpectedDraftVersionID != draft.ID {
			return ErrRevisionConflict
		}
		existing, err := tx.ListResourceBindingRows(ctx, scope, definition.ID, draft.ID)
		if err != nil {
			return err
		}
		items, err := buildResourceBindings(scope, definition.ID, draft.ID, request.Items, existing, s.now())
		if err != nil {
			return err
		}
		if err := tx.replaceDraftResourceBindings(ctx, scope, definition.ID, draft.ID, items, true); err != nil {
			return err
		}
		draft.UpdatedAt = s.now()
		if err := tx.UpdateDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}
		result = DraftResourceBindings{DraftVersionID: draft.ID, DraftLockVersion: draft.LockVersion, Items: items}
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

func buildResourceBindings(scope Scope, digitalHumanID, draftID uuid.UUID, inputs []ResourceBindingInput, existing []*ResourceBinding, now time.Time) ([]*ResourceBinding, error) {
	items := make([]*ResourceBinding, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	seenIDs := make(map[uuid.UUID]bool, len(inputs))
	byID := make(map[uuid.UUID]*ResourceBinding, len(existing))
	for _, row := range existing {
		if row.OrganizationID == scope.OrganizationID && row.WorkspaceID == scope.WorkspaceID && row.DigitalHumanID == digitalHumanID && row.DigitalHumanVersionID == draftID {
			byID[row.ID] = row
		}
	}
	actorID := scope.ActorID
	for _, input := range inputs {
		match, err := resourcebindings.NormalizeMatch(resourcebindings.Match{
			Type: input.Type, ResourceID: input.ResourceID, ParentResourceID: input.ParentResourceID, AccessMode: input.AccessMode,
		})
		legacy := match.Type == resourcebindings.BindingTypeIntegrationConnection
		if err != nil || strings.TrimSpace(input.ResourceRevision) == "" || len(input.ResourceRevision) > 255 || len(input.CredentialRef) > 512 || (!validBindingMode(match.Type, match.AccessMode) && !legacy) {
			return nil, ErrResourceBindingInvalid
		}
		if match.Type == resourcebindings.BindingTypeWorkflow {
			// Workflow bindings are keyed by the workflow Agent id, matching the key
			// Skills authorization and binding verification derive at runtime.
			workflowAgentID := strings.ToLower(stringMapValue(input.Metadata, "workflow_agent_id"))
			if workflowAgentID == "" || (match.ParentResourceID != "" && !strings.EqualFold(match.ParentResourceID, workflowAgentID)) {
				return nil, ErrResourceBindingInvalid
			}
			match.ParentResourceID = workflowAgentID
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
		var prior *ResourceBinding
		if input.ID != nil {
			prior = byID[*input.ID]
			if *input.ID == uuid.Nil || prior == nil || seenIDs[*input.ID] || prior.BindingType != match.Type || prior.ResourceID != match.ResourceID || prior.ParentResourceID != match.ParentResourceID || prior.AccessMode != match.AccessMode {
				return nil, ErrResourceBindingInvalid
			}
			seenIDs[*input.ID] = true
		} else if !legacy && match.Type != resourcebindings.BindingTypeConnector {
			// Preserve the existing no-ID behavior for unrelated resource editors.
			for _, row := range existing {
				if row.BindingType == match.Type && row.ResourceID == match.ResourceID && row.ParentResourceID == match.ParentResourceID && row.AccessMode == match.AccessMode && row.DisplayName == input.DisplayName && row.ResourceRevision == input.ResourceRevision && row.CredentialRef == input.CredentialRef && equivalentResourceMetadata(row.Metadata, metadata) {
					prior = row
					break
				}
			}
		}
		if prior != nil && prior.DisplayName == input.DisplayName && prior.ResourceRevision == input.ResourceRevision && prior.CredentialRef == input.CredentialRef && equivalentResourceMetadata(prior.Metadata, metadata) {
			// This is retention in the same locked draft, never a fresh authorization.
			items = append(items, prior)
			continue
		}
		if legacy {
			return nil, ErrResourceBindingInvalid
		}
		revision, err := hashRevision(struct {
			Type             resourcebindings.BindingType `json:"type"`
			ResourceID       string                       `json:"resource_id"`
			ParentResourceID string                       `json:"parent_resource_id"`
			AccessMode       resourcebindings.AccessMode  `json:"access_mode"`
			DisplayName      string                       `json:"display_name"`
			ResourceRevision string                       `json:"resource_revision"`
			CredentialRef    string                       `json:"credential_ref,omitempty"`
			Metadata         json.RawMessage              `json:"metadata"`
		}{match.Type, match.ResourceID, match.ParentResourceID, match.AccessMode, input.DisplayName, input.ResourceRevision, input.CredentialRef, json.RawMessage(metadata)})
		if err != nil {
			return nil, err
		}
		row := &ResourceBinding{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, DigitalHumanVersionID: draftID,
			BindingType: match.Type, ResourceID: match.ResourceID, ParentResourceID: match.ParentResourceID,
			DisplayName: input.DisplayName, AccessMode: match.AccessMode, ResourceRevision: input.ResourceRevision,
			BindingRevision: revision, CredentialRef: input.CredentialRef, Metadata: metadata,
			AuthorizedBy: &actorID, AuthorizedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		if prior != nil {
			row.ID, row.CreatedAt = prior.ID, prior.CreatedAt
		}
		items = append(items, row)
	}
	return items, nil
}

func equivalentResourceMetadata(left, right []byte) bool {
	var a, b any
	l, r := json.NewDecoder(bytes.NewReader(left)), json.NewDecoder(bytes.NewReader(right))
	l.UseNumber()
	r.UseNumber()
	return l.Decode(&a) == nil && r.Decode(&b) == nil && equivalentResourceJSONValue(a, b)
}

func equivalentResourceJSONValue(left, right any) bool {
	switch a := left.(type) {
	case json.Number:
		b, ok := right.(json.Number)
		if !ok {
			return false
		}
		x, validX := new(big.Rat).SetString(string(a))
		y, validY := new(big.Rat).SetString(string(b))
		return validX && validY && x.Cmp(y) == 0
	case map[string]any:
		b, ok := right.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, value := range a {
			other, found := b[key]
			if !found || !equivalentResourceJSONValue(value, other) {
				return false
			}
		}
		return true
	case []any:
		b, ok := right.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !equivalentResourceJSONValue(a[i], b[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}

func sameResourceBindingContent(left, right *ResourceBinding) bool {
	return left.BindingType == right.BindingType && left.ResourceID == right.ResourceID && left.ParentResourceID == right.ParentResourceID && left.AccessMode == right.AccessMode && left.DisplayName == right.DisplayName && left.ResourceRevision == right.ResourceRevision && left.CredentialRef == right.CredentialRef && equivalentResourceMetadata(left.Metadata, right.Metadata)
}

func validBindingMode(bindingType resourcebindings.BindingType, mode resourcebindings.AccessMode) bool {
	switch bindingType {
	case resourcebindings.BindingTypeSkill, resourcebindings.BindingTypeWorkflow, resourcebindings.BindingTypeConnector:
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
