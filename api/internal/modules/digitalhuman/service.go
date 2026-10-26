package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/datatypes"
)

const (
	defaultPageSize            = 20
	maxPageSize                = 100
	maxNameRunes               = 80
	maxAvatarRunes             = 2_048
	maxIconRunes               = 4_096
	maxDescriptionRunes        = 4_000
	maxPersonaRunes            = 16_000
	maxGoals                   = 64
	maxGoalRunes               = 1_000
	maxRules                   = 128
	maxRuleRunes               = 2_000
	maxDefaultModelRunes       = 255
	maxPolicyBytes             = 64 * 1024
	maxPresentationTitleRunes  = 120
	maxOpeningStatementRunes   = 2_000
	maxInputPlaceholderRunes   = 160
	maxSuggestedQuestions      = 6
	maxSuggestedQuestionRunes  = 240
	maxVersionNameRunes        = 80
	maxVersionDescriptionRunes = 500
	versionRevisionPrefix      = "sha256:"
)

// Service owns DigitalHuman lifecycle and publication rules.
type Service struct {
	repo         *GormRepository
	iconResolver interface {
		GetFileURL(context.Context, string) (string, error)
	}
	projectionRepairer DigitalHumanMemoryProjectionRepairer
	// previewExtractor runs the memory preview; it is optional.
	previewExtractor MemoryExtractor
	now              func() time.Time
}

// SetConnectorCatalog lets draft saves and publication admit Connector
// bindings; without it every Connector selection is rejected.
func (s *Service) SetConnectorCatalog(connectorCatalog ConnectorCatalog) {
	if s != nil && s.repo != nil {
		s.repo.SetConnectorCatalog(connectorCatalog)
	}
}

// SetIconResolver enables Agent-compatible image icon URLs without coupling the domain to file storage.
func (s *Service) SetIconResolver(resolver interface {
	GetFileURL(context.Context, string) (string, error)
}) {
	s.iconResolver = resolver
}

// SetMemoryProjectionRepairer enables the operator-facing long-term memory
// repair. It stays optional so a deployment without a vector store can still
// serve the rest of the DigitalHuman surface.
func (s *Service) SetMemoryProjectionRepairer(repairer DigitalHumanMemoryProjectionRepairer) {
	s.projectionRepairer = repairer
}

// NewService constructs a DigitalHuman lifecycle service.
func NewService(repo *GormRepository) *Service {
	if repo == nil {
		panic("digital human service requires repository")
	}
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// Create creates aggregate metadata and its initial draft atomically.
func (s *Service) Create(ctx context.Context, scope Scope, request CreateRequest) (*Detail, error) {
	request, err := normalizeCreateRequest(scope, request)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	goals, err := marshalStringList(request.Goals)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	rules, err := marshalRules(request.Rules)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	if err := validateMemoryPolicy(request.MemoryPolicy); err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	memoryPolicy, err := marshalPolicy(request.MemoryPolicy)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	triggerPolicy, err := marshalPolicy(request.TriggerPolicy)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	presentationPolicy, err := marshalPresentationPolicy(request.PresentationPolicy)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	now := s.now()
	definitionID := uuid.New()
	draftID := uuid.New()
	actorID := scope.ActorID
	definition := &Definition{
		ID: definitionID, WebAppID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Name: request.Name, Avatar: request.Avatar, IconType: request.IconType, Icon: request.Icon, Description: request.Description,
		Status: StatusDraft, AutonomyLevel: request.AutonomyLevel, Visibility: request.Visibility,
		Revision: 1, CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	}
	// New digital humans start with uploads off, like Agents; one created
	// before this keeps its saved policy.
	uploadPolicy := chatfile.Clone(request.FileUploadPolicy)
	if uploadPolicy == nil {
		uploadPolicy = &chatfile.Policy{AllowedFileTypes: []string{}}
	}
	uploadPolicy.Normalize()
	draft := &Version{
		FileUploadPolicy: uploadPolicy,
		ID:               draftID, DigitalHumanID: definitionID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		State: VersionStateDraft, IdentityName: request.IdentityName, Persona: request.Persona, Goals: goals, Rules: rules, DefaultModel: request.DefaultModel,
		MemoryPolicy: memoryPolicy, TriggerPolicy: triggerPolicy, PresentationPolicy: presentationPolicy, IdentitySnapshot: datatypes.JSON([]byte(`{}`)),
		ExecutionRuntime: cloneExecutionRuntime(request.ExecutionRuntime),
		LockVersion:      1, CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	}
	if draft.FileUploadPolicy.Validate() != nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.create")
	}
	draft.Revision, err = configRevision(draft)
	if err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	if err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		if err := tx.CreateDefinition(ctx, definition); err != nil {
			return err
		}
		if err := tx.CreateVersion(ctx, draft); err != nil {
			return err
		}
		return tx.AttachInitialDraft(ctx, scope, definitionID, draftID)
	}); err != nil {
		return nil, applicationError(err, "digital_human.create")
	}
	definition.DraftVersionID = &draftID
	s.resolveDefinitionIcon(ctx, definition)
	return &Detail{Definition: definition, Draft: draft, HasUnpublishedChanges: true}, nil
}

