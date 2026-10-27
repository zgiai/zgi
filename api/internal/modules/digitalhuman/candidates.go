package digitalhuman

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const defaultResourceCandidateLimit = 50

// ResourceCandidateQuery filters resources that can be pinned to a draft.
type ResourceCandidateQuery struct {
	Type             resourcebindings.BindingType
	ParentResourceID string
	Search           string
	Limit            int
}

// ResourceCandidate is a safe, subject-neutral projection of a bindable resource.
type ResourceCandidate struct {
	Type             resourcebindings.BindingType  `json:"type"`
	ResourceID       string                        `json:"resource_id"`
	ParentResourceID string                        `json:"parent_resource_id,omitempty"`
	DisplayName      string                        `json:"display_name"`
	Description      string                        `json:"description,omitempty"`
	ResourceRevision string                        `json:"resource_revision"`
	AccessModes      []resourcebindings.AccessMode `json:"access_modes"`
	Metadata         map[string]any                `json:"metadata"`
	Status           string                        `json:"status"`
	Available        bool                          `json:"available"`
}

// ResourceCandidateList is the bounded candidate result returned to the console.
type ResourceCandidateList struct {
	Items []ResourceCandidate `json:"items"`
	Total int                 `json:"total"`
}

type resourceCandidateStore interface {
	ListResourceCandidates(context.Context, Scope, uuid.UUID, ResourceCandidateQuery) ([]ResourceCandidate, error)
	// ListConnectorCandidates is the pre-creation projection: the same
	// owner-scoped Connector choices without requiring a draft id.
	ListConnectorCandidates(context.Context, Scope) ([]ResourceCandidate, error)
}

type skillCandidateCatalog interface {
	ListSkills(context.Context, chatruntime.Scope) ([]skills.SkillDiscoveryMetadata, error)
}

// ListResourceCandidates returns safe, version-pinned choices for the draft binding editor.
func (s *RuntimeService) ListResourceCandidates(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ResourceCandidateQuery) (*ResourceCandidateList, error) {
	query, err := normalizeResourceCandidateQuery(query)
	if err != nil || !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.bindings.candidates")
	}
	store, ok := s.store.(resourceCandidateStore)
	if !ok {
		return nil, applicationError(ErrResourceBindingInvalid, "digital_human.bindings.candidates")
	}

	var candidates []ResourceCandidate
	if query.Type == resourcebindings.BindingTypeSkill {
		if _, err := store.ListResourceCandidates(ctx, scope, digitalHumanID, query); err != nil {
			return nil, applicationError(err, "digital_human.bindings.candidates")
		}
		candidates, err = s.skillCandidates(ctx, scope)
		if err != nil {
			return nil, applicationError(err, "digital_human.bindings.candidates")
		}
	} else {
		candidates, err = store.ListResourceCandidates(ctx, scope, digitalHumanID, query)
		if err != nil {
			return nil, applicationError(err, "digital_human.bindings.candidates")
		}
	}

	return finalizeResourceCandidates(candidates, query), nil
}

// finalizeResourceCandidates applies the shared search filter, stable ordering,
// and bounded limit used by every candidate surface.
func finalizeResourceCandidates(candidates []ResourceCandidate, query ResourceCandidateQuery) *ResourceCandidateList {
	filtered := make([]ResourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if resourceCandidateMatches(candidate, query.Search) {
			filtered = append(filtered, candidate)
		}
	}
	sortResourceCandidates(filtered)
	total := len(filtered)
	if total > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return &ResourceCandidateList{Items: filtered, Total: total}
}

// ListSkillCandidates returns the skills a brand new digital human may bind.
//
// Creation happens before the draft exists, so this mirrors
// ListResourceCandidates for the skill type without requiring an id. Both paths
// call skillCandidates, so the creation flow can never offer a skill the draft
// editor would hide.
func (s *RuntimeService) ListSkillCandidates(ctx context.Context, scope Scope) (*ResourceCandidateList, error) {
	if !validScope(scope) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.bindings.candidates")
	}
	candidates, err := s.skillCandidates(ctx, scope)
	if err != nil {
		return nil, applicationError(err, "digital_human.bindings.candidates")
	}
	sortResourceCandidates(candidates)
	return &ResourceCandidateList{Items: candidates, Total: len(candidates)}, nil
}

