package integrations

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

type memoryActionPolicyRepository struct {
	policies map[string]IntegrationActionPolicy
}

func newMemoryActionPolicyRepository() *memoryActionPolicyRepository {
	return &memoryActionPolicyRepository{policies: make(map[string]IntegrationActionPolicy)}
}

func policyMemoryKey(organizationID uuid.UUID, integrationID, actionID string) string {
	return organizationID.String() + "/" + integrationID + "/" + actionID
}

func (repository *memoryActionPolicyRepository) Get(_ context.Context, organizationID uuid.UUID, integrationID, actionID string) (*IntegrationActionPolicy, error) {
	policy, exists := repository.policies[policyMemoryKey(organizationID, integrationID, actionID)]
	if !exists {
		return nil, nil
	}
	return &policy, nil
}

func (repository *memoryActionPolicyRepository) List(_ context.Context, organizationID uuid.UUID, integrationID string) ([]IntegrationActionPolicy, error) {
	result := make([]IntegrationActionPolicy, 0, len(repository.policies))
	for _, policy := range repository.policies {
		if policy.OrganizationID == organizationID && policy.IntegrationID == integrationID {
			result = append(result, policy)
		}
	}
	return result, nil
}

func (repository *memoryActionPolicyRepository) Replace(_ context.Context, organizationID uuid.UUID, integrationID string, policies []IntegrationActionPolicy) error {
	for key, policy := range repository.policies {
		if policy.OrganizationID == organizationID && policy.IntegrationID == integrationID {
			delete(repository.policies, key)
		}
	}
	for _, policy := range policies {
		repository.policies[policyMemoryKey(organizationID, integrationID, policy.ActionID)] = policy
	}
	return nil
}

func (repository *memoryActionPolicyRepository) ReplaceIfRevision(ctx context.Context, organizationID uuid.UUID, integrationID, expectedRevision string, actions []ActionDefinition, policies []IntegrationActionPolicy) error {
	current, err := repository.List(ctx, organizationID, integrationID)
	if err != nil {
		return err
	}
	if actionPolicyRevision(actions, current) != expectedRevision {
		return ErrActionPolicyChanged
	}
	return repository.Replace(ctx, organizationID, integrationID, policies)
}

func TestActionPolicyServiceDefaultsToProviderPolicyAndOnlyTightens(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	searchAction := ActionDefinition{
		ID: ActionWebSearch, Name: "Search", Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: "api.exa.ai",
	}
	fetchAction := ActionDefinition{
		ID: ActionWebFetch, Name: "Fetch", Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelMedium,
		DataEgress: true, ExternalDestination: "api.exa.ai",
	}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{searchAction, fetchAction}})
	organizationID := uuid.New()
	decision, err := service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, searchAction)
	if err != nil {
		t.Fatalf("Resolve(default) error = %v", err)
	}
	if !decision.Enabled || !decision.DataEgressAllowed || decision.ApprovalPolicy != IntegrationApprovalPolicyInherit {
		t.Fatalf("default decision = %#v", decision)
	}

	views, err := service.Replace(context.Background(), organizationID, IntegrationWebSearch, []ActionPolicyInput{
		{ActionID: ActionWebSearch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false},
		{ActionID: ActionWebFetch, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true},
	}, nil)
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("policy views = %#v", views)
	}
	var searchView ActionPolicyView
	for _, view := range views {
		if view.ActionID == ActionWebSearch {
			searchView = view
		}
	}
	if searchView.Enabled || searchView.DataEgressAllowed || searchView.ApprovalPolicy != IntegrationApprovalPolicyAlwaysAsk {
		t.Fatalf("search view = %#v", searchView)
	}
	if searchView.Effect != searchAction.Effect || searchView.RiskLevel != searchAction.RiskLevel || searchView.ExternalDestination != searchAction.ExternalDestination {
		t.Fatalf("provider governance changed by organization policy: %#v", searchView)
	}
	decision, err = service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, searchAction)
	if err != nil || decision.Enabled || decision.DataEgressAllowed {
		t.Fatalf("Resolve(stored) = %#v, %v", decision, err)
	}
}

func TestActionPolicyServiceUsesDeclaredActionDefaults(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	action := ActionDefinition{
		ID: ActionWebSearch, Name: "Search", DataEgress: true,
		DefaultPolicy: &DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: false,
		},
	}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.New()
	decision, err := service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, action)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if decision.Enabled || decision.DataEgressAllowed || decision.ApprovalPolicy != IntegrationApprovalPolicyAlwaysAsk {
		t.Fatalf("provider default decision = %#v", decision)
	}
	views, err := service.List(context.Background(), organizationID, IntegrationWebSearch)
	if err != nil || len(views) != 1 {
		t.Fatalf("List() = %#v, %v", views, err)
	}
	if views[0].Enabled || views[0].DataEgressAllowed || views[0].ApprovalPolicy != IntegrationApprovalPolicyAlwaysAsk {
		t.Fatalf("provider default view = %#v", views[0])
	}
}

