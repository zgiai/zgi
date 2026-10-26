package container

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/backgroundauth"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	socialmcp "github.com/zgiai/zgi/api/internal/modules/connectors/providers/socialmcp"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	llmdefaultsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"gorm.io/gorm"
)

type socialOperationsBindingReader interface {
	ListBindingsByTarget(context.Context, uuid.UUID, uuid.UUID, connectorcore.TargetType, string, bindings.ListOptions) ([]connectorcore.TargetBinding, error)
}

type socialOperationsPreflightConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error)
}

type socialOperationsGrantReader interface {
	ListGrants(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]backgroundauth.Grant, error)
}

type socialOperationsDefinitionReader interface {
	Get(string) (catalog.Definition, bool)
}

type socialOperationsPrerequisiteReader interface {
	RestoreSocialOperationsPrerequisites(context.Context, socialops.Configuration) ([]socialops.PrerequisiteFinding, error)
}

// SocialOperationsPreflightRestorer builds a point-in-time, credential-free
// preflight snapshot from current server authority. It never consumes a grant.
type SocialOperationsPreflightRestorer struct {
	bindings      socialOperationsBindingReader
	connections   socialOperationsPreflightConnectionReader
	grants        socialOperationsGrantReader
	definitions   socialOperationsDefinitionReader
	prerequisites socialOperationsPrerequisiteReader
	now           func() time.Time
}

