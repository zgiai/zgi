package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"gorm.io/datatypes"
)

const (
	maxRoleItems        = 128
	maxRoleItemRunes    = 2_000
	maxRoleReferences   = 256
	maxResourceBindings = 512
)

var _ resourcebindings.Reader = (*GormRepository)(nil)

// CreateRoleTemplate creates a reusable job definition and its mutable draft.
func (s *Service) CreateRoleTemplate(ctx context.Context, scope Scope, request CreateRoleTemplateRequest) (*RoleTemplateDetail, error) {
	request, err := normalizeCreateRoleTemplateRequest(scope, request)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.create")
	}
	exists, err := s.repo.RoleTemplateNameExists(ctx, scope, request.Name, uuid.Nil)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.create")
	}
	if exists {
		return nil, applicationError(ErrRoleNameConflict, "digital_human.role_template.create")
	}
	version, err := newRoleTemplateDraft(scope, uuid.New(), request, s.now())
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.create")
	}
	templateID := version.RoleTemplateID
	actorID := scope.ActorID
	template := &RoleTemplate{
		ID: templateID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Name: request.Name, Description: request.Description, Revision: 1,
		CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: version.CreatedAt, UpdatedAt: version.UpdatedAt,
	}
	err = s.repo.Transaction(ctx, func(tx *GormRepository) error {
		if err := tx.CreateRoleTemplate(ctx, template); err != nil {
			return err
		}
		if err := tx.CreateRoleTemplateVersion(ctx, version); err != nil {
			return err
		}
		if err := tx.AttachInitialRoleDraft(ctx, scope, template.ID, version.ID); err != nil {
			return err
		}
		draftID := version.ID
		template.DraftVersionID = &draftID
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.create")
	}
	return &RoleTemplateDetail{Template: template, Draft: version}, nil
}

func (s *Service) GetRoleTemplate(ctx context.Context, scope Scope, id uuid.UUID) (*RoleTemplateDetail, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.get")
	}
	detail, err := s.repo.RoleTemplateDetail(ctx, scope, id)
	return detail, applicationError(err, "digital_human.role_template.get")
}

func (s *Service) ListRoleTemplates(ctx context.Context, scope Scope, query ListQuery) (*RoleTemplateListResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.list")
	}
	items, total, err := s.repo.ListRoleTemplates(ctx, scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.list")
	}
	return &RoleTemplateListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: int64(query.Page*query.PageSize) < total}, nil
}

func (s *Service) UpdateRoleTemplate(ctx context.Context, scope Scope, id uuid.UUID, request UpdateRoleTemplateRequest) (*RoleTemplateDetail, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedRevision < 1 || (request.Name == nil && request.Description == nil) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.update")
	}
	updates := make(map[string]any)
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
			return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.update")
		}
		exists, err := s.repo.RoleTemplateNameExists(ctx, scope, name, id)
		if err != nil {
			return nil, applicationError(err, "digital_human.role_template.update")
		}
		if exists {
			return nil, applicationError(ErrRoleNameConflict, "digital_human.role_template.update")
		}
		updates["name"] = name
	}
	if request.Description != nil {
		description := strings.TrimSpace(*request.Description)
		if utf8.RuneCountInString(description) > maxDescriptionRunes {
			return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.update")
		}
		updates["description"] = description
	}
	if err := s.repo.UpdateRoleTemplateCAS(ctx, scope, id, request.ExpectedRevision, updates); err != nil {
		return nil, applicationError(err, "digital_human.role_template.update")
	}
	return s.GetRoleTemplate(ctx, scope, id)
}

func (s *Service) UpdateRoleTemplateDraft(ctx context.Context, scope Scope, id uuid.UUID, request UpdateRoleTemplateDraftRequest) (*RoleTemplateVersion, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedLockVersion < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.draft.update")
	}
	template, err := s.repo.RoleTemplate(ctx, scope, id, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.draft.update")
	}
	draft, err := s.repo.RoleTemplateDraft(ctx, scope, template, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.draft.update")
	}
	if draft.LockVersion != request.ExpectedLockVersion {
		return nil, applicationError(ErrRevisionConflict, "digital_human.role_template.draft.update")
	}
	if err := applyRoleTemplateDraftUpdate(draft, request); err != nil {
		return nil, applicationError(err, "digital_human.role_template.draft.update")
	}
	draft.Revision, err = roleTemplateRevision(draft)
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.draft.update")
	}
	draft.UpdatedAt = s.now()
	if err := s.repo.UpdateRoleTemplateDraftCAS(ctx, scope, draft, request.ExpectedLockVersion); err != nil {
		return nil, applicationError(err, "digital_human.role_template.draft.update")
	}
	return draft, nil
}