// Get returns scoped aggregate metadata with draft and current publication.
func (s *Service) Get(ctx context.Context, scope Scope, id uuid.UUID) (*Detail, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.get")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.get")
	}
	s.resolveDefinitionIcon(ctx, definition)
	draft, err := s.repo.Draft(ctx, scope, definition, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.get")
	}
	published, err := s.repo.Published(ctx, scope, definition)
	if err != nil {
		return nil, applicationError(err, "digital_human.get")
	}
	detail := &Detail{Definition: definition, Draft: draft, Published: published}
	if detail.HasUnpublishedChanges, err = s.hasUnpublishedChanges(ctx, s.repo, scope, definition.ID, draft, published); err != nil {
		return nil, applicationError(err, "digital_human.get")
	}
	return detail, nil
}

// hasUnpublishedChanges reports whether publishing would change what runs:
// either the draft configuration or its resource bindings differ from the
// current publication.
func (s *Service) hasUnpublishedChanges(ctx context.Context, repo *GormRepository, scope Scope, digitalHumanID uuid.UUID, draft, published *Version) (bool, error) {
	if draft == nil || published == nil || draft.Revision != published.Revision {
		return true, nil
	}
	draftBindings, err := repo.ListResourceBindingRows(ctx, scope, digitalHumanID, draft.ID)
	if err != nil {
		return false, err
	}
	publishedBindings, err := repo.ListResourceBindingRows(ctx, scope, digitalHumanID, published.ID)
	if err != nil {
		return false, err
	}
	if len(versionBindingChanges(publishedBindings, draftBindings)) > 0 {
		return true, nil
	}
	return bindingAuthorizersChanged(publishedBindings, draftBindings), nil
}

// bindingAuthorizersChanged reports a re-authorized binding. The binding
// revision hashes only its content, but runtime tools act with the published
// authorizer's permissions, so a new authorizer must be publishable.
func bindingAuthorizersChanged(published, draft []*ResourceBinding) bool {
	authorizers := make(map[string]string, len(published))
	for _, item := range published {
		authorizers[restoreBindingKey(item)] = bindingAuthorizer(item)
	}
	for _, item := range draft {
		if authorizer, ok := authorizers[restoreBindingKey(item)]; ok && authorizer != bindingAuthorizer(item) {
			return true
		}
	}
	return false
}

func bindingAuthorizer(item *ResourceBinding) string {
	if item == nil || item.AuthorizedBy == nil {
		return ""
	}
	return item.AuthorizedBy.String()
}

// List returns one workspace-scoped page.
func (s *Service) List(ctx context.Context, scope Scope, query ListQuery) (*ListResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.list")
	}
	items, total, err := s.repo.ListDefinitions(ctx, scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.list")
	}
	for _, item := range items {
		s.resolveDefinitionIcon(ctx, item)
	}
	return &ListResult{
		Items: items, Total: total, Page: query.Page, PageSize: query.PageSize,
		HasMore: int64(query.Page)*int64(query.PageSize) < total,
	}, nil
}

// Update updates aggregate metadata using the supplied revision.
func (s *Service) Update(ctx context.Context, scope Scope, id uuid.UUID, request UpdateRequest) (*Detail, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedRevision < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.update")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.update")
	}
	if definition.Revision != request.ExpectedRevision {
		return nil, applicationError(ErrRevisionConflict, "digital_human.update")
	}
	updates, err := normalizeDefinitionUpdates(definition, request)
	if err != nil {
		return nil, applicationError(err, "digital_human.update")
	}
	if err := s.repo.UpdateDefinitionCAS(ctx, scope, id, request.ExpectedRevision, updates); err != nil {
		return nil, applicationError(err, "digital_human.update")
	}
	return s.Get(ctx, scope, id)
}