func NewSocialOperationsPreflightRestorer(bindingReader socialOperationsBindingReader, connectionReader socialOperationsPreflightConnectionReader, grantReader socialOperationsGrantReader, definitionReader socialOperationsDefinitionReader, prerequisiteReader socialOperationsPrerequisiteReader) (*SocialOperationsPreflightRestorer, error) {
	if bindingReader == nil || connectionReader == nil || grantReader == nil || definitionReader == nil || prerequisiteReader == nil {
		return nil, backgroundInternal("initialize social operations preflight", nil)
	}
	return &SocialOperationsPreflightRestorer{bindings: bindingReader, connections: connectionReader, grants: grantReader, definitions: definitionReader, prerequisites: prerequisiteReader, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (r *SocialOperationsPreflightRestorer) RestoreSocialOperationsPreflight(ctx context.Context, configuration socialops.Configuration) (socialops.PreflightInput, error) {
	input := socialops.PreflightInput{Scope: configuration.Scope, PublishedEmployeeVersionID: configuration.EmployeeVersionID, ConfigurationRevision: configuration.Revision}
	if r == nil || r.bindings == nil || r.connections == nil || r.grants == nil || r.definitions == nil || r.prerequisites == nil {
		return input, backgroundInternal("restore social operations preflight", nil)
	}
	if err := configuration.Validate(); err != nil {
		return input, err
	}
	prerequisites, err := r.prerequisites.RestoreSocialOperationsPrerequisites(ctx, configuration)
	if err != nil {
		return input, err
	}
	input.Prerequisites = append(prerequisites, socialOperationsBrandAssetsPrerequisite(configuration))
	allBindings, err := r.bindings.ListBindingsByTarget(ctx, configuration.Scope.OrganizationID, configuration.Scope.OwnerID,
		connectorcore.TargetTypeDigitalHuman, configuration.Scope.EmployeeID.String(), bindings.ListOptions{
			Scope: connectorcore.ScopePublished, VersionID: configuration.EmployeeVersionID.String(), Statuses: []connectorcore.BindingStatus{connectorcore.BindingStatusActive},
		})
	if err != nil {
		return input, err
	}
	grants, err := r.grants.ListGrants(ctx, configuration.Scope.OrganizationID, configuration.Scope.WorkspaceID, configuration.Scope.OwnerID)
	if err != nil {
		return input, err
	}
	research, err := r.restoreResearchPrerequisite(ctx, configuration, allBindings, grants)
	if err != nil {
		return input, err
	}
	input.Prerequisites = append(input.Prerequisites, research)

	type configuredConnection struct {
		connectorID  string
		connectionID uuid.UUID
		accountID    string
		handoff      *socialops.TeamHandoff
	}
	wanted := make([]configuredConnection, 0, len(configuration.Platforms)+1)
	for _, platform := range configuration.Platforms {
		wanted = append(wanted, configuredConnection{connectorID: platform.ConnectorID, connectionID: platform.ConnectionID, accountID: platform.AccountID})
	}
	handoff := configuration.TeamHandoff
	wanted = append(wanted, configuredConnection{connectorID: handoff.ConnectorID, connectionID: handoff.ConnectionID, handoff: &handoff})
	seen := map[string]bool{}
	for _, configured := range wanted {
		key := configured.connectorID + "/" + configured.connectionID.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		binding, ok := exactSocialOperationsBinding(allBindings, configuration, configured.connectorID, configured.connectionID)
		if !ok {
			continue
		}
		connection, err := r.connections.GetConnection(ctx, configuration.Scope.OrganizationID, configuration.Scope.OwnerID, configured.connectionID)
		if err != nil {
			continue
		}
		definition, ok := r.definitions.Get(configured.connectorID)
		if !ok || definition.ID != configured.connectorID || connection.ID != configured.connectionID ||
			connection.OrganizationID != configuration.Scope.OrganizationID || connection.OwnerAccountID != configuration.Scope.OwnerID ||
			string(connection.ConnectorID) != configured.connectorID || connection.State != connectorcore.ReadinessReady ||
			connection.Revision < 1 || connection.CredentialVersion < 1 {
			continue
		}
		digest, err := catalog.DefinitionContractDigest(definition)
		if err != nil {
			continue
		}
		available := socialOperationsAvailableOperations(definition, binding, connection, configuration.Scope.OwnerID, r.currentTime())
		fixedInputs := socialOperationsFixedInputs(configuration, configured, definition)
		granted, destinations := socialOperationsEligibleGrants(configuration, binding, connection, digest, available, fixedInputs, configured.handoff, grants, r.currentTime())
		accounts := socialOperationsKnownAccountIDs(connection)
		input.Connections = append(input.Connections, socialops.RestoredConnection{
			Scope: configuration.Scope, ConnectorID: configured.connectorID, ConnectionID: configured.connectionID,
			Definition: definition, AccountIDs: accounts, AvailableOperations: available, GrantedOperations: granted,
			KnownDestinations: destinations,
		})
	}
	return input, nil
}

func socialOperationsBrandAssetsPrerequisite(configuration socialops.Configuration) socialops.PrerequisiteFinding {
	if !socialOperationsImagesRequired(configuration) {
		return socialops.PrerequisiteFinding{Key: "brand_assets", Ready: true, Reason: "no platform requires image composition"}
	}
	if len(configuration.BrandArtifactRefs) > 0 {
		return socialops.PrerequisiteFinding{Key: "brand_assets", Ready: true, Reason: "validated brand image assets are configured"}
	}
	return socialops.PrerequisiteFinding{Key: "brand_assets", Reason: "required image composition has no configured brand image asset"}
}

func (r *SocialOperationsPreflightRestorer) restoreResearchPrerequisite(ctx context.Context, configuration socialops.Configuration, candidates []connectorcore.TargetBinding, grants []backgroundauth.Grant) (socialops.PrerequisiteFinding, error) {
	identity := configuration.Research.ConnectorID + "/" + configuration.Research.ConnectionID.String()
	finding := socialops.PrerequisiteFinding{Key: "research", Reason: "configured research connection " + identity + " is absent, unavailable, or lacks live bounded web.search and web.fetch grants"}
	now := r.currentTime()
	binding, ok := exactSocialOperationsBinding(candidates, configuration, configuration.Research.ConnectorID, configuration.Research.ConnectionID)
	if !ok {
		return finding, nil
	}
	connection, err := r.connections.GetConnection(ctx, configuration.Scope.OrganizationID, configuration.Scope.OwnerID, configuration.Research.ConnectionID)
	if err != nil {
		return finding, nil
	}
	definition, ok := r.definitions.Get(configuration.Research.ConnectorID)
	if !ok || definition.ID != configuration.Research.ConnectorID || connection.ID != configuration.Research.ConnectionID ||
		connection.OrganizationID != configuration.Scope.OrganizationID || connection.OwnerAccountID != configuration.Scope.OwnerID ||
		connection.ConnectorID.String() != configuration.Research.ConnectorID || connection.State != connectorcore.ReadinessReady ||
		connection.Revision < 1 || connection.CredentialVersion < 1 {
		return finding, nil
	}
	available := map[string]bool{}
	for _, operation := range socialOperationsAvailableOperations(definition, binding, connection, configuration.Scope.OwnerID, now) {
		available[operation] = true
	}
	if !available["web.search"] || !available["web.fetch"] {
		return finding, nil
	}
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		return finding, nil
	}
	granted := map[string]bool{}
	for _, grant := range grants {
		if grant.OrganizationID != configuration.Scope.OrganizationID || grant.WorkspaceID != configuration.Scope.WorkspaceID ||
			grant.OwnerID != configuration.Scope.OwnerID || grant.EmployeeID != configuration.Scope.EmployeeID ||
			grant.VersionID != configuration.EmployeeVersionID || grant.BindingID != binding.ID || grant.ConnectionID != connection.ID ||
			grant.BindingRevision != binding.Revision || grant.ConnectionRevision != connection.Revision ||
			grant.CredentialVersion != connection.CredentialVersion || grant.ContractDigest != digest || grant.RevokedAt != nil ||
			!grant.ExpiresAt.After(now) || grant.UsedInvocations >= grant.MaxInvocations {
			continue
		}
		var operations []string
		var constraints map[string]string
		if json.Unmarshal([]byte(grant.OperationsJSON), &operations) != nil || json.Unmarshal([]byte(grant.DestinationsJSON), &constraints) != nil || len(constraints) != 0 {
			continue
		}
		for _, operation := range operations {
			if operation == "web.search" || operation == "web.fetch" {
				granted[operation] = true
			}
		}
	}
	if granted["web.search"] && granted["web.fetch"] {
		finding.Ready, finding.Reason = true, "configured research connection "+identity+" has live bounded web.search and web.fetch grants"
	}
	return finding, nil
}

type socialOperationsDatabasePrerequisites struct {
	db     *gorm.DB
	models llmdefaultsvc.DefaultModelResolver
}

func (r socialOperationsDatabasePrerequisites) RestoreSocialOperationsPrerequisites(ctx context.Context, configuration socialops.Configuration) ([]socialops.PrerequisiteFinding, error) {
	findings := []socialops.PrerequisiteFinding{
		{Key: "default_model", Reason: "published employee default model is unavailable to the organization"},
		{Key: "image_review", Reason: "required image review has no available vision model"},
		{Key: "social_operations_skill", Reason: "published employee does not include the social-operations skill"},
		socialOperationsImageTextLanguagePrerequisite(configuration),
	}
	if r.db == nil || r.models == nil {
		return findings, backgroundInternal("restore social operations prerequisites", nil)
	}
	var version digitalhuman.Version
	err := r.db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND state = ?",
		configuration.EmployeeVersionID, configuration.Scope.EmployeeID, configuration.Scope.OrganizationID, configuration.Scope.WorkspaceID, digitalhuman.VersionStatePublished).Take(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return findings, nil
	}
	if err != nil {
		return nil, backgroundInternal("restore published social operations employee", err)
	}
	modelName := strings.TrimSpace(version.DefaultModel)
	if modelName != "" {
		resolved, resolveErr := r.models.ResolveModelType(ctx, configuration.Scope.OrganizationID.String(), nil, &modelName, sharedmodel.ModelTypeLLM)
		if resolveErr == nil && resolved != nil && strings.TrimSpace(resolved.Provider) != "" && strings.TrimSpace(resolved.Model) == modelName {
			findings[0].Ready = true
			findings[0].Reason = "published employee model is available: " + resolved.Provider + "/" + resolved.Model
		} else if resolveErr != nil && !errors.Is(resolveErr, llmdefaultsvc.ErrModelUnavailable) && !errors.Is(resolveErr, llmdefaultsvc.ErrDefaultModelNotFound) {
			return nil, backgroundInternal("resolve published social operations model", resolveErr)
		}
	}
	if !socialOperationsImagesRequired(configuration) {
		findings[1].Ready, findings[1].Reason = true, "no platform requires image review"
	} else {
		vision, visionErr := r.models.ResolveUseCase(ctx, configuration.Scope.OrganizationID.String(), llmmodel.UseCaseVision, nil, nil)
		if visionErr == nil && vision != nil && strings.TrimSpace(vision.Provider) != "" && strings.TrimSpace(vision.Model) != "" {
			findings[1].Ready = true
			findings[1].Reason = "organization vision model is available: " + vision.Provider + "/" + vision.Model
		} else if visionErr != nil && !errors.Is(visionErr, llmdefaultsvc.ErrModelUnavailable) && !errors.Is(visionErr, llmdefaultsvc.ErrDefaultModelNotFound) {
			return nil, backgroundInternal("resolve social operations image review model", visionErr)
		}
	}
	var skillCount int64
	err = r.db.WithContext(ctx).Model(&digitalhuman.ResourceBinding{}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND binding_type = ? AND resource_id = ? AND access_mode = ?",
		configuration.Scope.OrganizationID, configuration.Scope.WorkspaceID, configuration.Scope.EmployeeID, configuration.EmployeeVersionID,
		resourcebindings.BindingTypeSkill, "social-operations", resourcebindings.AccessModeExecute,
	).Count(&skillCount).Error
	if err != nil {
		return nil, backgroundInternal("restore social operations skill binding", err)
	}
	if skillCount == 1 {
		findings[2].Ready, findings[2].Reason = true, "published employee includes the social-operations skill"
	} else if skillCount > 1 {
		return nil, backgroundDenied(nil)
	}
	return findings, nil
}