func TestActionPolicyServiceCanEnableDefaultDisabledActionWithoutDowngradingApproval(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	action := ActionDefinition{
		ID: ActionWebSearch, Name: "Send", DataEgress: true,
		DefaultPolicy: &DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
		},
	}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.New()
	views, err := service.Replace(context.Background(), organizationID, IntegrationWebSearch, []ActionPolicyInput{{
		ActionID: ActionWebSearch, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || !views[0].Enabled || views[0].ApprovalPolicy != IntegrationApprovalPolicyAlwaysAsk {
		t.Fatalf("effective policy view = %#v", views)
	}
	decision, err := service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, action)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Enabled || decision.ApprovalPolicy != IntegrationApprovalPolicyAlwaysAsk || !decision.DataEgressAllowed {
		t.Fatalf("effective policy decision = %#v", decision)
	}
}

func TestActionPolicyServiceRejectsUnknownDuplicateAndApprovalDowngrade(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	action := ActionDefinition{ID: ActionWebSearch, Name: "Search", DataEgress: true}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.New()
	tests := [][]ActionPolicyInput{
		{{ActionID: "unknown", Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true}},
		{
			{ActionID: ActionWebSearch, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true},
			{ActionID: ActionWebSearch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false},
		},
		{{ActionID: ActionWebSearch, Enabled: true, ApprovalPolicy: "never_ask", DataEgressAllowed: true}},
	}
	for index, inputs := range tests {
		if _, err := service.Replace(context.Background(), organizationID, IntegrationWebSearch, inputs, nil); err == nil {
			t.Fatalf("Replace(test %d) error = nil", index)
		}
	}
	if len(repository.policies) != 0 {
		t.Fatalf("invalid policy input mutated repository: %#v", repository.policies)
	}
}

func TestActionPolicyServiceEmptyReplaceRestoresDefaults(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	action := ActionDefinition{ID: ActionWebSearch, Name: "Search", DataEgress: true}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.New()
	if _, err := service.Replace(context.Background(), organizationID, IntegrationWebSearch, []ActionPolicyInput{{
		ActionID: ActionWebSearch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false,
	}}, nil); err != nil {
		t.Fatal(err)
	}
	views, err := service.Replace(context.Background(), organizationID, IntegrationWebSearch, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || !views[0].Enabled || !views[0].DataEgressAllowed || views[0].ApprovalPolicy != IntegrationApprovalPolicyInherit {
		t.Fatalf("restored views = %#v", views)
	}
}

func TestActionPolicyServiceVersionedReplaceRejectsStaleAdministratorSnapshot(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	action := ActionDefinition{ID: ActionWebSearch, Name: "Search", DataEgress: true}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.New()
	initial, err := service.ListVersioned(context.Background(), organizationID, IntegrationWebSearch)
	if err != nil || len(initial.Revision) != 64 {
		t.Fatalf("ListVersioned() = %#v, %v", initial, err)
	}
	updated, err := service.ReplaceVersioned(context.Background(), organizationID, IntegrationWebSearch, initial.Revision, []ActionPolicyInput{{
		ActionID: ActionWebSearch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false,
	}}, nil)
	if err != nil || updated.Revision == initial.Revision {
		t.Fatalf("ReplaceVersioned() = %#v, %v", updated, err)
	}
	_, err = service.ReplaceVersioned(context.Background(), organizationID, IntegrationWebSearch, initial.Revision, []ActionPolicyInput{{
		ActionID: ActionWebSearch, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}}, nil)
	if ErrorCode(err) != ErrorCodePolicyConflict {
		t.Fatalf("stale ReplaceVersioned() error = %v, code = %q", err, ErrorCode(err))
	}
	decision, resolveErr := service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, action)
	if resolveErr != nil || decision.Enabled || decision.DataEgressAllowed {
		t.Fatalf("stale replace changed policy: decision=%#v error=%v", decision, resolveErr)
	}
}

func TestActionPolicyServiceVersionedReplaceRequiresCompleteCurrentCatalog(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	actions := []ActionDefinition{
		{ID: ActionWebSearch, Name: "Search", DataEgress: true, ExternalDestination: "api.exa.ai"},
		{ID: ActionWebFetch, Name: "Fetch", DataEgress: true, ExternalDestination: "api.exa.ai"},
	}
	service := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: actions})
	organizationID := uuid.New()
	initial, err := service.ListVersioned(context.Background(), organizationID, IntegrationWebSearch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceVersioned(context.Background(), organizationID, IntegrationWebSearch, initial.Revision, []ActionPolicyInput{{
		ActionID: ActionWebSearch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false,
	}}, nil)
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("incomplete ReplaceVersioned() error = %v, code = %q", err, ErrorCode(err))
	}
	if len(repository.policies) != 0 {
		t.Fatalf("incomplete policy set mutated repository: %#v", repository.policies)
	}
}

func TestActionPolicyServiceCatalogChangeInvalidatesAdministratorRevision(t *testing.T) {
	repository := newMemoryActionPolicyRepository()
	catalog := &staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{{
		ID: ActionWebSearch, Name: "Search", DataEgress: true, ExternalDestination: "api.exa.ai",
	}}}
	service := NewActionPolicyService(repository, catalog)
	organizationID := uuid.New()
	initial, err := service.ListVersioned(context.Background(), organizationID, IntegrationWebSearch)
	if err != nil {
		t.Fatal(err)
	}
	catalog.actions = append(catalog.actions, ActionDefinition{
		ID: ActionWebFetch, Name: "Fetch", DataEgress: true, ExternalDestination: "api.exa.ai",
	})
	_, err = service.ReplaceVersioned(context.Background(), organizationID, IntegrationWebSearch, initial.Revision, []ActionPolicyInput{
		{ActionID: ActionWebSearch, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true},
		{ActionID: ActionWebFetch, Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false},
	}, nil)
	if ErrorCode(err) != ErrorCodePolicyConflict {
		t.Fatalf("catalog-stale ReplaceVersioned() error = %v, code = %q", err, ErrorCode(err))
	}
	decision, resolveErr := service.Resolve(context.Background(), organizationID.String(), IntegrationWebSearch, catalog.actions[1])
	if resolveErr != nil || !decision.Enabled || !decision.DataEgressAllowed {
		t.Fatalf("stale catalog replace changed new action default: decision=%#v error=%v", decision, resolveErr)
	}
}