// UpdateDraft updates only the mutable draft using its lock version.
func (s *Service) UpdateDraft(ctx context.Context, scope Scope, id uuid.UUID, request UpdateDraftRequest) (*Version, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedLockVersion < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.draft.update")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.draft.update")
	}
	draft, err := s.repo.Draft(ctx, scope, definition, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.draft.update")
	}
	if draft.LockVersion != request.ExpectedLockVersion {
		return nil, applicationError(ErrRevisionConflict, "digital_human.draft.update")
	}
	if err := applyDraftUpdate(draft, request); err != nil {
		return nil, applicationError(err, "digital_human.draft.update")
	}
	draft.IdentitySnapshot = datatypes.JSON([]byte(`{}`))
	draft.UpdatedAt = s.now()
	draft.Revision, err = configRevision(draft)
	if err != nil {
		return nil, applicationError(err, "digital_human.draft.update")
	}
	if err := s.repo.UpdateDraftCAS(ctx, scope, draft, request.ExpectedLockVersion); err != nil {
		return nil, applicationError(err, "digital_human.draft.update")
	}
	return draft, nil
}

// Publish atomically promotes the current draft and creates the next draft.
func (s *Service) Publish(ctx context.Context, scope Scope, id uuid.UUID, request PublishRequest) (*Detail, error) {
	if !validScope(scope) || id == uuid.Nil || request.ExpectedRevision < 1 || request.ExpectedDraftLockVersion < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.publish")
	}
	request, err := normalizePublishRequest(request)
	if err != nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.publish")
	}
	var result Detail
	err = s.repo.Transaction(ctx, func(tx *GormRepository) error {
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
		if err := validatePublishable(draft); err != nil {
			return err
		}
		identity, err := identitySnapshot(definition)
		if err != nil {
			return err
		}
		draft.IdentitySnapshot = identity
		draft.Revision, err = publishedRevision(draft)
		if err != nil {
			return err
		}
		number, err := tx.NextPublishedVersionNumber(ctx, definition.ID)
		if err != nil {
			return err
		}
		now := s.now()
		actorID := scope.ActorID
		draft.State = VersionStatePublished
		draft.VersionNumber = number
		draft.Name = request.Name
		draft.Description = request.Description
		draft.PublishedBy = &actorID
		draft.PublishedAt = &now
		draft.UpdatedBy = &actorID
		draft.UpdatedAt = now
		if err := tx.PromoteDraftCAS(ctx, scope, draft, request.ExpectedDraftLockVersion); err != nil {
			return err
		}

		nextDraftID := uuid.New()
		nextDraft := &Version{
			FileUploadPolicy: chatfile.Clone(draft.FileUploadPolicy),
			ID:               nextDraftID, DigitalHumanID: definition.ID, OrganizationID: definition.OrganizationID, WorkspaceID: definition.WorkspaceID,
			State: VersionStateDraft, IdentityName: draft.IdentityName, Persona: draft.Persona, Goals: cloneJSON(draft.Goals), Rules: cloneJSON(draft.Rules), DefaultModel: draft.DefaultModel,
			MemoryPolicy: cloneJSON(draft.MemoryPolicy), TriggerPolicy: cloneJSON(draft.TriggerPolicy), PresentationPolicy: cloneJSON(draft.PresentationPolicy),
			ExecutionRuntime: cloneExecutionRuntime(draft.ExecutionRuntime),
			IdentitySnapshot: datatypes.JSON([]byte(`{}`)), LockVersion: 1,
			CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
		}
		nextDraft.Revision, err = configRevision(nextDraft)
		if err != nil {
			return err
		}
		if err := tx.CreateVersion(ctx, nextDraft); err != nil {
			return err
		}
		if err := tx.CloneVersionResources(ctx, scope, definition.ID, draft.ID, nextDraft.ID, actorID, now); err != nil {
			return err
		}
		publishedID := draft.ID
		definition.DraftVersionID = &nextDraftID
		definition.PublishedVersionID = &publishedID
		definition.Status = StatusActive
		definition.UpdatedAt = now
		if err := tx.AdvancePublicationCAS(ctx, scope, definition, request.ExpectedRevision); err != nil {
			return err
		}
		// The next draft is a copy of the publication, so nothing is pending.
		result = Detail{Definition: definition, Draft: nextDraft, Published: draft}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.publish")
	}
	return &result, nil
}