func socialOperationsImagesRequired(configuration socialops.Configuration) bool {
	for _, platform := range configuration.Platforms {
		for _, capability := range platform.Capabilities {
			if capability.Capability == socialops.CapabilityImages && capability.Mode == socialops.RequirementRequired {
				return true
			}
		}
	}
	return false
}

func socialOperationsImageTextLanguagePrerequisite(configuration socialops.Configuration) socialops.PrerequisiteFinding {
	finding := socialops.PrerequisiteFinding{Key: "image_text_language", Reason: "fixed image font supports only English publication text"}
	if !socialOperationsImagesRequired(configuration) {
		finding.Ready, finding.Reason = true, "no platform requires composed image text"
		return finding
	}
	publication := strings.ToLower(strings.TrimSpace(configuration.PublicationLanguage))
	english := publication == "en" || strings.HasPrefix(publication, "en-") || publication == "english"
	if english {
		finding.Ready, finding.Reason = true, "configured image text languages are supported by the fixed font"
	}
	return finding
}

func socialOperationsKnownAccountIDs(connection connectorcore.PersonalConnection) []string {
	accounts := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		if value != "" && strings.TrimSpace(value) == value && len(value) <= 255 && !seen[value] {
			seen[value] = true
			accounts = append(accounts, value)
		}
	}
	evidence := connection.ProviderEvidence
	if connection.ConnectorID != "linkedin-social" {
		if connection.ExternalAccountID != nil {
			add(*connection.ExternalAccountID)
		}
		return accounts
	}
	if evidence == nil || evidence.Social == nil || connection.ExternalAccountID == nil {
		return accounts
	}
	social := evidence.Social
	if social.Version != 1 || social.Provider != connection.ConnectorID.String() || social.Identity != *connection.ExternalAccountID || social.CheckedAt.IsZero() || len(social.AccountIDs) > 50 {
		return accounts
	}
	evidenceSeen := map[string]bool{}
	for _, accountID := range social.AccountIDs {
		const prefix = "urn:li:organization:"
		if !strings.HasPrefix(accountID, prefix) || len(accountID) <= len(prefix) || len(accountID) > 255 {
			return accounts
		}
		for _, character := range accountID[len(prefix):] {
			if character < '0' || character > '9' {
				return accounts
			}
		}
		if evidenceSeen[accountID] {
			return accounts
		}
		evidenceSeen[accountID] = true
	}
	for _, accountID := range social.AccountIDs {
		add(accountID)
	}
	return accounts
}