func (s *Service) PublishRoleTemplate(ctx context.Context, scope Scope, id uuid.UUID, request PublishRoleTemplateRequest) (*RoleTemplateDetail, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedRevision < 1 || request.ExpectedDraftLockVersion < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.role_template.publish")
	}
	var result RoleTemplateDetail
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		template, err := tx.RoleTemplate(ctx, scope, id, true)
		if err != nil {
			return err
		}
		if template.Revision != request.ExpectedRevision {
			return ErrRevisionConflict
		}
		draft, err := tx.RoleTemplateDraft(ctx, scope, template, true)
		if err != nil {
			return err
		}
		if draft.LockVersion != request.ExpectedDraftLockVersion {
			return ErrRevisionConflict
		}
		if !roleTemplateDraftPublishable(draft) {
			return ErrRolePublishInvalid
		}
		draft.Revision, err = roleTemplateRevision(draft)
		if err != nil {
			return err
		}
		draft.VersionNumber, err = tx.NextPublishedRoleTemplateVersion(ctx, template.ID)
		if err != nil {
			return err
		}
		now := s.now()
		actorID := scope.ActorID
		draft.State, draft.PublishedBy, draft.PublishedAt = VersionStatePublished, &actorID, &now
		draft.UpdatedBy, draft.UpdatedAt = &actorID, now
		if err := tx.PromoteRoleTemplateDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}
		next := &RoleTemplateVersion{
			ID: uuid.New(), RoleTemplateID: template.ID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			State: VersionStateDraft, Goals: jsonClone(draft.Goals), Responsibilities: jsonClone(draft.Responsibilities),
			Capabilities: jsonClone(draft.Capabilities), SOPRefs: jsonClone(draft.SOPRefs), KnowledgeRefs: jsonClone(draft.KnowledgeRefs),
			Policy: jsonClone(draft.Policy), EvaluationRefs: jsonClone(draft.EvaluationRefs), LockVersion: 1,
			CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
		}
		next.Revision, err = roleTemplateRevision(next)
		if err != nil {
			return err
		}
		if err := tx.CreateRoleTemplateVersion(ctx, next); err != nil {
			return err
		}
		draftID, publishedID := next.ID, draft.ID
		template.DraftVersionID, template.PublishedVersionID = &draftID, &publishedID
		template.UpdatedAt = now
		if err := tx.AdvanceRoleTemplatePublicationCAS(ctx, scope, template, request.ExpectedRevision); err != nil {
			return err
		}
		result = RoleTemplateDetail{Template: template, Draft: next, Published: draft}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.role_template.publish")
	}
	return &result, nil
}

func (s *Service) ListDraftRoleAssignments(ctx context.Context, scope Scope, id uuid.UUID) (*DraftRoleAssignments, error) {
	definition, draft, err := s.loadDigitalHumanDraft(ctx, scope, id, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.roles.list")
	}
	items, err := s.repo.ListRoleAssignments(ctx, scope, definition.ID, draft.ID)
	if err != nil {
		return nil, applicationError(err, "digital_human.roles.list")
	}
	return &DraftRoleAssignments{DraftLockVersion: draft.LockVersion, Items: items}, nil
}

func (s *Service) ReplaceDraftRoleAssignments(ctx context.Context, scope Scope, id uuid.UUID, request ReplaceRoleAssignmentsRequest) (*DraftRoleAssignments, error) {
	if request.ExpectedDraftLockVersion < 1 || len(request.Items) > maxRoleItems {
		return nil, applicationError(ErrInvalidRequest, "digital_human.roles.replace")
	}
	var result DraftRoleAssignments
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
		items, err := buildRoleAssignments(ctx, tx, scope, definition.ID, draft.ID, request.Items, s.now())
		if err != nil {
			return err
		}
		if err := tx.ReplaceDraftRoleAssignments(ctx, scope, definition.ID, draft.ID, items); err != nil {
			return err
		}
		draft.UpdatedAt = s.now()
		if err := tx.UpdateDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}
		result = DraftRoleAssignments{DraftLockVersion: draft.LockVersion, Items: items}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.roles.replace")
	}
	return &result, nil
}

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