func normalizePublishRequest(request PublishRequest) (PublishRequest, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if utf8.RuneCountInString(request.Name) > maxVersionNameRunes ||
		utf8.RuneCountInString(request.Description) > maxVersionDescriptionRunes {
		return PublishRequest{}, ErrInvalidRequest
	}
	return request, nil
}

// PublishedVersions returns immutable publication history.
func (s *Service) PublishedVersions(ctx context.Context, scope Scope, id uuid.UUID, query ListQuery) (*PublishedVersionsResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil || id == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.versions.list")
	}
	if _, err := s.repo.Definition(ctx, scope, id); err != nil {
		return nil, applicationError(err, "digital_human.versions.list")
	}
	items, total, err := s.repo.ListPublishedVersions(ctx, scope, id, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.versions.list")
	}
	return &PublishedVersionsResult{
		Items: items, Total: total, Page: query.Page, PageSize: query.PageSize,
		HasMore: int64(query.Page)*int64(query.PageSize) < total,
	}, nil
}

// DeleteImpact reports durable blockers without mutating the aggregate.
func (s *Service) DeleteImpact(ctx context.Context, scope Scope, id uuid.UUID) (*DeleteImpact, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.delete_impact")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.delete_impact")
	}
	count, err := s.repo.PublishedVersionCount(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.delete_impact")
	}
	blockers := make([]string, 0, 1)
	// Published versions are immutable, so an online digital human cannot be
	// removed. Taking it offline (status=disabled) is the operator step that
	// unlocks deletion.
	if count > 0 && definition.Status != StatusDisabled {
		blockers = append(blockers, "published_versions")
	}
	return &DeleteImpact{CanDelete: len(blockers) == 0, PublishedVersionCount: count, Blockers: blockers}, nil
}

// Delete soft-deletes an unpublished aggregate after a locked impact check.
func (s *Service) Delete(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64) error {
	if !validScope(scope) || id == uuid.Nil || expectedRevision < 1 {
		return applicationError(ErrInvalidRequest, "digital_human.delete")
	}
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		definition, err := tx.LockDefinition(ctx, scope, id)
		if err != nil {
			return err
		}
		if definition.Revision != expectedRevision {
			return ErrRevisionConflict
		}
		count, err := tx.PublishedVersionCount(ctx, scope, id)
		if err != nil {
			return err
		}
		// Published versions are immutable, so deletion requires taking the
		// digital human offline first.
		if count > 0 && definition.Status != StatusDisabled {
			return ErrDeleteBlocked
		}
		return tx.SoftDeleteCAS(ctx, scope, id, expectedRevision, s.now())
	})
	return applicationError(err, "digital_human.delete")
}