func exactSocialOperationsBinding(candidates []connectorcore.TargetBinding, configuration socialops.Configuration, connectorID string, connectionID uuid.UUID) (connectorcore.TargetBinding, bool) {
	var match connectorcore.TargetBinding
	count := 0
	for _, binding := range candidates {
		if binding.OrganizationID == configuration.Scope.OrganizationID && binding.OwnerAccountID == configuration.Scope.OwnerID &&
			binding.ConnectionID == connectionID && string(binding.ConnectorID) == connectorID && binding.Status == connectorcore.BindingStatusActive &&
			binding.Target.Type == connectorcore.TargetTypeDigitalHuman && binding.Target.ID == configuration.Scope.EmployeeID.String() &&
			binding.Target.EffectiveScope() == connectorcore.ScopePublished && binding.Target.VersionID == configuration.EmployeeVersionID.String() && binding.Revision > 0 {
			match = binding
			count++
		}
	}
	return match, count == 1
}

func socialOperationsAvailableOperations(definition catalog.Definition, binding connectorcore.TargetBinding, connection connectorcore.PersonalConnection, owner uuid.UUID, now time.Time) []string {
	available := make([]string, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		if backgroundOperationsAllowed([]string{operation.ID}, definition, binding, connection, owner, now) {
			available = append(available, operation.ID)
		}
	}
	return available
}