func normalizeCreateRoleTemplateRequest(scope Scope, request CreateRoleTemplateRequest) (CreateRoleTemplateRequest, error) {
	request.Name, request.Description = strings.TrimSpace(request.Name), strings.TrimSpace(request.Description)
	if !validScope(scope) || request.Name == "" || utf8.RuneCountInString(request.Name) > maxNameRunes || utf8.RuneCountInString(request.Description) > maxDescriptionRunes {
		return CreateRoleTemplateRequest{}, ErrInvalidRequest
	}
	if err := validateStringList(request.Goals); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	if err := validateStringList(request.Responsibilities); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	var err error
	if request.Capabilities, err = normalizeRoleRefs(request.Capabilities, "capability"); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	if request.SOPRefs, err = normalizeRoleRefs(request.SOPRefs, "sop"); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	if request.KnowledgeRefs, err = normalizeRoleRefs(request.KnowledgeRefs, "knowledge"); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	if request.EvaluationRefs, err = normalizeRoleRefs(request.EvaluationRefs, "evaluation"); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	if _, err := safeJSON(request.Policy); err != nil {
		return CreateRoleTemplateRequest{}, err
	}
	return request, nil
}

func newRoleTemplateDraft(scope Scope, templateID uuid.UUID, request CreateRoleTemplateRequest, now time.Time) (*RoleTemplateVersion, error) {
	goals, err := safeJSON(request.Goals)
	if err != nil {
		return nil, err
	}
	responsibilities, _ := safeJSON(request.Responsibilities)
	capabilities, _ := safeJSON(request.Capabilities)
	sopRefs, _ := safeJSON(request.SOPRefs)
	knowledgeRefs, _ := safeJSON(request.KnowledgeRefs)
	policy, _ := safeJSON(request.Policy)
	evaluationRefs, _ := safeJSON(request.EvaluationRefs)
	actorID := scope.ActorID
	version := &RoleTemplateVersion{
		ID: uuid.New(), RoleTemplateID: templateID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		State: VersionStateDraft, Goals: goals, Responsibilities: responsibilities, Capabilities: capabilities,
		SOPRefs: sopRefs, KnowledgeRefs: knowledgeRefs, Policy: policy, EvaluationRefs: evaluationRefs, LockVersion: 1,
		CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	}
	version.Revision, err = roleTemplateRevision(version)
	return version, err
}

func applyRoleTemplateDraftUpdate(draft *RoleTemplateVersion, request UpdateRoleTemplateDraftRequest) error {
	if request.Goals != nil {
		if err := validateStringList(*request.Goals); err != nil {
			return err
		}
		draft.Goals, _ = safeJSON(*request.Goals)
	}
	if request.Responsibilities != nil {
		if err := validateStringList(*request.Responsibilities); err != nil {
			return err
		}
		draft.Responsibilities, _ = safeJSON(*request.Responsibilities)
	}
	updates := []struct {
		value *[]runspec.VersionRef
		kind  string
		set   func(datatypes.JSON)
	}{
		{request.Capabilities, "capability", func(value datatypes.JSON) { draft.Capabilities = value }},
		{request.SOPRefs, "sop", func(value datatypes.JSON) { draft.SOPRefs = value }},
		{request.KnowledgeRefs, "knowledge", func(value datatypes.JSON) { draft.KnowledgeRefs = value }},
		{request.EvaluationRefs, "evaluation", func(value datatypes.JSON) { draft.EvaluationRefs = value }},
	}
	for _, update := range updates {
		if update.value == nil {
			continue
		}
		refs, err := normalizeRoleRefs(*update.value, update.kind)
		if err != nil {
			return err
		}
		encoded, _ := safeJSON(refs)
		update.set(encoded)
	}
	if request.Policy != nil {
		policy, err := safeJSON(*request.Policy)
		if err != nil {
			return err
		}
		draft.Policy = policy
	}
	return nil
}

func normalizeRoleRefs(refs []runspec.VersionRef, kind string) ([]runspec.VersionRef, error) {
	if len(refs) > maxRoleReferences {
		return nil, ErrInvalidRequest
	}
	normalized := append([]runspec.VersionRef(nil), refs...)
	seen := make(map[string]struct{}, len(normalized))
	for i := range normalized {
		ref := &normalized[i]
		ref.Kind, ref.ID, ref.Version, ref.Revision = strings.ToLower(strings.TrimSpace(ref.Kind)), strings.TrimSpace(ref.ID), strings.TrimSpace(ref.Version), strings.TrimSpace(ref.Revision)
		if ref.Kind != kind || ref.ID == "" || (ref.Version == "" && ref.Revision == "") {
			return nil, ErrInvalidRequest
		}
		key := strings.Join([]string{ref.Kind, ref.ID, ref.Version, ref.Revision}, "\x00")
		if _, ok := seen[key]; ok {
			return nil, ErrInvalidRequest
		}
		seen[key] = struct{}{}
	}
	slices.SortFunc(normalized, func(a, b runspec.VersionRef) int {
		return strings.Compare(strings.Join([]string{a.Kind, a.ID, a.Version, a.Revision}, "\x00"), strings.Join([]string{b.Kind, b.ID, b.Version, b.Revision}, "\x00"))
	})
	return normalized, nil
}