func normalizeCreateRequest(scope Scope, request CreateRequest) (CreateRequest, error) {
	if !validScope(scope) {
		return CreateRequest{}, ErrInvalidRequest
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Avatar = strings.TrimSpace(request.Avatar)
	request.IconType = strings.TrimSpace(request.IconType)
	request.Icon = strings.TrimSpace(request.Icon)
	request.Description = strings.TrimSpace(request.Description)
	request.IdentityName = strings.TrimSpace(request.IdentityName)
	request.Persona = strings.TrimSpace(request.Persona)
	request.DefaultModel = strings.TrimSpace(request.DefaultModel)
	if request.AutonomyLevel == "" {
		request.AutonomyLevel = AutonomySupervised
	}
	if request.Visibility == "" {
		request.Visibility = VisibilityWorkspace
	}
	if request.IdentityName == "" {
		request.IdentityName = request.Name
	}
	if request.IconType == "" {
		request.IconType = "text"
	}
	if request.Name == "" || !validRuneLength(request.Name, maxNameRunes) || !validRuneLength(request.Avatar, maxAvatarRunes) ||
		!validIconType(request.IconType) || !validRuneLength(request.Icon, maxIconRunes) ||
		!validRuneLength(request.Description, maxDescriptionRunes) || !validRuneLength(request.IdentityName, maxNameRunes) || !validRuneLength(request.Persona, maxPersonaRunes) ||
		!validRuneLength(request.DefaultModel, maxDefaultModelRunes) || !validAutonomy(request.AutonomyLevel) || !validVisibility(request.Visibility) {
		return CreateRequest{}, ErrInvalidRequest
	}
	request.Goals = normalizeStringList(request.Goals)
	if !validStringList(request.Goals, maxGoals, maxGoalRunes) {
		return CreateRequest{}, ErrInvalidRequest
	}
	request.Rules = normalizeRules(request.Rules)
	if len(request.Rules) > maxRules {
		return CreateRequest{}, ErrInvalidRequest
	}
	for _, rule := range request.Rules {
		if !validRuneLength(rule, maxRuleRunes) {
			return CreateRequest{}, ErrInvalidRequest
		}
	}
	return request, nil
}

func normalizeDefinitionUpdates(definition *Definition, request UpdateRequest) (map[string]any, error) {
	updates := make(map[string]any)
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		if value == "" || !validRuneLength(value, maxNameRunes) {
			return nil, ErrInvalidRequest
		}
		updates["name"] = value
	}
	if request.Avatar != nil {
		value := strings.TrimSpace(*request.Avatar)
		if !validRuneLength(value, maxAvatarRunes) {
			return nil, ErrInvalidRequest
		}
		updates["avatar"] = value
	}
	if request.IconType != nil {
		value := strings.TrimSpace(*request.IconType)
		if !validIconType(value) {
			return nil, ErrInvalidRequest
		}
		updates["icon_type"] = value
	}
	if request.Icon != nil {
		value := strings.TrimSpace(*request.Icon)
		if !validRuneLength(value, maxIconRunes) {
			return nil, ErrInvalidRequest
		}
		updates["icon"] = value
	}
	if request.Description != nil {
		value := strings.TrimSpace(*request.Description)
		if !validRuneLength(value, maxDescriptionRunes) {
			return nil, ErrInvalidRequest
		}
		updates["description"] = value
	}
	if request.AutonomyLevel != nil {
		if !validAutonomy(*request.AutonomyLevel) {
			return nil, ErrInvalidRequest
		}
		updates["autonomy_level"] = *request.AutonomyLevel
	}
	if request.Visibility != nil {
		if !validVisibility(*request.Visibility) {
			return nil, ErrInvalidRequest
		}
		updates["visibility"] = *request.Visibility
	}
	if request.Status != nil {
		if !validStatusTransition(definition, *request.Status) {
			return nil, ErrInvalidRequest
		}
		updates["status"] = *request.Status
	}
	if len(updates) == 0 {
		return nil, ErrInvalidRequest
	}
	return updates, nil
}

func applyDraftUpdate(draft *Version, request UpdateDraftRequest) error {
	changed := false
	if request.IdentityName != nil {
		value := strings.TrimSpace(*request.IdentityName)
		if value == "" || !validRuneLength(value, maxNameRunes) {
			return ErrInvalidRequest
		}
		draft.IdentityName = value
		changed = true
	}
	if request.Persona != nil {
		value := strings.TrimSpace(*request.Persona)
		if !validRuneLength(value, maxPersonaRunes) {
			return ErrInvalidRequest
		}
		draft.Persona = value
		changed = true
	}
	if request.Goals != nil {
		goals := normalizeStringList(*request.Goals)
		if !validStringList(goals, maxGoals, maxGoalRunes) {
			return ErrInvalidRequest
		}
		encoded, err := marshalStringList(goals)
		if err != nil {
			return err
		}
		draft.Goals = encoded
		changed = true
	}
	if request.Rules != nil {
		rules := normalizeRules(*request.Rules)
		if len(rules) > maxRules {
			return ErrInvalidRequest
		}
		for _, rule := range rules {
			if !validRuneLength(rule, maxRuleRunes) {
				return ErrInvalidRequest
			}
		}
		encoded, err := marshalRules(rules)
		if err != nil {
			return err
		}
		draft.Rules = encoded
		changed = true
	}
	if request.DefaultModel != nil {
		value := strings.TrimSpace(*request.DefaultModel)
		if !validRuneLength(value, maxDefaultModelRunes) {
			return ErrInvalidRequest
		}
		draft.DefaultModel = value
		changed = true
	}
	if request.MemoryPolicy != nil {
		if err := validateMemoryPolicy(*request.MemoryPolicy); err != nil {
			return err
		}
		encoded, err := marshalPolicy(*request.MemoryPolicy)
		if err != nil {
			return err
		}
		draft.MemoryPolicy = encoded
		changed = true
	}
	if request.TriggerPolicy != nil {
		encoded, err := marshalPolicy(*request.TriggerPolicy)
		if err != nil {
			return err
		}
		draft.TriggerPolicy = encoded
		changed = true
	}
	if request.PresentationPolicy != nil {
		encoded, err := marshalPresentationPolicy(*request.PresentationPolicy)
		if err != nil {
			return err
		}
		draft.PresentationPolicy = encoded
		changed = true
	}
	if request.ExecutionRuntime != nil {
		draft.ExecutionRuntime = cloneExecutionRuntime(request.ExecutionRuntime)
		changed = true
	}
	if request.FileUploadPolicy != nil {
		request.FileUploadPolicy.Normalize()
		if request.FileUploadPolicy.Validate() != nil {
			return ErrInvalidRequest
		}
		draft.FileUploadPolicy = chatfile.Clone(request.FileUploadPolicy)
		draft.FileUploadPolicy.Normalize()
		changed = true
	}
	if !changed {
		return ErrInvalidRequest
	}
	return nil
}