func socialOperationsFixedInputs(configuration socialops.Configuration, configured struct {
	connectorID  string
	connectionID uuid.UUID
	accountID    string
	handoff      *socialops.TeamHandoff
}, definition catalog.Definition) map[string]map[string]any {
	inputs := map[string]map[string]any{}
	for _, operation := range definition.Operations {
		values := map[string]any{}
		if configured.accountID != "" {
			values["account_id"] = configured.accountID
		}
		if configured.handoff != nil && operation.ID == configured.handoff.OperationID {
			fixed, err := outboundInput(socialops.Outbound{Kind: "handoff", ConnectorID: configured.handoff.ConnectorID,
				OperationID: configured.handoff.OperationID, Destination: configured.handoff.Destination}, socialops.OutboundMessage{})
			if err == nil {
				values = fixed
			}
		}
		inputs[operation.ID] = values
	}
	return inputs
}

func fixedDestinationInput(operation catalog.Operation, destination string) (string, any, bool) {
	properties, ok := operation.InputSchema["properties"].(map[string]any)
	if !ok {
		return "", nil, false
	}
	for _, field := range []string{"to", "destination", "channel_id", "chat_id", "conversation_id", "recipient", "recipient_id"} {
		property, ok := properties[field].(map[string]any)
		if !ok {
			continue
		}
		switch property["type"] {
		case "string":
			return field, destination, true
		case "array":
			return field, []any{destination}, true
		}
	}
	return "", nil, false
}

func socialOperationsEligibleGrants(configuration socialops.Configuration, binding connectorcore.TargetBinding, connection connectorcore.PersonalConnection, digest string, available []string, fixedInputs map[string]map[string]any, handoff *socialops.TeamHandoff, grants []backgroundauth.Grant, now time.Time) ([]string, []string) {
	availableSet := make(map[string]bool, len(available))
	for _, operation := range available {
		availableSet[operation] = true
	}
	grantedSet := map[string]bool{}
	destinationSet := map[string]bool{}
	for _, grant := range grants {
		if grant.OrganizationID != configuration.Scope.OrganizationID || grant.WorkspaceID != configuration.Scope.WorkspaceID ||
			grant.OwnerID != configuration.Scope.OwnerID || grant.EmployeeID != configuration.Scope.EmployeeID ||
			grant.VersionID != configuration.EmployeeVersionID || grant.BindingID != binding.ID || grant.ConnectionID != connection.ID ||
			grant.BindingRevision != binding.Revision || grant.ConnectionRevision != connection.Revision ||
			grant.CredentialVersion != connection.CredentialVersion || grant.ContractDigest != digest || grant.RevokedAt != nil ||
			!grant.ExpiresAt.After(now) || grant.UsedInvocations >= grant.MaxInvocations {
			continue
		}
		var operations []string
		var constraints map[string]string
		if json.Unmarshal([]byte(grant.OperationsJSON), &operations) != nil || json.Unmarshal([]byte(grant.DestinationsJSON), &constraints) != nil {
			continue
		}
		for _, operation := range operations {
			if !availableSet[operation] || !fixedGrantConstraintsMatch(fixedInputs[operation], constraints) {
				continue
			}
			if handoff != nil && operation == handoff.OperationID {
				field, _, ok := fixedDestinationForOperation(fixedInputs[operation], handoff.Destination)
				if !ok || constraints[field] != handoff.Destination {
					continue
				}
				destinationSet[handoff.Destination] = true
			}
			grantedSet[operation] = true
		}
	}
	granted := make([]string, 0, len(grantedSet))
	for _, operation := range available {
		if grantedSet[operation] {
			granted = append(granted, operation)
		}
	}
	destinations := make([]string, 0, len(destinationSet))
	if handoff != nil && destinationSet[handoff.Destination] {
		destinations = append(destinations, handoff.Destination)
	}
	return granted, destinations
}