func validateStringList(values []string) error {
	if len(values) > maxRoleItems {
		return ErrInvalidRequest
	}
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
		if values[i] == "" || utf8.RuneCountInString(values[i]) > maxRoleItemRunes {
			return ErrInvalidRequest
		}
	}
	return nil
}

func roleTemplateRevision(version *RoleTemplateVersion) (string, error) {
	return hashRevision(struct {
		Goals, Responsibilities, Capabilities, SOPRefs, KnowledgeRefs, Policy, EvaluationRefs json.RawMessage
	}{
		Goals: json.RawMessage(version.Goals), Responsibilities: json.RawMessage(version.Responsibilities), Capabilities: json.RawMessage(version.Capabilities),
		SOPRefs: json.RawMessage(version.SOPRefs), KnowledgeRefs: json.RawMessage(version.KnowledgeRefs), Policy: json.RawMessage(version.Policy), EvaluationRefs: json.RawMessage(version.EvaluationRefs),
	})
}

func roleTemplateDraftPublishable(draft *RoleTemplateVersion) bool {
	var goals, responsibilities []string
	return json.Unmarshal(draft.Goals, &goals) == nil && len(goals) > 0 &&
		json.Unmarshal(draft.Responsibilities, &responsibilities) == nil && len(responsibilities) > 0
}

func buildRoleAssignments(ctx context.Context, repo *GormRepository, scope Scope, digitalHumanID, draftID uuid.UUID, inputs []RoleAssignmentInput, now time.Time) ([]*RoleAssignment, error) {
	items := make([]*RoleAssignment, 0, len(inputs))
	seen := make(map[uuid.UUID]struct{}, len(inputs))
	actorID := scope.ActorID
	for _, input := range inputs {
		if input.RoleTemplateVersionID == uuid.Nil || (input.EffectiveFrom != nil && input.EffectiveTo != nil && !input.EffectiveTo.After(*input.EffectiveFrom)) {
			return nil, ErrInvalidRequest
		}
		if _, ok := seen[input.RoleTemplateVersionID]; ok {
			return nil, ErrInvalidRequest
		}
		seen[input.RoleTemplateVersionID] = struct{}{}
		roleVersion, err := repo.PublishedRoleTemplateVersion(ctx, scope, input.RoleTemplateVersionID)
		if err != nil {
			return nil, err
		}
		parameters, err := safeJSON(input.Parameters)
		if err != nil {
			return nil, err
		}
		overridePolicy, err := safeJSON(input.OverridePolicy)
		if err != nil {
			return nil, err
		}
		revision, err := hashRevision(struct {
			RoleTemplateVersionID uuid.UUID       `json:"role_template_version_id"`
			RoleRevision          string          `json:"role_revision"`
			Parameters            json.RawMessage `json:"parameters"`
			OverridePolicy        json.RawMessage `json:"override_policy"`
			EffectiveFrom         *time.Time      `json:"effective_from,omitempty"`
			EffectiveTo           *time.Time      `json:"effective_to,omitempty"`
		}{input.RoleTemplateVersionID, roleVersion.Revision, json.RawMessage(parameters), json.RawMessage(overridePolicy), input.EffectiveFrom, input.EffectiveTo})
		if err != nil {
			return nil, err
		}
		items = append(items, &RoleAssignment{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, DigitalHumanVersionID: draftID,
			RoleTemplateID: roleVersion.RoleTemplateID, RoleTemplateVersionID: roleVersion.ID,
			Parameters: parameters, OverridePolicy: overridePolicy, EffectiveFrom: input.EffectiveFrom, EffectiveTo: input.EffectiveTo,
			Revision: revision, CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
		})
	}
	return items, nil
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

func roleTemplateError(err error) bool {
	return errors.Is(err, ErrRoleTemplateNotFound) || errors.Is(err, ErrRoleVersionNotFound)
}

func formatRoleTemplateError(operation string, err error) error {
	if roleTemplateError(err) {
		return applicationError(err, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