func normalizeListQuery(scope Scope, query ListQuery) (ListQuery, error) {
	if !validScope(scope) || !utf8.ValidString(query.Search) {
		return ListQuery{}, ErrInvalidRequest
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = defaultPageSize
	}
	query.Search = strings.TrimSpace(query.Search)
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > maxPageSize || utf8.RuneCountInString(query.Search) > maxNameRunes {
		return ListQuery{}, ErrInvalidRequest
	}
	return query, nil
}

func validatePublishable(draft *Version) error {
	if draft == nil || strings.TrimSpace(draft.IdentityName) == "" || strings.TrimSpace(draft.Persona) == "" || strings.TrimSpace(draft.DefaultModel) == "" {
		return ErrPublishInvalid
	}
	var rules []string
	if err := json.Unmarshal(draft.Rules, &rules); err != nil {
		return ErrPublishInvalid
	}
	return nil
}

func validStatusTransition(definition *Definition, next Status) bool {
	if definition == nil {
		return false
	}
	switch next {
	case StatusDraft:
		return definition.PublishedVersionID == nil && definition.Status == StatusDraft
	case StatusActive, StatusDisabled:
		return definition.PublishedVersionID != nil
	default:
		return false
	}
}

func validAutonomy(value AutonomyLevel) bool {
	switch value {
	case AutonomyAssistive, AutonomySupervised, AutonomyAutonomous:
		return true
	default:
		return false
	}
}

func validVisibility(value Visibility) bool {
	return value == VisibilityPrivate || value == VisibilityWorkspace
}

func validIconType(value string) bool {
	return value == "text" || value == "image"
}

func (s *Service) resolveDefinitionIcon(ctx context.Context, definition *Definition) {
	if definition == nil {
		return
	}
	definition.IconURL = ""
	if definition.IconType == "image" && definition.Icon != "" {
		if strings.HasPrefix(definition.Icon, "http://") || strings.HasPrefix(definition.Icon, "https://") {
			definition.IconURL = definition.Icon
			return
		}
		if s.iconResolver != nil {
			if iconURL, err := s.iconResolver.GetFileURL(ctx, definition.Icon); err == nil {
				definition.IconURL = iconURL
				return
			}
		}
	}
	// Preserve display compatibility for local records created before Agent-style icons were added.
	if definition.Avatar != "" {
		definition.IconURL = definition.Avatar
	}
}

func validRuneLength(value string, maxRunes int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxRunes
}