func fixedGrantConstraintsMatch(input map[string]any, constraints map[string]string) bool {
	for field, expected := range constraints {
		if !exactSocialOperationsDestination(input[field], expected) {
			return false
		}
	}
	return true
}

func exactSocialOperationsDestination(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected
	case []any:
		return len(typed) == 1 && typed[0] == expected
	case []string:
		return len(typed) == 1 && typed[0] == expected
	default:
		return false
	}
}

func fixedDestinationForOperation(input map[string]any, destination string) (string, any, bool) {
	for field, value := range input {
		if exactSocialOperationsDestination(value, destination) {
			return field, value, true
		}
	}
	return "", nil, false
}

func (r *SocialOperationsPreflightRestorer) currentTime() time.Time {
	if r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}

// SocialOperationsSetupService is the authenticated management boundary used
// by the console. Scope, owner, employee version and configuration revision are
// always restored or checked server-side.
type SocialOperationsSetupService struct {
	employees    backgroundEmployeeReader
	journal      *socialops.Service
	restorer     *SocialOperationsPreflightRestorer
	provisioning *socialops.ProvisioningService
}

func NewSocialOperationsSetupService(employees backgroundEmployeeReader, journal *socialops.Service, restorer *SocialOperationsPreflightRestorer, tasks socialops.ProvisioningTaskCreator) (*SocialOperationsSetupService, error) {
	if employees == nil || journal == nil || restorer == nil || tasks == nil {
		return nil, backgroundInternal("initialize social operations setup", nil)
	}
	provisioning, err := socialops.NewProvisioningService(journal, restorer, tasks)
	if err != nil {
		return nil, err
	}
	return &SocialOperationsSetupService{employees: employees, journal: journal, restorer: restorer, provisioning: provisioning}, nil
}

func (s *SocialOperationsSetupService) GetConfiguration(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID) (socialops.Configuration, socialops.Record, error) {
	configurationScope, _, err := s.restoreManagementScope(ctx, scope, employeeID)
	if err != nil {
		return socialops.Configuration{}, socialops.Record{}, err
	}
	return s.journal.LoadCurrentConfiguration(ctx, configurationScope)
}

func (s *SocialOperationsSetupService) SaveConfiguration(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID, expectedRevision int64, submitted socialops.Configuration) (socialops.Configuration, socialops.Record, error) {
	configurationScope, versionID, err := s.restoreManagementScope(ctx, scope, employeeID)
	if err != nil {
		return socialops.Configuration{}, socialops.Record{}, err
	}
	// Authority fields in the request are ignored. Only the authenticated scope,
	// current publication and journal CAS determine the stored configuration.
	submitted.Scope, submitted.EmployeeVersionID, submitted.Revision = configurationScope, versionID, 0
	validationCopy := submitted
	validationCopy.Revision = expectedRevision + 1
	if err = validationCopy.Validate(); err != nil {
		return socialops.Configuration{}, socialops.Record{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, err)
	}
	for _, source := range submitted.Sources {
		if source.ArtifactID != uuid.Nil {
			artifact, artifactErr := s.journal.GetArtifact(ctx, configurationScope, source.ArtifactID)
			if artifactErr != nil {
				return socialops.Configuration{}, socialops.Record{}, artifactErr
			}
			data, readErr := s.journal.ReadArtifact(ctx, configurationScope, source.ArtifactID)
			if readErr != nil {
				return socialops.Configuration{}, socialops.Record{}, readErr
			}
			if artifact.Size != int64(len(data)) {
				err = connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
				return socialops.Configuration{}, socialops.Record{}, err
			}
		}
	}
	for _, artifactID := range submitted.BrandArtifactRefs {
		artifact, artifactErr := s.journal.GetArtifact(ctx, configurationScope, artifactID)
		if artifactErr != nil {
			return socialops.Configuration{}, socialops.Record{}, artifactErr
		}
		data, readErr := s.journal.ReadArtifact(ctx, configurationScope, artifact.ID)
		if readErr != nil {
			return socialops.Configuration{}, socialops.Record{}, readErr
		}
		if artifact.Size != int64(len(data)) {
			return socialops.Configuration{}, socialops.Record{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
		}
		if _, imageErr := socialmcp.PrepareImage(data); imageErr != nil {
			return socialops.Configuration{}, socialops.Record{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, imageErr)
		}
	}
	return s.journal.SaveCurrentConfiguration(ctx, configurationScope, expectedRevision, submitted)
}

func (s *SocialOperationsSetupService) Preflight(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID) (socialops.PreflightReport, error) {
	configuration, _, err := s.GetConfiguration(ctx, scope, employeeID)
	if err != nil {
		return socialops.PreflightReport{}, err
	}
	input, err := s.restorer.RestoreSocialOperationsPreflight(ctx, configuration)
	if err != nil {
		return socialops.PreflightReport{}, err
	}
	return socialops.Preflight(configuration, input)
}

func (s *SocialOperationsSetupService) Setup(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID, expectedRevision int64, expectedVersion uuid.UUID) (socialops.ProvisioningResult, error) {
	configurationScope, versionID, err := s.restoreManagementScope(ctx, scope, employeeID)
	if err != nil {
		return socialops.ProvisioningResult{}, err
	}
	if expectedVersion == uuid.Nil || expectedVersion != versionID {
		return socialops.ProvisioningResult{}, socialops.ErrProvisioningStale
	}
	access := digitalhuman.TaskAccessContext{
		OrganizationID: configurationScope.OrganizationID, WorkspaceID: configurationScope.WorkspaceID, DigitalHumanID: configurationScope.EmployeeID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: configurationScope.OwnerID.String()},
		Owner:     digitalhuman.SharedTaskOwner(), Surface: digitalhuman.SurfaceConsole,
	}
	return s.provisioning.Provision(ctx, socialops.ProvisioningRequest{
		Scope: configurationScope, ExpectedEmployeeVersionID: expectedVersion, ExpectedConfigurationRevision: expectedRevision, Access: access,
	})
}

