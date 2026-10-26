package container

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/backgroundauth"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	llmdefaultsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

type socialOperationsBindingReaderStub struct{ items []connectorcore.TargetBinding }

func (s socialOperationsBindingReaderStub) ListBindingsByTarget(context.Context, uuid.UUID, uuid.UUID, connectorcore.TargetType, string, bindings.ListOptions) ([]connectorcore.TargetBinding, error) {
	return append([]connectorcore.TargetBinding(nil), s.items...), nil
}

type socialOperationsConnectionReaderStub struct {
	items map[uuid.UUID]connectorcore.PersonalConnection
}

func (s socialOperationsConnectionReaderStub) GetConnection(_ context.Context, _, _ uuid.UUID, id uuid.UUID) (connectorcore.PersonalConnection, error) {
	return s.items[id], nil
}

type socialOperationsGrantReaderStub struct{ items []backgroundauth.Grant }

func (s socialOperationsGrantReaderStub) ListGrants(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]backgroundauth.Grant, error) {
	return append([]backgroundauth.Grant(nil), s.items...), nil
}

type socialOperationsDefinitionReaderStub struct{ items map[string]catalog.Definition }

func (s socialOperationsDefinitionReaderStub) Get(id string) (catalog.Definition, bool) {
	definition, ok := s.items[id]
	return definition, ok
}

type socialOperationsPrerequisiteReaderStub struct {
	findings []socialops.PrerequisiteFinding
}

type socialOperationsModelResolverStub struct {
	available bool
	vision    bool
}

func (s *socialOperationsModelResolverStub) ResolveModelType(_ context.Context, _ string, _ *string, model *string, _ sharedmodel.ModelType) (*llmdefaultsvc.ResolvedModel, error) {
	if !s.available || model == nil {
		return nil, llmdefaultsvc.ErrModelUnavailable
	}
	return &llmdefaultsvc.ResolvedModel{Provider: "test-provider", Model: *model}, nil
}

func (s *socialOperationsModelResolverStub) ResolveUseCase(_ context.Context, _ string, useCase llmmodel.UseCase, provider, model *string) (*llmdefaultsvc.ResolvedModel, error) {
	if !s.available || !s.vision || useCase != llmmodel.UseCaseVision {
		return nil, llmdefaultsvc.ErrModelUnavailable
	}
	return &llmdefaultsvc.ResolvedModel{Provider: "vision-provider", Model: "vision-model"}, nil
}

func (s socialOperationsPrerequisiteReaderStub) RestoreSocialOperationsPrerequisites(context.Context, socialops.Configuration) ([]socialops.PrerequisiteFinding, error) {
	return append([]socialops.PrerequisiteFinding(nil), s.findings...), nil
}