func normalizeRules(rules []string) []string {
	return normalizeStringList(rules)
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func validStringList(values []string, maxItems, maxRunes int) bool {
	if len(values) > maxItems {
		return false
	}
	for _, value := range values {
		if value == "" || !validRuneLength(value, maxRunes) {
			return false
		}
	}
	return true
}

func marshalRules(rules []string) (datatypes.JSON, error) {
	return marshalStringList(rules)
}

func marshalStringList(values []string) (datatypes.JSON, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	return datatypes.JSON(encoded), nil
}

func marshalPolicy(policy map[string]any) (datatypes.JSON, error) {
	if policy == nil {
		policy = map[string]any{}
	}
	encoded, err := json.Marshal(policy)
	if err != nil || len(encoded) > maxPolicyBytes {
		return nil, ErrInvalidRequest
	}
	return datatypes.JSON(encoded), nil
}

func marshalPresentationPolicy(policy PresentationPolicy) (datatypes.JSON, error) {
	policy.HomeTitle = strings.TrimSpace(policy.HomeTitle)
	policy.OpeningStatement = strings.TrimSpace(policy.OpeningStatement)
	policy.InputPlaceholder = strings.TrimSpace(policy.InputPlaceholder)
	policy.SuggestedQuestions = normalizeStringList(policy.SuggestedQuestions)
	if !validRuneLength(policy.HomeTitle, maxPresentationTitleRunes) ||
		!validRuneLength(policy.OpeningStatement, maxOpeningStatementRunes) ||
		!validRuneLength(policy.InputPlaceholder, maxInputPlaceholderRunes) ||
		len(policy.SuggestedQuestions) > maxSuggestedQuestions {
		return nil, ErrInvalidRequest
	}
	for _, question := range policy.SuggestedQuestions {
		if !validRuneLength(question, maxSuggestedQuestionRunes) {
			return nil, ErrInvalidRequest
		}
	}
	encoded, err := json.Marshal(policy)
	if err != nil || len(encoded) > maxPolicyBytes {
		return nil, ErrInvalidRequest
	}
	return datatypes.JSON(encoded), nil
}

func configRevision(version *Version) (string, error) {
	payload := struct {
		FileUploadPolicy   *chatfile.Policy        `json:"file_upload_policy,omitempty"`
		IdentityName       string                  `json:"identity_name"`
		Persona            string                  `json:"persona"`
		Goals              json.RawMessage         `json:"goals"`
		Rules              json.RawMessage         `json:"rules"`
		DefaultModel       string                  `json:"default_model"`
		MemoryPolicy       json.RawMessage         `json:"memory_policy"`
		TriggerPolicy      json.RawMessage         `json:"trigger_policy"`
		PresentationPolicy json.RawMessage         `json:"presentation_policy"`
		ExecutionRuntime   *ExecutionRuntimeConfig `json:"execution_runtime,omitempty"`
	}{
		FileUploadPolicy:   chatfile.Clone(version.FileUploadPolicy),
		IdentityName:       version.IdentityName,
		Persona:            version.Persona,
		Goals:              json.RawMessage(cloneJSON(version.Goals)),
		Rules:              json.RawMessage(cloneJSON(version.Rules)),
		DefaultModel:       version.DefaultModel,
		MemoryPolicy:       json.RawMessage(cloneJSON(version.MemoryPolicy)),
		TriggerPolicy:      json.RawMessage(cloneJSON(version.TriggerPolicy)),
		PresentationPolicy: json.RawMessage(cloneJSON(version.PresentationPolicy)),
		ExecutionRuntime:   cloneExecutionRuntime(version.ExecutionRuntime),
	}
	return hashRevision(payload)
}

func publishedRevision(version *Version) (string, error) {
	return configRevision(version)
}

type identitySnapshotPayload struct {
	Name          string        `json:"name"`
	Avatar        string        `json:"avatar"`
	IconType      string        `json:"icon_type"`
	Icon          string        `json:"icon"`
	Description   string        `json:"description"`
	AutonomyLevel AutonomyLevel `json:"autonomy_level"`
	Visibility    Visibility    `json:"visibility"`
}

func identitySnapshot(definition *Definition) (datatypes.JSON, error) {
	payload := identitySnapshotPayload{
		Name:          definition.Name,
		Avatar:        definition.Avatar,
		IconType:      definition.IconType,
		Icon:          definition.Icon,
		Description:   definition.Description,
		AutonomyLevel: definition.AutonomyLevel,
		Visibility:    definition.Visibility,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode digital human identity snapshot: %w", err)
	}
	return datatypes.JSON(encoded), nil
}

func hashRevision(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode digital human version revision: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return versionRevisionPrefix + hex.EncodeToString(digest[:]), nil
}

func cloneJSON(value datatypes.JSON) datatypes.JSON {
	return append(datatypes.JSON(nil), value...)
}

func applicationError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if _, ok := apperror.As(err); ok {
		return err
	}
	var code apperror.Code
	switch {
	case errors.Is(err, ErrInvalidRequest):
		code = AppCodeRequestInvalid
	case errors.Is(err, ErrNotFound):
		code = AppCodeNotFound
	case errors.Is(err, ErrVersionNotFound):
		code = AppCodeVersionNotFound
	case errors.Is(err, ErrNameConflict):
		code = AppCodeNameConflict
	case errors.Is(err, ErrRevisionConflict):
		code = AppCodeRevisionConflict
	case errors.Is(err, ErrPublishedImmutable):
		code = AppCodePublishedImmutable
	case errors.Is(err, ErrPublishInvalid):
		code = AppCodePublishInvalid
	case errors.Is(err, ErrVersionRestoreImpactChanged):
		code = AppCodeVersionRestoreImpactChanged
	case errors.Is(err, ErrDeleteBlocked):
		code = AppCodeDeleteBlocked
	case errors.Is(err, ErrPersonaTemplateNotFound):
		code = AppCodePersonaTemplateNotFound
	case errors.Is(err, ErrPersonaTemplateNameConflict):
		code = AppCodePersonaTemplateNameConflict
	case errors.Is(err, ErrResourceBindingInvalid):
		code = AppCodeResourceBindingInvalid
	case errors.Is(err, ErrRunNotFound):
		code = AppCodeRunNotFound
	case errors.Is(err, ErrRunConflict):
		code = AppCodeRunConflict
	case errors.Is(err, ErrEndUserSuspended):
		code = AppCodeEndUserSuspended
	case errors.Is(err, ErrConversationNotFound):
		code = AppCodeConversationNotFound
	case errors.Is(err, ErrConsoleFileNotAccessible):
		code = AppCodeConsoleFileNotAccessible
	case errors.Is(err, ErrWebAppFileRequiresLogin):
		code = AppCodeWebAppFileRequiresLogin
	case errors.Is(err, ErrMemoryNotFound):
		code = AppCodeMemoryNotFound
	case errors.Is(err, ErrMemoryConflict):
		code = AppCodeMemoryConflict
	case errors.Is(err, ErrMemoryPolicyRejected):
		code = AppCodeMemoryPolicyRejected
	case errors.Is(err, ErrMemoryUndoUnavailable):
		code = AppCodeMemoryUndoUnavailable
	case errors.Is(err, ErrMemoryProjectionUnavailable):
		code = AppCodeMemoryProjectionUnavailable
	case errors.Is(err, ErrMemoryProjectionRepairFailed):
		code = AppCodeMemoryRepairFailed
	case errors.Is(err, ErrTaskNotFound):
		code = AppCodeTaskNotFound
	case errors.Is(err, ErrTaskConflict):
		code = AppCodeTaskConflict
	case errors.Is(err, ErrTaskVisibilityForbidden), errors.Is(err, ErrTaskReadOnly):
		code = appcatalog.CodePermissionDenied
	case errors.Is(err, ErrTaskPolicyRejected):
		code = AppCodeTaskPolicyRejected
	case errors.Is(err, ErrChannelInstallationNotFound):
		code = AppCodeChannelInstallationNotFound
	case errors.Is(err, ErrChannelConflict):
		code = AppCodeChannelConflict
	case errors.Is(err, ErrChannelNotReady):
		code = AppCodeChannelNotReady
	case errors.Is(err, ErrChannelTransportUnsupported):
		code = AppCodeChannelTransportUnsupported
	case errors.Is(err, ErrChannelCredentialManaged):
		code = AppCodeChannelCredentialManaged
	case errors.Is(err, ErrCredentialStorageUnavailable):
		code = AppCodeCredentialStorageUnavailable
	case errors.Is(err, ErrChannelDeliveryNotFound):
		code = AppCodeChannelDeliveryNotFound
	case errors.Is(err, ErrChannelDeliveryConflict):
		code = AppCodeChannelDeliveryConflict
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
	return apperror.Wrap(err, code, apperror.WithOperation(operation))
}

// taskApplicationError keeps the task REST contract stable without changing
// the broader DigitalHuman error namespace used by configuration and chat.
func taskApplicationError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if _, ok := apperror.As(err); ok {
		return err
	}
	var code apperror.Code
	switch {
	case errors.Is(err, ErrInvalidRequest):
		code = AppCodeTaskInvalidRequest
	case errors.Is(err, ErrTaskNotFound):
		code = AppCodeTaskNotFound
	case errors.Is(err, ErrTaskConflict), errors.Is(err, ErrRevisionConflict):
		code = AppCodeTaskConflict
	case errors.Is(err, ErrTaskVisibilityForbidden), errors.Is(err, ErrTaskReadOnly):
		code = appcatalog.CodePermissionDenied
	case errors.Is(err, ErrTaskPolicyRejected):
		code = AppCodeTaskPolicyRejected
	default:
		return applicationError(err, operation)
	}
	return apperror.Wrap(err, code, apperror.WithOperation(operation))
}