func (s *SocialOperationsSetupService) restoreManagementScope(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID) (socialops.Scope, uuid.UUID, error) {
	if s == nil || s.employees == nil || s.journal == nil || s.restorer == nil || s.provisioning == nil || employeeID == uuid.Nil ||
		scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.ActorID == uuid.Nil ||
		(scope.Surface != "" && scope.Surface != digitalhuman.SurfaceConsole) ||
		(scope.PrincipalType != "" && scope.PrincipalType != string(invocation.PrincipalAccount)) ||
		(scope.PrincipalID != uuid.Nil && scope.PrincipalID != scope.ActorID) {
		return socialops.Scope{}, uuid.Nil, backgroundDenied(nil)
	}
	detail, err := s.employees.Get(ctx, scope, employeeID)
	if err != nil {
		return socialops.Scope{}, uuid.Nil, err
	}
	if detail == nil || detail.Definition == nil || detail.Published == nil || detail.Definition.Status != digitalhuman.StatusActive ||
		detail.Definition.OrganizationID != scope.OrganizationID || detail.Definition.WorkspaceID != scope.WorkspaceID || detail.Definition.ID != employeeID ||
		detail.Definition.PublishedVersionID == nil || *detail.Definition.PublishedVersionID != detail.Published.ID ||
		detail.Published.DigitalHumanID != employeeID || detail.Published.OrganizationID != scope.OrganizationID ||
		detail.Published.WorkspaceID != scope.WorkspaceID || detail.Published.State != digitalhuman.VersionStatePublished {
		return socialops.Scope{}, uuid.Nil, backgroundDenied(errors.New("digital human is not currently published"))
	}
	return socialops.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, OwnerID: scope.ActorID, EmployeeID: employeeID}, detail.Published.ID, nil
}

func (c *ServiceContainer) GetSocialOperationsSetupService(tasks socialops.ProvisioningTaskCreator) (*SocialOperationsSetupService, error) {
	journal, err := c.GetSocialOperationsService()
	if err != nil {
		return nil, err
	}
	restorer, err := NewSocialOperationsPreflightRestorer(c.GetConnectorBindingRepository(), c.GetConnectorConnectionRepository(), c.GetConnectorBackgroundAuthorization(), c.GetConnectorPluginManager(), socialOperationsDatabasePrerequisites{db: c.db, models: c.GetDefaultModelService()})
	if err != nil {
		return nil, err
	}
	return NewSocialOperationsSetupService(c.GetDigitalHumanService(), journal, restorer, tasks)
}