// ListConnectorCandidates returns the owner-scoped Connector connections a
// brand new digital human may attach.
//
// Creation happens before the draft exists, so this reuses the draft editor's
// repository projection (Digital Human surface, readiness, and operation
// constraints) without requiring an id, then applies the same bounded filter,
// ordering, and limit as every other candidate surface. The binding type is
// forced to Connector server-side: callers may narrow and page the connector
// projection, but can never switch it to another resource type.
func (s *RuntimeService) ListConnectorCandidates(ctx context.Context, scope Scope, query ResourceCandidateQuery) (*ResourceCandidateList, error) {
	query.Type = resourcebindings.BindingTypeConnector
	query, err := normalizeResourceCandidateQuery(query)
	if err != nil || !validScope(scope) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.bindings.candidates")
	}
	store, ok := s.store.(resourceCandidateStore)
	if !ok {
		return nil, applicationError(ErrResourceBindingInvalid, "digital_human.bindings.candidates")
	}
	candidates, err := store.ListConnectorCandidates(ctx, scope)
	if err != nil {
		return nil, applicationError(err, "digital_human.bindings.candidates")
	}
	return finalizeResourceCandidates(candidates, query), nil
}

// skillCandidates projects the workspace's real skills through the exposure and
// caller gate shared by every digital human binding surface.
func (s *RuntimeService) skillCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	catalog, ok := s.chat.(skillCandidateCatalog)
	if !ok {
		return nil, ErrResourceBindingInvalid
	}
	workspaceID := scope.WorkspaceID
	metadata, err := catalog.ListSkills(ctx, chatruntime.Scope{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    &workspaceID,
		AccountID:      scope.ActorID,
	})
	if err != nil {
		return nil, err
	}
	return skillResourceCandidates(metadata)
}

// sortResourceCandidates puts bindable candidates first, then orders by name.
func sortResourceCandidates(candidates []ResourceCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Available != candidates[j].Available {
			return candidates[i].Available
		}
		left, right := strings.ToLower(candidates[i].DisplayName), strings.ToLower(candidates[j].DisplayName)
		if left != right {
			return left < right
		}
		return candidates[i].ResourceID < candidates[j].ResourceID
	})
}

func normalizeResourceCandidateQuery(query ResourceCandidateQuery) (ResourceCandidateQuery, error) {
	query.ParentResourceID = strings.ToLower(strings.TrimSpace(query.ParentResourceID))
	query.Search = strings.TrimSpace(query.Search)
	if query.Limit <= 0 {
		query.Limit = defaultResourceCandidateLimit
	}
	maximum := maxPageSize
	if query.Type == resourcebindings.BindingTypeConnector {
		maximum = maxResourceBindings
	}
	if query.Limit > maximum {
		query.Limit = maximum
	}
	switch query.Type {
	case resourcebindings.BindingTypeSkill,
		resourcebindings.BindingTypeKnowledgeDataset,
		resourcebindings.BindingTypeDatabase,
		resourcebindings.BindingTypeDatabaseTable,
		resourcebindings.BindingTypeWorkflow,
		resourcebindings.BindingTypeConnector:
	default:
		return ResourceCandidateQuery{}, ErrInvalidRequest
	}
	if query.Type == resourcebindings.BindingTypeDatabaseTable && query.ParentResourceID == "" {
		return ResourceCandidateQuery{}, ErrInvalidRequest
	}
	return query, nil
}

func skillResourceCandidates(items []skills.SkillDiscoveryMetadata) ([]ResourceCandidate, error) {
	forbidden := map[string]struct{}{
		skills.SkillAgentManagement: {},
		skills.SkillAgentMemory:     {},
	}
	candidates := make([]ResourceCandidate, 0, len(items))
	for _, item := range items {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		profile := skills.SkillExposureForMetadata(item)
		_, denied := forbidden[id]
		if id == "" || denied || !item.Enabled || item.Status == skills.SkillStatusInvalid ||
			!profile.UserSelectable || profile.RuntimeManaged || profile.SystemAsset ||
			!skills.SkillSupportsCaller(item.SupportedCallers, skills.SkillCallerDigitalHuman) {
			continue
		}
		revision, err := hashRevision(struct {
			ID               string   `json:"id"`
			Source           string   `json:"source"`
			RuntimeType      string   `json:"runtime_type"`
			RequiredConfig   []string `json:"required_config"`
			SupportedCallers []string `json:"supported_callers"`
		}{id, item.Source, item.RuntimeType, item.RequiredConfig, item.SupportedCallers})
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = id
		}
		candidates = append(candidates, ResourceCandidate{
			Type: resourcebindings.BindingTypeSkill, ResourceID: id, DisplayName: name,
			Description: strings.TrimSpace(item.Description), ResourceRevision: revision,
			AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeExecute},
			Metadata: map[string]any{
				"source":            item.Source,
				"runtime_type":      item.RuntimeType,
				"when_to_use":       strings.TrimSpace(item.WhenToUse),
				"display":           item.Display,
				"has_tools":         item.HasTools,
				"has_references":    item.HasReferences,
				"has_scripts":       item.HasScripts,
				"scripts_supported": item.ScriptsSupported,
			},
			Status: strings.TrimSpace(item.Status), Available: true,
		})
	}
	return candidates, nil
}

func resourceCandidateMatches(candidate ResourceCandidate, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	values := []string{candidate.DisplayName, candidate.Description, candidate.ResourceID, candidate.ParentResourceID}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}
