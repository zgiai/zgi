package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ActionPolicyResolver interface {
	Resolve(ctx context.Context, organizationID, integrationID string, action ActionDefinition) (ActionPolicyDecision, error)
}

type ActionPolicyService interface {
	ActionPolicyResolver
	List(ctx context.Context, organizationID uuid.UUID, integrationID string) ([]ActionPolicyView, error)
	Replace(ctx context.Context, organizationID uuid.UUID, integrationID string, inputs []ActionPolicyInput, actorID *uuid.UUID) ([]ActionPolicyView, error)
	ListVersioned(ctx context.Context, organizationID uuid.UUID, integrationID string) (ActionPolicySetView, error)
	ReplaceVersioned(ctx context.Context, organizationID uuid.UUID, integrationID, expectedRevision string, inputs []ActionPolicyInput, actorID *uuid.UUID) (ActionPolicySetView, error)
}

type DefaultActionPolicyService struct {
	repository ActionPolicyRepository
	catalog    ConnectionCatalog
}

func NewActionPolicyService(repository ActionPolicyRepository, catalog ConnectionCatalog) *DefaultActionPolicyService {
	return &DefaultActionPolicyService{repository: repository, catalog: catalog}
}

func (service *DefaultActionPolicyService) Resolve(ctx context.Context, organizationID, integrationID string, action ActionDefinition) (ActionPolicyDecision, error) {
	decision := defaultActionPolicyDecision(action)
	if service == nil || service.repository == nil {
		return ActionPolicyDecision{}, fmt.Errorf("integration action policy service is unavailable")
	}
	parsedOrganizationID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil || parsedOrganizationID == uuid.Nil {
		return ActionPolicyDecision{}, invalidInput("organization id is required", err)
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	if action.ID == "" {
		return ActionPolicyDecision{}, invalidInput("action id is required", nil)
	}
	policy, err := service.repository.Get(ctx, parsedOrganizationID, integrationID, action.ID)
	if err != nil {
		return ActionPolicyDecision{}, err
	}
	if policy == nil {
		return decision, nil
	}
	decision.Enabled = policy.Enabled
	if policy.ApprovalPolicy == IntegrationApprovalPolicyAlwaysAsk {
		decision.ApprovalPolicy = IntegrationApprovalPolicyAlwaysAsk
	}
	decision.DataEgressAllowed = decision.DataEgressAllowed && (!action.DataEgress || policy.DataEgressAllowed)
	return decision, nil
}

func (service *DefaultActionPolicyService) List(ctx context.Context, organizationID uuid.UUID, integrationID string) ([]ActionPolicyView, error) {
	views, _, _, err := service.listWithStored(ctx, organizationID, integrationID)
	return views, err
}

func (service *DefaultActionPolicyService) ListVersioned(ctx context.Context, organizationID uuid.UUID, integrationID string) (ActionPolicySetView, error) {
	views, actions, stored, err := service.listWithStored(ctx, organizationID, integrationID)
	if err != nil {
		return ActionPolicySetView{}, err
	}
	return ActionPolicySetView{Revision: actionPolicyRevision(actions, stored), Items: views}, nil
}

func (service *DefaultActionPolicyService) listWithStored(ctx context.Context, organizationID uuid.UUID, integrationID string) ([]ActionPolicyView, []ActionDefinition, []IntegrationActionPolicy, error) {
	if service == nil || service.repository == nil || service.catalog == nil {
		return nil, nil, nil, fmt.Errorf("integration action policy service is unavailable")
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	actions := service.catalog.Actions(integrationID)
	if organizationID == uuid.Nil || len(actions) == 0 {
		return nil, nil, nil, invalidInput("organization or integration is invalid", nil)
	}
	stored, err := service.repository.List(ctx, organizationID, integrationID)
	if err != nil {
		return nil, nil, nil, err
	}
	byActionID := make(map[string]IntegrationActionPolicy, len(stored))
	for _, policy := range stored {
		byActionID[policy.ActionID] = policy
	}
	views := make([]ActionPolicyView, 0, len(actions))
	for _, action := range actions {
		defaults := defaultActionPolicyDecision(action)
		view := ActionPolicyView{
			IntegrationID:       integrationID,
			ActionID:            action.ID,
			Name:                action.Name,
			Effect:              action.Effect,
			RiskLevel:           action.RiskLevel,
			DataEgress:          action.DataEgress,
			ExternalDestination: action.ExternalDestination,
			Enabled:             defaults.Enabled,
			ApprovalPolicy:      defaults.ApprovalPolicy,
			DataEgressAllowed:   defaults.DataEgressAllowed,
		}
		if policy, exists := byActionID[action.ID]; exists {
			view.Enabled = policy.Enabled
			if policy.ApprovalPolicy == IntegrationApprovalPolicyAlwaysAsk {
				view.ApprovalPolicy = IntegrationApprovalPolicyAlwaysAsk
			}
			view.DataEgressAllowed = view.DataEgressAllowed && (!action.DataEgress || policy.DataEgressAllowed)
		}
		views = append(views, view)
	}
	return views, actions, stored, nil
}

func defaultActionPolicyDecision(action ActionDefinition) ActionPolicyDecision {
	decision := ActionPolicyDecision{
		Enabled:           true,
		ApprovalPolicy:    IntegrationApprovalPolicyInherit,
		DataEgressAllowed: true,
	}
	if action.DefaultPolicy == nil {
		return decision
	}
	decision.Enabled = action.DefaultPolicy.Enabled
	decision.DataEgressAllowed = !action.DataEgress || action.DefaultPolicy.DataEgressAllowed
	if action.DefaultPolicy.ApprovalPolicy == "always_ask" {
		decision.ApprovalPolicy = IntegrationApprovalPolicyAlwaysAsk
	}
	return decision
}

func (service *DefaultActionPolicyService) Replace(ctx context.Context, organizationID uuid.UUID, integrationID string, inputs []ActionPolicyInput, actorID *uuid.UUID) ([]ActionPolicyView, error) {
	integrationID, _, policies, err := service.validatedPolicies(organizationID, integrationID, inputs, actorID, false)
	if err != nil {
		return nil, err
	}
	if err := service.repository.Replace(ctx, organizationID, integrationID, policies); err != nil {
		return nil, err
	}
	return service.List(ctx, organizationID, integrationID)
}

func (service *DefaultActionPolicyService) ReplaceVersioned(ctx context.Context, organizationID uuid.UUID, integrationID, expectedRevision string, inputs []ActionPolicyInput, actorID *uuid.UUID) (ActionPolicySetView, error) {
	integrationID, actions, policies, err := service.validatedPolicies(organizationID, integrationID, inputs, actorID, true)
	if err != nil {
		return ActionPolicySetView{}, err
	}
	versionedRepository, ok := service.repository.(VersionedActionPolicyRepository)
	if !ok {
		return ActionPolicySetView{}, fmt.Errorf("versioned integration action policy repository is unavailable")
	}
	if err := versionedRepository.ReplaceIfRevision(ctx, organizationID, integrationID, expectedRevision, actions, policies); err != nil {
		if errors.Is(err, ErrActionPolicyChanged) {
			return ActionPolicySetView{}, NewError(ErrorCodePolicyConflict, "integration action policies changed; reload them and retry", err)
		}
		return ActionPolicySetView{}, err
	}
	return service.ListVersioned(ctx, organizationID, integrationID)
}

func (service *DefaultActionPolicyService) validatedPolicies(organizationID uuid.UUID, integrationID string, inputs []ActionPolicyInput, actorID *uuid.UUID, requireComplete bool) (string, []ActionDefinition, []IntegrationActionPolicy, error) {
	if service == nil || service.repository == nil || service.catalog == nil {
		return "", nil, nil, fmt.Errorf("integration action policy service is unavailable")
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	actions := service.catalog.Actions(integrationID)
	if organizationID == uuid.Nil || len(actions) == 0 {
		return "", nil, nil, invalidInput("organization or integration is invalid", nil)
	}
	actionByID := make(map[string]ActionDefinition, len(actions))
	for _, action := range actions {
		actionByID[action.ID] = action
	}
	seen := make(map[string]struct{}, len(inputs))
	policies := make([]IntegrationActionPolicy, 0, len(inputs))
	for _, input := range inputs {
		actionID := strings.ToLower(strings.TrimSpace(input.ActionID))
		action, exists := actionByID[actionID]
		if !exists {
			return "", nil, nil, invalidInput("action policy references an unknown action", nil)
		}
		if _, duplicated := seen[actionID]; duplicated {
			return "", nil, nil, invalidInput("action policy is duplicated", nil)
		}
		seen[actionID] = struct{}{}
		approval := input.ApprovalPolicy
		if approval == "" {
			approval = IntegrationApprovalPolicyInherit
		}
		if approval != IntegrationApprovalPolicyInherit && approval != IntegrationApprovalPolicyAlwaysAsk {
			return "", nil, nil, invalidInput("approval policy must be inherit or always_ask", nil)
		}
		dataEgressAllowed := input.DataEgressAllowed
		if !action.DataEgress {
			dataEgressAllowed = true
		} else if action.DefaultPolicy != nil && !action.DefaultPolicy.DataEgressAllowed {
			// Organization policy can tighten provider defaults, never loosen
			// a provider-owned external-data boundary.
			dataEgressAllowed = false
		}
		policies = append(policies, IntegrationActionPolicy{
			OrganizationID:    organizationID,
			IntegrationID:     integrationID,
			ActionID:          actionID,
			Enabled:           input.Enabled,
			ApprovalPolicy:    approval,
			DataEgressAllowed: dataEgressAllowed,
			UpdatedBy:         cloneUUIDPointer(actorID),
		})
	}
	if requireComplete && len(seen) != len(actionByID) {
		return "", nil, nil, invalidInput("action policy set must include every current integration action", nil)
	}
	return integrationID, actions, policies, nil
}