func TestSocialOperationsPreflightRestorerRequiresMatchingLiveGrantSnapshots(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	configuration := socialOperationsRestorerConfiguration()
	configuration.BrandArtifactRefs = []uuid.UUID{uuid.New()}
	configuration.TeamHandoff.ConnectorID = "gmail"
	configuration.TeamHandoff.OperationID = "gmail.message.send"
	configuration.TeamHandoff.Destination = "ops@example.com"
	for index := range configuration.Platforms[0].Capabilities {
		if configuration.Platforms[0].Capabilities[index].Capability == socialops.CapabilityImages {
			configuration.Platforms[0].Capabilities[index].Mode = socialops.RequirementRequired
		}
	}
	xDefinition := catalog.SocialMCPDefinitions()[0]
	if xDefinition.ID != "x-social" {
		t.Fatal("x-social catalog order changed")
	}
	xDigest, err := catalog.DefinitionContractDigest(xDefinition)
	if err != nil {
		t.Fatal(err)
	}
	mailDefinition, ok := catalog.MailDefinition("gmail")
	if !ok {
		t.Fatal("gmail definition missing")
	}
	mailDefinition, err = catalog.WithDigitalHumanSurface(mailDefinition, mailDefinition.PluginVersion+"-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	mailDigest, err := catalog.DefinitionContractDigest(mailDefinition)
	if err != nil {
		t.Fatal(err)
	}
	builtins, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var researchDefinition catalog.Definition
	for _, definition := range builtins {
		if definition.ID == "web-search" {
			researchDefinition = definition
			break
		}
	}
	if researchDefinition.ID == "" {
		t.Fatal("web-search definition missing")
	}
	researchDigest, err := catalog.DefinitionContractDigest(researchDefinition)
	if err != nil {
		t.Fatal(err)
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: configuration.Scope.EmployeeID.String(), Scope: connectorcore.ScopePublished,
		VersionID: configuration.EmployeeVersionID.String(), BindingInstanceID: uuid.NewString()}
	xBinding := connectorcore.TargetBinding{ID: uuid.New(), OrganizationID: configuration.Scope.OrganizationID, OwnerAccountID: configuration.Scope.OwnerID,
		ConnectionID: configuration.Platforms[0].ConnectionID, ConnectorID: "x-social", Target: target, Status: connectorcore.BindingStatusActive, Revision: 3}
	mailTarget := target
	mailTarget.BindingInstanceID = uuid.NewString()
	mailBinding := connectorcore.TargetBinding{ID: uuid.New(), OrganizationID: configuration.Scope.OrganizationID, OwnerAccountID: configuration.Scope.OwnerID,
		ConnectionID: configuration.TeamHandoff.ConnectionID, ConnectorID: "gmail", Target: mailTarget, Status: connectorcore.BindingStatusActive, Revision: 5}
	researchTarget := target
	researchTarget.BindingInstanceID = uuid.NewString()
	researchBinding := connectorcore.TargetBinding{ID: uuid.New(), OrganizationID: configuration.Scope.OrganizationID, OwnerAccountID: configuration.Scope.OwnerID,
		ConnectionID: configuration.Research.ConnectionID, ConnectorID: connectorcore.Identifier(configuration.Research.ConnectorID), Target: researchTarget, Status: connectorcore.BindingStatusActive, Revision: 2}
	xBinding.Constraint = socialOperationsBindingConstraint(configuration.Scope.OwnerID, target, xDefinition)
	mailBinding.Constraint = socialOperationsBindingConstraint(configuration.Scope.OwnerID, mailTarget, mailDefinition)
	researchBinding.Constraint = socialOperationsBindingConstraint(configuration.Scope.OwnerID, researchTarget, researchDefinition)
	accountID := configuration.Platforms[0].AccountID
	xConnection := socialOperationsReadyConnection(configuration, xBinding, accountID, xDefinition)
	mailAccount := "ops@example.com"
	mailConnection := socialOperationsReadyConnection(configuration, mailBinding, mailAccount, mailDefinition)
	researchConnection := socialOperationsReadyConnection(configuration, researchBinding, "exa-account", researchDefinition)
	xGrant := socialOperationsGrant(configuration, xBinding, xConnection, xDigest, []string{"x-social.post.create"}, nil, now)
	mailGrant := socialOperationsGrant(configuration, mailBinding, mailConnection, mailDigest, []string{"gmail.message.send"}, map[string]string{"to": configuration.TeamHandoff.Destination}, now)
	researchGrant := socialOperationsGrant(configuration, researchBinding, researchConnection, researchDigest, []string{"web.search", "web.fetch"}, nil, now)

	restorer, err := NewSocialOperationsPreflightRestorer(
		socialOperationsBindingReaderStub{items: []connectorcore.TargetBinding{xBinding, mailBinding, researchBinding}},
		socialOperationsConnectionReaderStub{items: map[uuid.UUID]connectorcore.PersonalConnection{xConnection.ID: xConnection, mailConnection.ID: mailConnection, researchConnection.ID: researchConnection}},
		socialOperationsGrantReaderStub{items: []backgroundauth.Grant{xGrant, mailGrant, researchGrant}},
		socialOperationsDefinitionReaderStub{items: map[string]catalog.Definition{"x-social": xDefinition, "gmail": mailDefinition, "web-search": researchDefinition}},
		socialOperationsPrerequisiteReaderStub{findings: []socialops.PrerequisiteFinding{
			{Key: "default_model", Ready: true, Reason: "configured"}, {Key: "social_operations_skill", Ready: true, Reason: "bound"},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	restorer.now = func() time.Time { return now }
	input, err := restorer.RestoreSocialOperationsPreflight(t.Context(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	report, err := socialops.Preflight(configuration, input)
	if err != nil || !report.Ready || len(input.Connections) != 2 || !report.Handoff.Ready {
		t.Fatalf("ready report = %#v, input = %#v, error = %v", report, input, err)
	}
	mismatchedResearch := configuration
	mismatchedResearch.Research.ConnectionID = xConnection.ID
	mismatchedInput, err := restorer.RestoreSocialOperationsPreflight(t.Context(), mismatchedResearch)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedReport, err := socialops.Preflight(mismatchedResearch, mismatchedInput)
	if err != nil || mismatchedReport.Ready || prerequisiteReady(mismatchedReport.Prerequisites, "research") {
		t.Fatalf("mismatched research report = %#v, error = %v", mismatchedReport, err)
	}

	mailGrant.UsedInvocations = mailGrant.MaxInvocations
	restorer.grants = socialOperationsGrantReaderStub{items: []backgroundauth.Grant{xGrant, mailGrant, researchGrant}}
	input, err = restorer.RestoreSocialOperationsPreflight(t.Context(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	report, err = socialops.Preflight(configuration, input)
	if err != nil || report.Ready || report.Handoff.Ready {
		t.Fatalf("exhausted handoff grant report = %#v, error = %v", report, err)
	}

	mailGrant.UsedInvocations = 0
	mailGrant.ContractDigest = strings.Repeat("f", 64)
	restorer.grants = socialOperationsGrantReaderStub{items: []backgroundauth.Grant{xGrant, mailGrant, researchGrant}}
	input, err = restorer.RestoreSocialOperationsPreflight(t.Context(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	report, err = socialops.Preflight(configuration, input)
	if err != nil || report.Handoff.Ready {
		t.Fatalf("stale contract grant report = %#v, error = %v", report, err)
	}
}

func TestSocialOperationsKnownAccountIDsUsesOnlyVerifiedLinkedInEvidence(t *testing.T) {
	identity := "person-42"
	connection := connectorcore.PersonalConnection{ConnectorID: "linkedin-social", ExternalAccountID: &identity, ProviderEvidence: &connectorcore.ProviderEvidence{
		Social: &connectorcore.SocialEvidence{Version: 1, Provider: "linkedin-social", Identity: identity,
			AccountIDs: []string{"urn:li:organization:123", "urn:li:organization:456"}, CheckedAt: time.Now().UTC()},
	}}
	accounts := socialOperationsKnownAccountIDs(connection)
	if len(accounts) != 2 || accounts[0] != "urn:li:organization:123" || accounts[1] != "urn:li:organization:456" {
		t.Fatalf("known account IDs = %#v", accounts)
	}
	connection.ProviderEvidence.Social.AccountIDs = []string{"urn:li:organization:123", "urn:li:person:456"}
	accounts = socialOperationsKnownAccountIDs(connection)
	if len(accounts) != 0 {
		t.Fatalf("malformed evidence account IDs = %#v", accounts)
	}
}

func TestSocialOperationsDatabasePrerequisitesRequirePublishedModelAndSkill(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	if err := db.AutoMigrate(&digitalhuman.Version{}, &digitalhuman.ResourceBinding{}); err != nil {
		t.Fatal(err)
	}
	configuration := socialOperationsRestorerConfiguration()
	for index := range configuration.Platforms[0].Capabilities {
		if configuration.Platforms[0].Capabilities[index].Capability == socialops.CapabilityImages {
			configuration.Platforms[0].Capabilities[index].Mode = socialops.RequirementRequired
		}
	}
	version := digitalhuman.Version{ID: configuration.EmployeeVersionID, DigitalHumanID: configuration.Scope.EmployeeID,
		OrganizationID: configuration.Scope.OrganizationID, WorkspaceID: configuration.Scope.WorkspaceID, State: digitalhuman.VersionStatePublished,
		DefaultModel: "", Revision: "sha256:test", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	models := &socialOperationsModelResolverStub{}
	reader := socialOperationsDatabasePrerequisites{db: db, models: models}
	findings, err := reader.RestoreSocialOperationsPrerequisites(t.Context(), configuration)
	if err != nil || findings[0].Ready || findings[1].Ready || findings[2].Ready || !findings[3].Ready {
		t.Fatalf("missing prerequisites = %#v, %v", findings, err)
	}
	if err := db.Model(&digitalhuman.Version{}).Where("id = ?", version.ID).Update("default_model", "model-1").Error; err != nil {
		t.Fatal(err)
	}
	findings, err = reader.RestoreSocialOperationsPrerequisites(t.Context(), configuration)
	if err != nil || findings[0].Ready {
		t.Fatalf("unavailable configured model = %#v, %v", findings, err)
	}
	binding := digitalhuman.ResourceBinding{ID: uuid.New(), OrganizationID: configuration.Scope.OrganizationID, WorkspaceID: configuration.Scope.WorkspaceID,
		DigitalHumanID: configuration.Scope.EmployeeID, DigitalHumanVersionID: configuration.EmployeeVersionID,
		BindingType: resourcebindings.BindingTypeSkill, ResourceID: "social-operations", AccessMode: resourcebindings.AccessModeExecute,
		Metadata: []byte(`{}`), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	models.available = true
	findings, err = reader.RestoreSocialOperationsPrerequisites(t.Context(), configuration)
	if err != nil || !findings[0].Ready || findings[1].Ready || !findings[2].Ready {
		t.Fatalf("missing vision prerequisite = %#v, %v", findings, err)
	}
	models.vision = true
	findings, err = reader.RestoreSocialOperationsPrerequisites(t.Context(), configuration)
	if err != nil || !findings[0].Ready || !findings[1].Ready || !findings[2].Ready || !findings[3].Ready {
		t.Fatalf("ready prerequisites = %#v, %v", findings, err)
	}
	configuration.PublicationLanguage = "ja-JP"
	if finding := socialOperationsImageTextLanguagePrerequisite(configuration); finding.Ready {
		t.Fatalf("unsupported image text language = %#v", finding)
	}
}

func prerequisiteReady(findings []socialops.PrerequisiteFinding, key string) bool {
	for _, finding := range findings {
		if finding.Key == key {
			return finding.Ready
		}
	}
	return false
}

func socialOperationsRestorerConfiguration() socialops.Configuration {
	scope := socialops.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}
	capabilities := make([]socialops.CapabilityRequirement, 0, 7)
	for _, capability := range []socialops.Capability{socialops.CapabilityAccount, socialops.CapabilityPost, socialops.CapabilityReadback, socialops.CapabilityComment, socialops.CapabilityInbox, socialops.CapabilityMetrics, socialops.CapabilityImages} {
		mode := socialops.RequirementOptional
		if capability == socialops.CapabilityPost {
			mode = socialops.RequirementRequired
		}
		capabilities = append(capabilities, socialops.CapabilityRequirement{Capability: capability, Mode: mode})
	}
	return socialops.Configuration{
		Scope: scope, EmployeeVersionID: uuid.New(), Revision: 1,
		Product: socialops.ProductConfiguration{Name: "Acme", Description: "Analytics"}, Audience: "operators",
		PublicationLanguage: "en-US", InternalReviewLanguage: "zh-Hans", Sources: []socialops.SourceReference{{URL: "https://example.com"}},
		Research:  socialops.ResearchBinding{ConnectorID: "web-search", ConnectionID: uuid.New()},
		Platforms: []socialops.PlatformBinding{{Platform: "x", ConnectorID: "x-social", ConnectionID: uuid.New(), AccountID: "x-account", MaxDailyPublications: 2, Capabilities: capabilities}},
		Timezone:  "UTC", Cadence: socialops.Cadence{DailyContentCron: "0 9 * * *", InboxCron: "*/20 * * * *", SevenDaySampleCron: "15 10 * * *", WeeklyReviewCron: "0 16 * * 5"},
		TeamHandoff: socialops.TeamHandoff{ConnectorID: "x-social", ConnectionID: uuid.New(), OperationID: "x-social.message.send", Destination: "conversation-42"},
	}
}

func socialOperationsReadyConnection(configuration socialops.Configuration, binding connectorcore.TargetBinding, externalID string, definition catalog.Definition) connectorcore.PersonalConnection {
	operations := make([]connectorcore.Identifier, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		operations = append(operations, connectorcore.Identifier(operation.ID))
	}
	connectionConstraint := connectorcore.OwnerConstraint(configuration.Scope.OwnerID, operations)
	return connectorcore.PersonalConnection{Constraint: connectionConstraint, ID: binding.ConnectionID, OrganizationID: configuration.Scope.OrganizationID, OwnerAccountID: configuration.Scope.OwnerID,
		ConnectorID: binding.ConnectorID, ExternalAccountID: &externalID, State: connectorcore.ReadinessReady, UsableOperationIDs: operations,
		UnavailableOperations: map[connectorcore.Identifier]string{}, Revision: 7, CredentialVersion: 2}
}

func socialOperationsBindingConstraint(owner uuid.UUID, target connectorcore.TargetRef, definition catalog.Definition) *connectorcore.AccessConstraint {
	operations := make([]connectorcore.Identifier, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		operations = append(operations, connectorcore.Identifier(operation.ID))
	}
	return &connectorcore.AccessConstraint{SchemaVersion: 1, AudienceOwner: owner, AllowedOperationIDs: operations,
		TargetBoundaries: []connectorcore.TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.EffectiveScope()), VersionID: target.VersionID}},
		DenyReasons:      []string{}, AuthorizationMode: connectorcore.AuthorizationModeDirect}
}

func socialOperationsGrant(configuration socialops.Configuration, binding connectorcore.TargetBinding, connection connectorcore.PersonalConnection, digest string, operations []string, destinations map[string]string, now time.Time) backgroundauth.Grant {
	encodedOperations, _ := json.Marshal(operations)
	if destinations == nil {
		destinations = map[string]string{}
	}
	encodedDestinations, _ := json.Marshal(destinations)
	return backgroundauth.Grant{ID: uuid.New(), Scope: backgroundauth.Scope{
		OrganizationID: configuration.Scope.OrganizationID, WorkspaceID: configuration.Scope.WorkspaceID, OwnerID: configuration.Scope.OwnerID,
		EmployeeID: configuration.Scope.EmployeeID, VersionID: configuration.EmployeeVersionID, BindingID: binding.ID, ConnectionID: connection.ID,
		BindingRevision: binding.Revision, ConnectionRevision: connection.Revision, CredentialVersion: connection.CredentialVersion, ContractDigest: digest,
	}, OperationsJSON: string(encodedOperations), DestinationsJSON: string(encodedDestinations), ExpiresAt: now.Add(time.Hour), MaxInvocations: 10, CreatedAt: now}
}
