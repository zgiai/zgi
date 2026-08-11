package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type Registry struct {
	mu            sync.RWMutex
	registrations map[string]Registration
}

func NewRegistry() *Registry {
	return &Registry{registrations: make(map[string]Registration)}
}

func (r *Registry) Register(registration Registration) error {
	if r == nil {
		return fmt.Errorf("integration registry is required")
	}
	normalized, err := normalizeRegistration(registration)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.registrations[normalized.IntegrationID]; exists {
		return fmt.Errorf("integration %s is already registered", normalized.IntegrationID)
	}
	r.registrations[normalized.IntegrationID] = normalized
	return nil
}

func (r *Registry) OAuthProvider(integrationID, driverID string) (OAuth2Provider, bool) {
	registration, ok := r.Registration(integrationID)
	if !ok || registration.OAuth2Provider == nil {
		return nil, false
	}
	if normalizedDriverID := strings.ToLower(strings.TrimSpace(driverID)); normalizedDriverID != "" &&
		!strings.EqualFold(registration.Definition.DriverID, normalizedDriverID) {
		return nil, false
	}
	return registration.OAuth2Provider, true
}

func normalizeRegistration(registration Registration) (Registration, error) {
	legacyDefinition := providerDefinitionEmpty(registration.Definition)
	integrationID := strings.ToLower(strings.TrimSpace(registration.Definition.ID))
	legacyIntegrationID := strings.ToLower(strings.TrimSpace(registration.IntegrationID))
	if integrationID == "" {
		integrationID = legacyIntegrationID
	} else if legacyIntegrationID != "" && legacyIntegrationID != integrationID {
		return Registration{}, fmt.Errorf("integration definition id %s does not match registration id %s", integrationID, legacyIntegrationID)
	}
	if integrationID == "" {
		return Registration{}, fmt.Errorf("integration id is required")
	}
	if !integrationIdentifierPattern.MatchString(integrationID) {
		return Registration{}, fmt.Errorf("integration id %s is invalid", integrationID)
	}
	if registration.Adapter == nil || strings.TrimSpace(registration.Adapter.DriverID()) == "" {
		return Registration{}, fmt.Errorf("integration %s adapter is required", integrationID)
	}
	driverID := strings.ToLower(strings.TrimSpace(registration.Adapter.DriverID()))
	if !integrationIdentifierPattern.MatchString(driverID) {
		return Registration{}, fmt.Errorf("integration %s adapter driver id is invalid", integrationID)
	}

	definition := cloneProviderDefinition(registration.Definition)
	if legacyDefinition {
		definition = defaultProviderDefinition(integrationID, driverID, registration.Actions)
	} else {
		definition.ID = integrationID
		definedDriverID := strings.ToLower(strings.TrimSpace(definition.DriverID))
		if definedDriverID == "" {
			definition.DriverID = driverID
		} else if definedDriverID != driverID {
			return Registration{}, fmt.Errorf("integration %s definition driver %s does not match adapter driver %s", integrationID, definedDriverID, driverID)
		}
		if len(definition.Actions) == 0 {
			definition.Actions = cloneActions(registration.Actions)
		} else if len(registration.Actions) > 0 && !sameActionIdentities(definition.Actions, registration.Actions) {
			return Registration{}, fmt.Errorf("integration %s definition actions do not match registration actions", integrationID)
		}
	}
	if len(definition.Actions) == 0 {
		return Registration{}, fmt.Errorf("integration %s must register at least one action", integrationID)
	}

	normalizedDefinition, err := normalizeProviderDefinition(definition)
	if err != nil {
		return Registration{}, err
	}
	registration.Definition = normalizedDefinition
	registration.IntegrationID = normalizedDefinition.ID
	registration.Actions = cloneActions(normalizedDefinition.Actions)
	if registration.ConnectionTester == nil {
		registration.ConnectionTester, _ = registration.Adapter.(ConnectionTester)
	}
	if registration.CredentialValidator == nil {
		registration.CredentialValidator, _ = registration.Adapter.(CredentialValidator)
	}
	if registration.HealthProbe == nil {
		registration.HealthProbe, _ = registration.Adapter.(HealthProbe)
	}
	if registration.OAuth2Provider == nil {
		registration.OAuth2Provider, _ = registration.Adapter.(OAuth2Provider)
	}
	for _, method := range registration.Definition.AuthMethods {
		if method.Available && method.Type == AuthMethodTypeOAuth2 && registration.OAuth2Provider == nil {
			return Registration{}, fmt.Errorf("integration %s available OAuth method %s requires an OAuth provider", integrationID, method.ID)
		}
	}
	return registration, nil
}

func normalizeProviderDefinition(definition ProviderDefinition) (ProviderDefinition, error) {
	var err error
	definition.ID = strings.ToLower(strings.TrimSpace(definition.ID))
	definition.DriverID = strings.ToLower(strings.TrimSpace(definition.DriverID))
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.Author = strings.TrimSpace(definition.Author)
	definition.Icon = strings.TrimSpace(definition.Icon)
	definition.DocumentationURL = strings.TrimSpace(definition.DocumentationURL)
	if !integrationIdentifierPattern.MatchString(definition.ID) || !integrationIdentifierPattern.MatchString(definition.DriverID) {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider id and driver id are invalid", definition.ID)
	}
	if definition.Name == "" || len([]rune(definition.Name)) > 128 {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider name is required and must not exceed 128 characters", definition.ID)
	}
	if len([]rune(definition.Description)) > 2000 || len([]rune(definition.Author)) > 128 || len(definition.Icon) > 128 {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider metadata is too large", definition.ID)
	}
	definition.NameI18n, err = normalizeLocalizedText(definition.NameI18n, definition.Name, 128)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider localized name: %w", definition.ID, err)
	}
	definition.DescriptionI18n, err = normalizeLocalizedText(definition.DescriptionI18n, definition.Description, 2000)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider localized description: %w", definition.ID, err)
	}
	definition.DocumentationURLI18n, err = normalizeLocalizedText(definition.DocumentationURLI18n, definition.DocumentationURL, 2048)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider localized documentation url: %w", definition.ID, err)
	}
	if definition.Author == "" {
		definition.Author = "ZGI"
	}
	if definition.Icon == "" {
		definition.Icon = "plug"
	}
	if definition.DocumentationURL != "" {
		parsed, err := url.Parse(definition.DocumentationURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			return ProviderDefinition{}, fmt.Errorf("integration %s documentation url is invalid", definition.ID)
		}
	}
	for locale, documentationURL := range definition.DocumentationURLI18n {
		parsed, parseErr := url.Parse(documentationURL)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			return ProviderDefinition{}, fmt.Errorf("integration %s documentation url for locale %s is invalid", definition.ID, locale)
		}
	}
	definition.Tags = normalizeCatalogStringList(definition.Tags, 32)
	definition.Categories = normalizeCatalogStringList(definition.Categories, 16)
	definition.TagLabelsI18n, err = normalizeLocalizedLabelMap(definition.TagLabelsI18n, definition.Tags, 32, 128)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider localized tag labels: %w", definition.ID, err)
	}
	definition.CategoryLabelsI18n, err = normalizeLocalizedLabelMap(definition.CategoryLabelsI18n, definition.Categories, 16, 128)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s provider localized category labels: %w", definition.ID, err)
	}
	definition.Scopes, err = normalizeProviderScopeDefinitions(definition.ID, definition.Scopes)
	if err != nil {
		return ProviderDefinition{}, err
	}
	declaredScopes := make(map[string]ProviderScopeDefinition, len(definition.Scopes))
	for _, scope := range definition.Scopes {
		declaredScopes[scope.ID] = scope
	}
	if len(definition.AuthMethods) == 0 {
		return ProviderDefinition{}, fmt.Errorf("integration %s must declare at least one auth method", definition.ID)
	}
	seenAuthMethods := make(map[string]struct{}, len(definition.AuthMethods))
	availableAuthMethods := make(map[string]struct{}, len(definition.AuthMethods))
	for index := range definition.AuthMethods {
		method, err := normalizeAuthMethod(definition.ID, definition.AuthMethods[index])
		if err != nil {
			return ProviderDefinition{}, err
		}
		if _, duplicated := seenAuthMethods[method.ID]; duplicated {
			return ProviderDefinition{}, fmt.Errorf("integration %s auth method %s is duplicated", definition.ID, method.ID)
		}
		seenAuthMethods[method.ID] = struct{}{}
		if method.Available {
			availableAuthMethods[method.ID] = struct{}{}
		}
		if method.OAuth != nil {
			for _, scopeID := range method.OAuth.IdentityScopes {
				scope, declared := declaredScopes[scopeID]
				if !declared {
					return ProviderDefinition{}, fmt.Errorf(
						"integration %s OAuth method %s references undeclared identity scope %s",
						definition.ID,
						method.ID,
						scopeID,
					)
				}
				if scope.Category == ProviderScopeCategoryInternal {
					return ProviderDefinition{}, fmt.Errorf(
						"integration %s OAuth method %s cannot request internal identity scope %s",
						definition.ID,
						method.ID,
						scopeID,
					)
				}
			}
		}
		definition.AuthMethods[index] = method
	}
	sort.Slice(definition.AuthMethods, func(i, j int) bool { return definition.AuthMethods[i].ID < definition.AuthMethods[j].ID })
	if definition.HealthProbe.MayIncurCost && !definition.HealthProbe.Supported {
		return ProviderDefinition{}, fmt.Errorf("integration %s health probe cannot incur cost when unsupported", definition.ID)
	}
	definition.HealthProbe.Description = strings.TrimSpace(definition.HealthProbe.Description)
	if len([]rune(definition.HealthProbe.Description)) > 1000 {
		return ProviderDefinition{}, fmt.Errorf("integration %s health probe description is too large", definition.ID)
	}
	definition.HealthProbe.DescriptionI18n, err = normalizeLocalizedText(
		definition.HealthProbe.DescriptionI18n,
		definition.HealthProbe.Description,
		1000,
	)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s health probe localized description: %w", definition.ID, err)
	}

	seenIDs := make(map[string]struct{}, len(definition.Actions))
	seenTools := make(map[string]struct{}, len(definition.Actions))
	for index := range definition.Actions {
		action, err := normalizeActionDefinition(definition.ID, definition.Actions[index])
		if err != nil {
			return ProviderDefinition{}, err
		}
		if _, exists := seenIDs[action.ID]; exists {
			return ProviderDefinition{}, fmt.Errorf("integration %s action %s is duplicated", definition.ID, action.ID)
		}
		if _, exists := seenTools[action.ToolName]; exists {
			return ProviderDefinition{}, fmt.Errorf("integration %s tool %s is duplicated", definition.ID, action.ToolName)
		}
		if len(definition.AuthMethods) > 1 && len(action.SupportedAuthMethodIDs) == 0 {
			return ProviderDefinition{}, fmt.Errorf(
				"integration %s action %s must explicitly declare supported auth methods",
				definition.ID,
				action.ID,
			)
		}
		for _, scopeID := range ActionRequiredScopeIDs(action) {
			if _, declared := declaredScopes[scopeID]; !declared {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s action %s references undeclared scope %s",
					definition.ID,
					action.ID,
					scopeID,
				)
			}
		}
		for _, authMethodID := range action.SupportedAuthMethodIDs {
			if _, exists := seenAuthMethods[authMethodID]; !exists {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s action %s references unknown auth method %s",
					definition.ID,
					action.ID,
					authMethodID,
				)
			}
			if _, available := availableAuthMethods[authMethodID]; !available {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s action %s references unavailable auth method %s",
					definition.ID,
					action.ID,
					authMethodID,
				)
			}
		}
		seenIDs[action.ID] = struct{}{}
		seenTools[action.ToolName] = struct{}{}
		definition.Actions[index] = action
	}
	sort.Slice(definition.Actions, func(i, j int) bool { return definition.Actions[i].ID < definition.Actions[j].ID })
	actionsByID := make(map[string]ActionDefinition, len(definition.Actions))
	for _, action := range definition.Actions {
		actionsByID[action.ID] = action
	}
	for _, action := range definition.Actions {
		for _, hint := range action.PreparationHints {
			preparation, exists := actionsByID[hint.ActionID]
			if !exists {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s action %s references unknown preparation action %s",
					definition.ID,
					action.ID,
					hint.ActionID,
				)
			}
			if preparation.Effect != toolgovernance.EffectRead {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s action %s preparation action %s must be read-only",
					definition.ID,
					action.ID,
					hint.ActionID,
				)
			}
			for _, resultPath := range hint.ResultPaths {
				if !actionPreparationResultPathExists(preparation.OutputSchema, resultPath) {
					return ProviderDefinition{}, fmt.Errorf(
						"integration %s action %s preparation action %s has no output at %s",
						definition.ID,
						action.ID,
						hint.ActionID,
						resultPath,
					)
				}
			}
		}
	}
	for _, method := range definition.AuthMethods {
		if method.OAuth == nil {
			continue
		}
		for _, actionID := range method.OAuth.DefaultActionIDs {
			action, exists := actionsByID[actionID]
			if !exists {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s OAuth method %s references unknown default action %s",
					definition.ID,
					method.ID,
					actionID,
				)
			}
			if !ActionSupportsAuthMethod(action, method.ID) {
				return ProviderDefinition{}, fmt.Errorf(
					"integration %s OAuth method %s cannot use default action %s",
					definition.ID,
					method.ID,
					actionID,
				)
			}
		}
	}
	if err := validateProviderLocalizationContract(definition); err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s localization contract: %w", definition.ID, err)
	}

	declaredCatalogRevision := strings.ToLower(strings.TrimSpace(definition.CatalogRevision))
	definition.CatalogRevision = ""
	computedRevision, err := hashCatalogDefinition(definition)
	if err != nil {
		return ProviderDefinition{}, fmt.Errorf("integration %s catalog revision: %w", definition.ID, err)
	}
	if declaredCatalogRevision != "" && declaredCatalogRevision != computedRevision {
		return ProviderDefinition{}, fmt.Errorf("integration %s catalog revision does not match its definition", definition.ID)
	}
	definition.CatalogRevision = computedRevision
	for index := range definition.Actions {
		declaredActionRevision := strings.ToLower(strings.TrimSpace(definition.Actions[index].CatalogRevision))
		if declaredActionRevision != "" && declaredActionRevision != computedRevision {
			return ProviderDefinition{}, fmt.Errorf("integration %s action %s catalog revision does not match its provider", definition.ID, definition.Actions[index].ID)
		}
		definition.Actions[index].CatalogRevision = computedRevision
	}
	return definition, nil
}

func normalizeAuthMethod(integrationID string, method AuthMethodDefinition) (AuthMethodDefinition, error) {
	var err error
	method.ID = strings.ToLower(strings.TrimSpace(method.ID))
	method.Type = AuthMethodType(strings.ToLower(strings.TrimSpace(string(method.Type))))
	method.CredentialSource = ConnectionCredentialSource(strings.ToLower(strings.TrimSpace(string(method.CredentialSource))))
	method.IdentityKind = AuthIdentityKind(strings.ToLower(strings.TrimSpace(string(method.IdentityKind))))
	method.AcquisitionStrategy = AuthAcquisitionStrategy(strings.ToLower(strings.TrimSpace(string(method.AcquisitionStrategy))))
	method.LifecycleStrategy = AuthLifecycleStrategy(strings.ToLower(strings.TrimSpace(string(method.LifecycleStrategy))))
	method.RequestAuthStrategy = RequestAuthStrategy(strings.ToLower(strings.TrimSpace(string(method.RequestAuthStrategy))))
	method.ScopeEvidence = AuthScopeEvidence(strings.ToLower(strings.TrimSpace(string(method.ScopeEvidence))))
	method.Label = strings.TrimSpace(method.Label)
	method.Description = strings.TrimSpace(method.Description)
	if !integrationIdentifierPattern.MatchString(method.ID) || !validAuthMethodType(method.Type) || method.Label == "" {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method id, type, and label are required and must be valid", integrationID)
	}
	if len([]rune(method.Label)) > 128 || len([]rune(method.Description)) > 1000 {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s metadata is too large", integrationID, method.ID)
	}
	method.LabelI18n, err = normalizeLocalizedText(method.LabelI18n, method.Label, 128)
	if err != nil {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s localized label: %w", integrationID, method.ID, err)
	}
	method.DescriptionI18n, err = normalizeLocalizedText(method.DescriptionI18n, method.Description, 1000)
	if err != nil {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s localized description: %w", integrationID, method.ID, err)
	}
	applyAuthStrategyDefaults(&method)
	if method.ScopeEvidence == "" {
		method.ScopeEvidence = AuthScopeEvidenceProviderReported
	}
	if !validAuthScopeEvidence(method.ScopeEvidence) {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s has invalid scope evidence metadata", integrationID, method.ID)
	}
	if err := validateAuthStrategies(integrationID, method); err != nil {
		return AuthMethodDefinition{}, err
	}
	method.SetupGuide, err = normalizeAuthSetupGuide(integrationID, method)
	if err != nil {
		return AuthMethodDefinition{}, err
	}
	if method.Type == AuthMethodTypeNone {
		// Preserve the declaration for forward compatibility, but do not
		// advertise a connection flow that the current credential-backed
		// runtime cannot persist or resolve.
		method.Available = false
	}
	switch method.Type {
	case AuthMethodTypeNone:
		if method.CredentialSource != "" && method.CredentialSource != ConnectionCredentialSourceOrganization {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s no-auth method %s has an invalid credential source", integrationID, method.ID)
		}
		method.CredentialSource = ConnectionCredentialSourceOrganization
	default:
		if method.CredentialSource != ConnectionCredentialSourceOrganization && method.CredentialSource != ConnectionCredentialSourceAccount {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s must use organization or account credentials", integrationID, method.ID)
		}
	}
	if method.Type == AuthMethodTypeNone && len(method.Fields) > 0 {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s cannot declare credential fields", integrationID, method.ID)
	}
	if method.Type != AuthMethodTypeOAuth2 && method.OAuth != nil {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s non-OAuth method %s cannot declare OAuth metadata", integrationID, method.ID)
	}
	if method.Type == AuthMethodTypeOAuth2 {
		if len(method.Fields) > 0 {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s OAuth method %s cannot expose credential fields", integrationID, method.ID)
		}
		if method.Available && method.OAuth == nil {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s available OAuth method %s must declare OAuth metadata", integrationID, method.ID)
		}
		if method.OAuth != nil {
			metadata := *method.OAuth
			metadata.ClientConfigID = normalizeOAuthIdentifier(metadata.ClientConfigID)
			if metadata.ClientConfigID == "" {
				metadata.ClientConfigID = method.ID
			}
			if !integrationIdentifierPattern.MatchString(metadata.ClientConfigID) {
				return AuthMethodDefinition{}, fmt.Errorf("integration %s OAuth method %s client config id is invalid", integrationID, method.ID)
			}
			metadata.ProviderSetupURL = strings.TrimSpace(metadata.ProviderSetupURL)
			if metadata.ProviderSetupURL != "" {
				setupURL, parseErr := url.Parse(metadata.ProviderSetupURL)
				if parseErr != nil || !strings.EqualFold(setupURL.Scheme, "https") ||
					setupURL.Host == "" || setupURL.User != nil || setupURL.Fragment != "" {
					return AuthMethodDefinition{}, fmt.Errorf("integration %s OAuth method %s provider setup URL is invalid", integrationID, method.ID)
				}
			}
			metadata.IdentityScopes = normalizeScopes(metadata.IdentityScopes)
			metadata.DefaultActionIDs = normalizeCatalogStringList(metadata.DefaultActionIDs, 64)
			metadata.ClientFields, err = normalizeOAuthClientFields(integrationID, method.ID, metadata.ClientFields)
			if err != nil {
				return AuthMethodDefinition{}, err
			}
			method.OAuth = &metadata
		}
	}
	seenFields := make(map[string]struct{}, len(method.Fields))
	for index := range method.Fields {
		field := method.Fields[index]
		field.Key = strings.ToLower(strings.TrimSpace(field.Key))
		field.Label = strings.TrimSpace(field.Label)
		field.Description = strings.TrimSpace(field.Description)
		field.Placeholder = strings.TrimSpace(field.Placeholder)
		field.Input = CredentialFieldInput(strings.ToLower(strings.TrimSpace(string(field.Input))))
		if !integrationIdentifierPattern.MatchString(field.Key) || field.Label == "" || !validCredentialFieldInput(field.Input) {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field is invalid", integrationID, method.ID)
		}
		if _, duplicated := seenFields[field.Key]; duplicated {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s is duplicated", integrationID, method.ID, field.Key)
		}
		if len([]rune(field.Label)) > 128 || len([]rune(field.Description)) > 1000 || len([]rune(field.Placeholder)) > 256 {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s metadata is too large", integrationID, method.ID, field.Key)
		}
		field.LabelI18n, err = normalizeLocalizedText(field.LabelI18n, field.Label, 128)
		if err != nil {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s localized label: %w", integrationID, method.ID, field.Key, err)
		}
		field.DescriptionI18n, err = normalizeLocalizedText(field.DescriptionI18n, field.Description, 1000)
		if err != nil {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s localized description: %w", integrationID, method.ID, field.Key, err)
		}
		field.PlaceholderI18n, err = normalizeLocalizedText(field.PlaceholderI18n, field.Placeholder, 256)
		if err != nil {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s localized placeholder: %w", integrationID, method.ID, field.Key, err)
		}
		if field.Input == CredentialFieldInputSelect && len(field.Options) == 0 {
			return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s requires options", integrationID, method.ID, field.Key)
		}
		seenOptions := make(map[string]struct{}, len(field.Options))
		for optionIndex := range field.Options {
			option := field.Options[optionIndex]
			option.Value = strings.TrimSpace(option.Value)
			option.Label = strings.TrimSpace(option.Label)
			if option.Value == "" || option.Label == "" || len(option.Value) > 256 || len([]rune(option.Label)) > 128 {
				return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s option is invalid", integrationID, method.ID, field.Key)
			}
			option.LabelI18n, err = normalizeLocalizedText(option.LabelI18n, option.Label, 128)
			if err != nil {
				return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s option localized label: %w", integrationID, method.ID, field.Key, err)
			}
			if _, duplicated := seenOptions[option.Value]; duplicated {
				return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s credential field %s option %s is duplicated", integrationID, method.ID, field.Key, option.Value)
			}
			seenOptions[option.Value] = struct{}{}
			field.Options[optionIndex] = option
		}
		seenFields[field.Key] = struct{}{}
		method.Fields[index] = field
	}
	if method.Available && method.Type != AuthMethodTypeNone && method.Type != AuthMethodTypeOAuth2 && len(method.Fields) == 0 {
		return AuthMethodDefinition{}, fmt.Errorf("integration %s auth method %s must declare credential fields", integrationID, method.ID)
	}
	return method, nil
}

func normalizeAuthSetupGuide(integrationID string, method AuthMethodDefinition) (*AuthSetupGuideDefinition, error) {
	if method.SetupGuide == nil {
		return nil, nil
	}

	guide := *method.SetupGuide
	guide.ConsoleURL = strings.TrimSpace(guide.ConsoleURL)
	guide.DocumentationURL = strings.TrimSpace(guide.DocumentationURL)
	if err := validateAuthSetupURL(guide.ConsoleURL); err != nil {
		return nil, fmt.Errorf("integration %s auth method %s setup console URL: %w", integrationID, method.ID, err)
	}
	if err := validateAuthSetupURL(guide.DocumentationURL); err != nil {
		return nil, fmt.Errorf("integration %s auth method %s setup documentation URL: %w", integrationID, method.ID, err)
	}
	if len(guide.Steps) == 0 || len(guide.Steps) > 8 {
		return nil, fmt.Errorf("integration %s auth method %s setup guide must declare between 1 and 8 steps", integrationID, method.ID)
	}
	if len(guide.Notices) > 8 {
		return nil, fmt.Errorf("integration %s auth method %s setup guide declares too many notices", integrationID, method.ID)
	}

	seenStepIDs := make(map[string]struct{}, len(guide.Steps))
	for index := range guide.Steps {
		step := guide.Steps[index]
		step.ID = strings.ToLower(strings.TrimSpace(step.ID))
		step.Title = strings.TrimSpace(step.Title)
		step.Description = strings.TrimSpace(step.Description)
		step.Action = AuthSetupStepAction(strings.ToLower(strings.TrimSpace(string(step.Action))))
		if !integrationIdentifierPattern.MatchString(step.ID) || step.Title == "" {
			return nil, fmt.Errorf("integration %s auth method %s setup step id and title are required and must be valid", integrationID, method.ID)
		}
		if _, duplicated := seenStepIDs[step.ID]; duplicated {
			return nil, fmt.Errorf("integration %s auth method %s setup step %s is duplicated", integrationID, method.ID, step.ID)
		}
		if len([]rune(step.Title)) > 128 || len([]rune(step.Description)) > 1000 {
			return nil, fmt.Errorf("integration %s auth method %s setup step %s metadata is too large", integrationID, method.ID, step.ID)
		}
		var err error
		step.TitleI18n, err = normalizeLocalizedText(step.TitleI18n, step.Title, 128)
		if err != nil {
			return nil, fmt.Errorf("integration %s auth method %s setup step %s localized title: %w", integrationID, method.ID, step.ID, err)
		}
		step.DescriptionI18n, err = normalizeLocalizedText(step.DescriptionI18n, step.Description, 1000)
		if err != nil {
			return nil, fmt.Errorf("integration %s auth method %s setup step %s localized description: %w", integrationID, method.ID, step.ID, err)
		}
		switch step.Action {
		case AuthSetupStepActionNone:
		case AuthSetupStepActionOpenConsole:
			if guide.ConsoleURL == "" {
				return nil, fmt.Errorf("integration %s auth method %s setup step %s requires a console URL", integrationID, method.ID, step.ID)
			}
		case AuthSetupStepActionOpenDocumentation:
			if guide.DocumentationURL == "" {
				return nil, fmt.Errorf("integration %s auth method %s setup step %s requires a documentation URL", integrationID, method.ID, step.ID)
			}
		case AuthSetupStepActionCopyCallbackURL:
			if method.Type != AuthMethodTypeOAuth2 {
				return nil, fmt.Errorf("integration %s non-browser method %s cannot declare a callback setup step", integrationID, method.ID)
			}
		default:
			return nil, fmt.Errorf("integration %s auth method %s setup step %s action is invalid", integrationID, method.ID, step.ID)
		}
		seenStepIDs[step.ID] = struct{}{}
		guide.Steps[index] = step
	}

	seenNoticeIDs := make(map[string]struct{}, len(guide.Notices))
	for index := range guide.Notices {
		notice := guide.Notices[index]
		notice.ID = strings.ToLower(strings.TrimSpace(notice.ID))
		notice.Level = AuthSetupNoticeLevel(strings.ToLower(strings.TrimSpace(string(notice.Level))))
		notice.Text = strings.TrimSpace(notice.Text)
		if notice.Level == "" {
			notice.Level = AuthSetupNoticeLevelInfo
		}
		if !integrationIdentifierPattern.MatchString(notice.ID) || notice.Text == "" {
			return nil, fmt.Errorf("integration %s auth method %s setup notice id and text are required and must be valid", integrationID, method.ID)
		}
		if notice.Level != AuthSetupNoticeLevelInfo && notice.Level != AuthSetupNoticeLevelWarning {
			return nil, fmt.Errorf("integration %s auth method %s setup notice %s level is invalid", integrationID, method.ID, notice.ID)
		}
		if _, duplicated := seenNoticeIDs[notice.ID]; duplicated {
			return nil, fmt.Errorf("integration %s auth method %s setup notice %s is duplicated", integrationID, method.ID, notice.ID)
		}
		if len([]rune(notice.Text)) > 1000 {
			return nil, fmt.Errorf("integration %s auth method %s setup notice %s is too large", integrationID, method.ID, notice.ID)
		}
		var err error
		notice.TextI18n, err = normalizeLocalizedText(notice.TextI18n, notice.Text, 1000)
		if err != nil {
			return nil, fmt.Errorf("integration %s auth method %s setup notice %s localized text: %w", integrationID, method.ID, notice.ID, err)
		}
		seenNoticeIDs[notice.ID] = struct{}{}
		guide.Notices[index] = notice
	}

	return &guide, nil
}

func validateAuthSetupURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("must be an absolute HTTPS URL without credentials or fragments")
	}
	return nil
}

func applyAuthStrategyDefaults(method *AuthMethodDefinition) {
	if method == nil {
		return
	}
	switch method.Type {
	case AuthMethodTypeOAuth2:
		if method.IdentityKind == "" {
			method.IdentityKind = AuthIdentityKindUser
		}
		if method.AcquisitionStrategy == "" {
			method.AcquisitionStrategy = AuthAcquisitionStrategyBrowserRedirect
		}
		if method.LifecycleStrategy == "" {
			method.LifecycleStrategy = AuthLifecycleStrategyOAuthRefresh
		}
		if method.RequestAuthStrategy == "" {
			method.RequestAuthStrategy = RequestAuthStrategyBearerHeader
		}
	case AuthMethodTypeAPIKey:
		if method.IdentityKind == "" {
			method.IdentityKind = AuthIdentityKindApplication
		}
		if method.AcquisitionStrategy == "" {
			method.AcquisitionStrategy = AuthAcquisitionStrategyManualForm
		}
		if method.LifecycleStrategy == "" {
			method.LifecycleStrategy = AuthLifecycleStrategyStatic
		}
		if method.RequestAuthStrategy == "" {
			method.RequestAuthStrategy = RequestAuthStrategyAPIKeyHeader
		}
	case AuthMethodTypeServiceAccount:
		if method.IdentityKind == "" {
			method.IdentityKind = AuthIdentityKindService
		}
		if method.AcquisitionStrategy == "" {
			method.AcquisitionStrategy = AuthAcquisitionStrategyManualForm
		}
		if method.LifecycleStrategy == "" {
			method.LifecycleStrategy = AuthLifecycleStrategyExchangeOnDemand
		}
		if method.RequestAuthStrategy == "" {
			method.RequestAuthStrategy = RequestAuthStrategyProviderCustom
		}
	case AuthMethodTypeNone:
		if method.IdentityKind == "" {
			method.IdentityKind = AuthIdentityKindApplication
		}
		if method.AcquisitionStrategy == "" {
			method.AcquisitionStrategy = AuthAcquisitionStrategyNone
		}
		if method.LifecycleStrategy == "" {
			method.LifecycleStrategy = AuthLifecycleStrategyStatic
		}
		if method.RequestAuthStrategy == "" {
			method.RequestAuthStrategy = RequestAuthStrategyNone
		}
	default:
		if method.IdentityKind == "" {
			method.IdentityKind = AuthIdentityKindApplication
		}
		if method.AcquisitionStrategy == "" {
			method.AcquisitionStrategy = AuthAcquisitionStrategyManualForm
		}
		if method.LifecycleStrategy == "" {
			method.LifecycleStrategy = AuthLifecycleStrategyStatic
		}
		if method.RequestAuthStrategy == "" {
			method.RequestAuthStrategy = RequestAuthStrategyProviderCustom
		}
	}
}

func validateAuthStrategies(integrationID string, method AuthMethodDefinition) error {
	if !validAuthIdentityKind(method.IdentityKind) ||
		!validAuthAcquisitionStrategy(method.AcquisitionStrategy) ||
		!validAuthLifecycleStrategy(method.LifecycleStrategy) ||
		!validRequestAuthStrategy(method.RequestAuthStrategy) {
		return fmt.Errorf("integration %s auth method %s has invalid auth strategy metadata", integrationID, method.ID)
	}
	if method.Type == AuthMethodTypeNone {
		if method.AcquisitionStrategy != AuthAcquisitionStrategyNone ||
			method.LifecycleStrategy != AuthLifecycleStrategyStatic ||
			method.RequestAuthStrategy != RequestAuthStrategyNone {
			return fmt.Errorf("integration %s no-auth method %s has incompatible auth strategies", integrationID, method.ID)
		}
		return nil
	}
	if method.AcquisitionStrategy == AuthAcquisitionStrategyNone || method.RequestAuthStrategy == RequestAuthStrategyNone {
		return fmt.Errorf("integration %s auth method %s has incompatible empty auth strategies", integrationID, method.ID)
	}
	if method.Type == AuthMethodTypeOAuth2 {
		if method.AcquisitionStrategy != AuthAcquisitionStrategyBrowserRedirect ||
			method.LifecycleStrategy != AuthLifecycleStrategyOAuthRefresh ||
			(method.RequestAuthStrategy != RequestAuthStrategyBearerHeader &&
				method.RequestAuthStrategy != RequestAuthStrategyProviderCustom) {
			return fmt.Errorf("integration %s OAuth method %s has incompatible auth strategies", integrationID, method.ID)
		}
	} else if method.AcquisitionStrategy == AuthAcquisitionStrategyBrowserRedirect ||
		method.LifecycleStrategy == AuthLifecycleStrategyOAuthRefresh {
		return fmt.Errorf("integration %s non-OAuth method %s has OAuth-only auth strategies", integrationID, method.ID)
	}
	if method.LifecycleStrategy == AuthLifecycleStrategySignedRequest &&
		method.RequestAuthStrategy != RequestAuthStrategyOAuth1Signature &&
		method.RequestAuthStrategy != RequestAuthStrategyProviderCustom {
		return fmt.Errorf("integration %s auth method %s signed-request lifecycle requires a signing request strategy", integrationID, method.ID)
	}
	if method.IdentityKind == AuthIdentityKindChannel &&
		method.RequestAuthStrategy != RequestAuthStrategyWebhookURL &&
		method.RequestAuthStrategy != RequestAuthStrategyProviderCustom {
		return fmt.Errorf("integration %s channel auth method %s requires a channel-compatible request strategy", integrationID, method.ID)
	}
	return nil
}

func normalizeOAuthClientFields(integrationID, authMethodID string, fields []CredentialFieldDefinition) ([]CredentialFieldDefinition, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("integration %s OAuth method %s must declare OAuth client fields", integrationID, authMethodID)
	}
	seen := make(map[string]struct{}, len(fields))
	hasClientID := false
	out := make([]CredentialFieldDefinition, len(fields))
	for index, field := range fields {
		field.Key = strings.ToLower(strings.TrimSpace(field.Key))
		field.Label = strings.TrimSpace(field.Label)
		field.Description = strings.TrimSpace(field.Description)
		field.Placeholder = strings.TrimSpace(field.Placeholder)
		field.Input = CredentialFieldInput(strings.ToLower(strings.TrimSpace(string(field.Input))))
		if !integrationIdentifierPattern.MatchString(field.Key) || field.Label == "" || !validCredentialFieldInput(field.Input) {
			return nil, fmt.Errorf("integration %s OAuth method %s client field is invalid", integrationID, authMethodID)
		}
		if _, duplicated := seen[field.Key]; duplicated {
			return nil, fmt.Errorf("integration %s OAuth method %s client field %s is duplicated", integrationID, authMethodID, field.Key)
		}
		if field.Secret && field.Input != CredentialFieldInputPassword && field.Input != CredentialFieldInputTextarea {
			return nil, fmt.Errorf("integration %s OAuth method %s secret client field %s must use a secret input", integrationID, authMethodID, field.Key)
		}
		if field.Key == "client_id" {
			hasClientID = true
			field.Required = true
			field.Secret = false
		}
		var err error
		field.LabelI18n, err = normalizeLocalizedText(field.LabelI18n, field.Label, 128)
		if err != nil {
			return nil, fmt.Errorf("integration %s OAuth method %s client field %s localized label: %w", integrationID, authMethodID, field.Key, err)
		}
		field.DescriptionI18n, err = normalizeLocalizedText(field.DescriptionI18n, field.Description, 1000)
		if err != nil {
			return nil, fmt.Errorf("integration %s OAuth method %s client field %s localized description: %w", integrationID, authMethodID, field.Key, err)
		}
		field.PlaceholderI18n, err = normalizeLocalizedText(field.PlaceholderI18n, field.Placeholder, 256)
		if err != nil {
			return nil, fmt.Errorf("integration %s OAuth method %s client field %s localized placeholder: %w", integrationID, authMethodID, field.Key, err)
		}
		seen[field.Key] = struct{}{}
		out[index] = field
	}
	if !hasClientID {
		return nil, fmt.Errorf("integration %s OAuth method %s must declare a client_id field", integrationID, authMethodID)
	}
	return out, nil
}

func normalizeActionDefinition(integrationID string, action ActionDefinition) (ActionDefinition, error) {
	var err error
	action = cloneAction(action)
	action.ID = strings.ToLower(strings.TrimSpace(action.ID))
	action.ToolName = strings.TrimSpace(action.ToolName)
	action.Name = strings.TrimSpace(action.Name)
	action.Description = strings.TrimSpace(action.Description)
	if action.ID == "" || action.ToolName == "" {
		return ActionDefinition{}, fmt.Errorf("integration %s action id and tool name are required", integrationID)
	}
	if !integrationIdentifierPattern.MatchString(action.ID) || !integrationIdentifierPattern.MatchString(strings.ToLower(action.ToolName)) {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s id or tool name is invalid", integrationID, action.ID)
	}
	if action.Name == "" {
		action.Name = humanizeIdentifier(action.ID)
	}
	if len([]rune(action.Name)) > 128 || len([]rune(action.Description)) > 4000 {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s metadata is too large", integrationID, action.ID)
	}
	action.NameI18n, err = normalizeLocalizedText(action.NameI18n, action.Name, 128)
	if err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s localized name: %w", integrationID, action.ID, err)
	}
	action.DescriptionI18n, err = normalizeLocalizedText(action.DescriptionI18n, action.Description, 4000)
	if err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s localized description: %w", integrationID, action.ID, err)
	}
	if err := tools.ValidateJSONSchema(action.InputSchema); err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s input schema: %w", integrationID, action.ID, err)
	}
	if err := tools.ValidateJSONSchema(action.OutputSchema); err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s output schema: %w", integrationID, action.ID, err)
	}
	action.Effect = toolgovernance.NormalizeEffect(action.Effect)
	if action.Effect == toolgovernance.EffectNone {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s governance effect is required and must be valid", integrationID, action.ID)
	}
	if !validActionRiskLevel(action.RiskLevel) {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s governance risk level is required and must be valid", integrationID, action.ID)
	}
	action.RiskLevel = toolgovernance.NormalizeRiskLevel(action.RiskLevel)
	action.ExternalDestination = strings.ToLower(strings.TrimSpace(action.ExternalDestination))
	if action.DataEgress && action.ExternalDestination == "" {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s data-egress destination is required", integrationID, action.ID)
	}
	action.RequiredScopes = normalizeCatalogStringList(action.RequiredScopes, 128)
	action.RequiredAnyScopes = normalizeCatalogStringList(action.RequiredAnyScopes, 128)
	action.PreferredScopes = normalizeCatalogStringList(action.PreferredScopes, 128)
	requiredScopeIDs := ActionRequiredScopeIDs(action)
	requiredScopeSet := make(map[string]struct{}, len(requiredScopeIDs))
	for _, scopeID := range requiredScopeIDs {
		requiredScopeSet[scopeID] = struct{}{}
	}
	for _, scopeID := range action.PreferredScopes {
		if _, required := requiredScopeSet[scopeID]; !required {
			return ActionDefinition{}, fmt.Errorf(
				"integration %s action %s preferred scope %s is not part of its required scope union",
				integrationID,
				action.ID,
				scopeID,
			)
		}
	}
	if len(action.RequiredAnyScopes) > 0 {
		alternativeSet := make(map[string]struct{}, len(action.RequiredAnyScopes))
		for _, scopeID := range action.RequiredAnyScopes {
			alternativeSet[scopeID] = struct{}{}
		}
		preferredAlternativeCount := 0
		for _, scopeID := range action.PreferredScopes {
			if _, alternative := alternativeSet[scopeID]; alternative {
				preferredAlternativeCount++
			}
		}
		if preferredAlternativeCount != 1 {
			return ActionDefinition{}, fmt.Errorf(
				"integration %s action %s alternative scope group must declare exactly one preferred scope",
				integrationID,
				action.ID,
			)
		}
	}
	action.SupportedAuthMethodIDs = normalizeCatalogStringList(action.SupportedAuthMethodIDs, 32)
	action.ScopeLabelsI18n, err = normalizeLocalizedLabelMap(action.ScopeLabelsI18n, requiredScopeIDs, 128, 128)
	if err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s localized scope labels: %w", integrationID, action.ID, err)
	}
	callers, err := normalizeActionCallers(action.SupportedCallers)
	if err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s: %w", integrationID, action.ID, err)
	}
	action.SupportedCallers = callers
	if action.Effect != toolgovernance.EffectRead && actionSupportsCaller(action, tools.ToolInvokeFromAgent) {
		return ActionDefinition{}, fmt.Errorf(
			"integration %s action %s cannot advertise non-read execution to the non-interactive Agent runtime",
			integrationID,
			action.ID,
		)
	}
	action.PreparationHints, err = normalizeActionPreparationHints(integrationID, action)
	if err != nil {
		return ActionDefinition{}, err
	}
	if action.SuccessDeduplication != nil {
		if action.Idempotent || action.Effect == toolgovernance.EffectRead || action.Effect == toolgovernance.EffectNone {
			return ActionDefinition{}, fmt.Errorf("integration %s action %s success deduplication requires a non-idempotent side effect", integrationID, action.ID)
		}
		paths := normalizeCatalogStringList(action.SuccessDeduplication.TargetArgumentPaths, 16)
		if len(paths) == 0 {
			return ActionDefinition{}, fmt.Errorf("integration %s action %s success deduplication requires target argument paths", integrationID, action.ID)
		}
		for _, path := range paths {
			for _, segment := range strings.Split(path, ".") {
				if !integrationIdentifierPattern.MatchString(segment) {
					return ActionDefinition{}, fmt.Errorf("integration %s action %s success deduplication target path %s is invalid", integrationID, action.ID, path)
				}
			}
			if !actionPreparationResultPathExists(action.InputSchema, path) {
				return ActionDefinition{}, fmt.Errorf("integration %s action %s success deduplication target path %s must exist in the input schema", integrationID, action.ID, path)
			}
		}
		action.SuccessDeduplication = &SuccessDeduplicationDefinition{TargetArgumentPaths: paths}
	}
	if action.DefaultPolicy == nil {
		action.DefaultPolicy = &DefaultActionPolicy{
			Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyAutoByPermissionTier, DataEgressAllowed: true,
		}
	} else {
		policy := *action.DefaultPolicy
		policy.ApprovalPolicy = toolgovernance.ApprovalPolicy(strings.ToLower(strings.TrimSpace(string(policy.ApprovalPolicy))))
		switch policy.ApprovalPolicy {
		case toolgovernance.ApprovalPolicyAutoByPermissionTier, toolgovernance.ApprovalPolicyAlwaysAsk, toolgovernance.ApprovalPolicyNeverAsk:
		default:
			return ActionDefinition{}, fmt.Errorf("integration %s action %s default approval policy is invalid", integrationID, action.ID)
		}
		action.DefaultPolicy = &policy
	}
	computedSchemaHash, err := hashActionSchema(action)
	if err != nil {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s schema hash: %w", integrationID, action.ID, err)
	}
	declaredSchemaHash := strings.ToLower(strings.TrimSpace(action.SchemaHash))
	if declaredSchemaHash != "" && declaredSchemaHash != computedSchemaHash {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s schema hash does not match its schemas", integrationID, action.ID)
	}
	action.SchemaHash = computedSchemaHash
	action.SchemaRevision = strings.ToLower(strings.TrimSpace(action.SchemaRevision))
	if action.SchemaRevision == "" {
		action.SchemaRevision = computedSchemaHash
	} else if !validRevision(action.SchemaRevision) {
		return ActionDefinition{}, fmt.Errorf("integration %s action %s schema revision is invalid", integrationID, action.ID)
	}
	action.CatalogRevision = strings.ToLower(strings.TrimSpace(action.CatalogRevision))
	return action, nil
}

func validActionRiskLevel(risk toolgovernance.RiskLevel) bool {
	switch toolgovernance.RiskLevel(strings.ToLower(strings.TrimSpace(string(risk)))) {
	case toolgovernance.RiskLevelLow, toolgovernance.RiskLevelMedium, toolgovernance.RiskLevelHigh, toolgovernance.RiskLevelCritical:
		return true
	default:
		return false
	}
}

func normalizeActionCallers(callers []tools.ToolInvokeFrom) ([]tools.ToolInvokeFrom, error) {
	if len(callers) == 0 {
		return nil, nil
	}
	seen := make(map[tools.ToolInvokeFrom]struct{}, len(callers))
	out := make([]tools.ToolInvokeFrom, 0, len(callers))
	for _, raw := range callers {
		caller := tools.ToolInvokeFrom(strings.ToLower(strings.TrimSpace(string(raw))))
		switch caller {
		case tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent, tools.ToolInvokeFromWorkflow, tools.ToolInvokeFromAPI:
		default:
			return nil, fmt.Errorf("supported caller %q is invalid", raw)
		}
		if _, exists := seen[caller]; exists {
			continue
		}
		seen[caller] = struct{}{}
		out = append(out, caller)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func normalizeActionPreparationHints(integrationID string, action ActionDefinition) ([]ActionPreparationHint, error) {
	if len(action.PreparationHints) == 0 {
		return nil, nil
	}
	if len(action.PreparationHints) > 8 {
		return nil, fmt.Errorf("integration %s action %s declares too many preparation hints", integrationID, action.ID)
	}
	properties, _ := action.InputSchema["properties"].(map[string]interface{})
	seen := make(map[string]struct{}, len(action.PreparationHints))
	out := make([]ActionPreparationHint, 0, len(action.PreparationHints))
	for _, raw := range action.PreparationHints {
		hint := raw
		hint.ActionID = strings.ToLower(strings.TrimSpace(hint.ActionID))
		hint.Relation = ActionPreparationRelation(strings.ToLower(strings.TrimSpace(string(hint.Relation))))
		hint.Description = strings.TrimSpace(hint.Description)
		if !integrationIdentifierPattern.MatchString(hint.ActionID) || hint.ActionID == action.ID {
			return nil, fmt.Errorf("integration %s action %s preparation action is invalid", integrationID, action.ID)
		}
		switch hint.Relation {
		case ActionPreparationResolveTarget, ActionPreparationInspect:
		default:
			return nil, fmt.Errorf("integration %s action %s preparation relation is invalid", integrationID, action.ID)
		}
		if hint.Description == "" || len([]rune(hint.Description)) > 1000 {
			return nil, fmt.Errorf("integration %s action %s preparation description is required and bounded", integrationID, action.ID)
		}
		var err error
		hint.DescriptionI18n, err = normalizeLocalizedText(hint.DescriptionI18n, hint.Description, 1000)
		if err != nil {
			return nil, fmt.Errorf("integration %s action %s preparation description: %w", integrationID, action.ID, err)
		}
		hint.TargetArguments = normalizeCatalogStringList(hint.TargetArguments, 8)
		for _, argument := range hint.TargetArguments {
			if _, exists := properties[argument]; !exists {
				return nil, fmt.Errorf("integration %s action %s preparation references unknown argument %s", integrationID, action.ID, argument)
			}
		}
		if len(hint.ResultPaths) > 16 {
			return nil, fmt.Errorf("integration %s action %s preparation exposes too many result paths", integrationID, action.ID)
		}
		paths := make([]string, 0, len(hint.ResultPaths))
		pathSeen := make(map[string]struct{}, len(hint.ResultPaths))
		for _, rawPath := range hint.ResultPaths {
			path := strings.TrimSpace(rawPath)
			if path == "" || len([]rune(path)) > 256 || strings.ContainsAny(path, " \r\n\t") {
				return nil, fmt.Errorf("integration %s action %s preparation result path is invalid", integrationID, action.ID)
			}
			if _, exists := pathSeen[path]; exists {
				continue
			}
			pathSeen[path] = struct{}{}
			paths = append(paths, path)
		}
		sort.Strings(paths)
		hint.ResultPaths = paths
		key := string(hint.Relation) + "\x00" + hint.ActionID + "\x00" + strings.Join(hint.TargetArguments, "\x00")
		if _, duplicated := seen[key]; duplicated {
			return nil, fmt.Errorf("integration %s action %s preparation hint is duplicated", integrationID, action.ID)
		}
		seen[key] = struct{}{}
		out = append(out, hint)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActionID == out[j].ActionID {
			return out[i].Relation < out[j].Relation
		}
		return out[i].ActionID < out[j].ActionID
	})
	return out, nil
}

func actionPreparationResultPathExists(schema map[string]interface{}, path string) bool {
	current := schema
	segments := strings.Split(strings.TrimSpace(path), ".")
	if len(segments) == 0 {
		return false
	}
	for _, rawSegment := range segments {
		segment := strings.TrimSpace(rawSegment)
		if segment == "" {
			return false
		}
		arrayElement := strings.HasSuffix(segment, "[]")
		if arrayElement {
			segment = strings.TrimSuffix(segment, "[]")
			if segment == "" {
				return false
			}
		}
		properties, ok := current["properties"].(map[string]interface{})
		if !ok {
			return false
		}
		next, ok := properties[segment].(map[string]interface{})
		if !ok {
			return false
		}
		if arrayElement {
			if next["type"] != "array" {
				return false
			}
			next, ok = next["items"].(map[string]interface{})
			if !ok {
				return false
			}
		}
		current = next
	}
	return true
}

func (r *Registry) Resolve(integrationID, actionID string) (ResolvedAction, error) {
	if r == nil {
		return ResolvedAction{}, NewError(ErrorCodeDisabled, "integration registry is not configured", nil)
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	r.mu.RLock()
	registration, ok := r.registrations[integrationID]
	r.mu.RUnlock()
	if !ok {
		return ResolvedAction{}, NewError(ErrorCodeDisabled, "integration is not enabled", nil)
	}
	for _, action := range registration.Definition.Actions {
		if action.ID == actionID {
			return ResolvedAction{IntegrationID: integrationID, Adapter: registration.Adapter, Definition: cloneAction(action)}, nil
		}
	}
	return ResolvedAction{}, invalidInput("unknown integration action", nil)
}

func (r *Registry) Registration(integrationID string) (Registration, bool) {
	if r == nil {
		return Registration{}, false
	}
	r.mu.RLock()
	registration, ok := r.registrations[strings.ToLower(strings.TrimSpace(integrationID))]
	r.mu.RUnlock()
	if !ok {
		return Registration{}, false
	}
	return cloneRegistration(registration), true
}

func (r *Registry) Configured(integrationID string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.registrations[strings.ToLower(strings.TrimSpace(integrationID))]
	return ok
}

func (r *Registry) HasAction(integrationID, actionID string) bool {
	_, err := r.Resolve(integrationID, actionID)
	return err == nil
}

func (r *Registry) Actions(integrationID string) []ActionDefinition {
	registration, ok := r.Registration(integrationID)
	if !ok {
		return nil
	}
	return cloneActions(registration.Definition.Actions)
}

func (r *Registry) ActionDetail(integrationID, actionID string) (ActionDefinition, bool) {
	resolved, err := r.Resolve(integrationID, actionID)
	if err != nil {
		return ActionDefinition{}, false
	}
	return cloneAction(resolved.Definition), true
}

func (r *Registry) ProviderDefinitions() []ProviderDefinition {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]ProviderDefinition, 0, len(r.registrations))
	for _, registration := range r.registrations {
		out = append(out, cloneProviderDefinition(registration.Definition))
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Registry) ProviderDefinition(integrationID string) (ProviderDefinition, bool) {
	registration, ok := r.Registration(integrationID)
	if !ok {
		return ProviderDefinition{}, false
	}
	return cloneProviderDefinition(registration.Definition), true
}

func (r *Registry) Catalog() []ProviderCatalogItem {
	definitions := r.ProviderDefinitions()
	items := make([]ProviderCatalogItem, 0, len(definitions))
	for _, definition := range definitions {
		item := ProviderCatalogItem{
			ID: definition.ID, IntegrationID: definition.ID, DriverID: definition.DriverID,
			Name: definition.Name, NameI18n: cloneLocalizedText(definition.NameI18n),
			Description: definition.Description, DescriptionI18n: cloneLocalizedText(definition.DescriptionI18n), Author: definition.Author,
			Icon: definition.Icon, Tags: append([]string(nil), definition.Tags...), TagLabelsI18n: cloneLocalizedLabelMap(definition.TagLabelsI18n),
			Categories: append([]string(nil), definition.Categories...), CategoryLabelsI18n: cloneLocalizedLabelMap(definition.CategoryLabelsI18n),
			DocumentationURL: definition.DocumentationURL, DocumentationURLI18n: cloneLocalizedText(definition.DocumentationURLI18n), Enabled: true, CatalogRevision: definition.CatalogRevision,
			Auth: cloneAuthMethods(definition.AuthMethods), HealthProbe: definition.HealthProbe, Scopes: cloneProviderScopes(definition.Scopes),
			ConnectionTestMayIncurCost: definition.HealthProbe.MayIncurCost,
		}
		seenSources := map[ConnectionCredentialSource]struct{}{}
		seenAuthTypes := map[AuthMethodType]struct{}{}
		hasAvailableAuthMethod := false
		for _, method := range definition.AuthMethods {
			if !method.Available {
				continue
			}
			hasAvailableAuthMethod = true
			if !supportedConnectionCredentialSource(method.CredentialSource) {
				continue
			}
			if _, exists := seenSources[method.CredentialSource]; !exists {
				seenSources[method.CredentialSource] = struct{}{}
				item.CredentialSources = append(item.CredentialSources, method.CredentialSource)
			}
			if _, exists := seenAuthTypes[method.Type]; !exists {
				seenAuthTypes[method.Type] = struct{}{}
				item.AuthTypes = append(item.AuthTypes, method.Type)
			}
		}
		item.Enabled = hasAvailableAuthMethod
		sort.Slice(item.CredentialSources, func(i, j int) bool { return item.CredentialSources[i] < item.CredentialSources[j] })
		sort.Slice(item.AuthTypes, func(i, j int) bool { return item.AuthTypes[i] < item.AuthTypes[j] })
		item.Actions = make([]ActionSummary, 0, len(definition.Actions))
		for _, action := range definition.Actions {
			item.Actions = append(item.Actions, actionSummary(definition, action))
		}
		items = append(items, item)
	}
	return items
}

func (r *Registry) SearchActionSummaries(request ActionSearchRequest) []ActionSummary {
	query := strings.TrimSpace(request.Query)
	integrationID := strings.ToLower(strings.TrimSpace(request.IntegrationID))
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	definitions := r.ProviderDefinitions()
	if query == "" {
		out := make([]ActionSummary, 0, min(limit, 20))
		for _, definition := range definitions {
			if integrationID != "" && definition.ID != integrationID {
				continue
			}
			for _, action := range definition.Actions {
				if request.Caller != "" && !actionSupportsCaller(action, request.Caller) {
					continue
				}
				out = append(out, actionSummary(definition, action))
				if len(out) >= limit {
					return out
				}
			}
		}
		return out
	}

	type searchCandidate struct {
		summary ActionSummary
		score   int
	}
	candidates := make([]searchCandidate, 0, 20)
	for _, definition := range definitions {
		if integrationID != "" && definition.ID != integrationID {
			continue
		}
		for _, action := range definition.Actions {
			if request.Caller != "" && !actionSupportsCaller(action, request.Caller) {
				continue
			}
			score := actionSearchScore(query, definition, action)
			if score <= 0 {
				continue
			}
			candidates = append(candidates, searchCandidate{summary: actionSummary(definition, action), score: score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].summary.IntegrationID != candidates[j].summary.IntegrationID {
			return candidates[i].summary.IntegrationID < candidates[j].summary.IntegrationID
		}
		return candidates[i].summary.ID < candidates[j].summary.ID
	})
	out := make([]ActionSummary, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		out = append(out, candidate.summary)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func actionSearchScore(query string, definition ProviderDefinition, action ActionDefinition) int {
	type weightedValue struct {
		value string
		base  int
	}
	values := []weightedValue{
		{value: action.ID, base: 1200},
		{value: action.ToolName, base: 1150},
		{value: action.Name, base: 1000},
		{value: action.Description, base: 500},
		{value: strings.Join(action.RequiredScopes, " "), base: 350},
		{value: strings.Join(action.RequiredAnyScopes, " "), base: 350},
		{value: strings.Join(action.PreferredScopes, " "), base: 350},
		{value: definition.ID, base: 250},
		{value: definition.Name, base: 250},
		{value: strings.Join(definition.Tags, " "), base: 200},
		{value: strings.Join(definition.Categories, " "), base: 200},
	}
	for _, value := range localizedTextSearchValues(action.NameI18n) {
		values = append(values, weightedValue{value: value, base: 1000})
	}
	for _, value := range localizedTextSearchValues(action.DescriptionI18n) {
		values = append(values, weightedValue{value: value, base: 500})
	}
	for _, value := range localizedLabelMapSearchValues(action.ScopeLabelsI18n) {
		values = append(values, weightedValue{value: value, base: 350})
	}
	for _, value := range localizedTextSearchValues(definition.NameI18n) {
		values = append(values, weightedValue{value: value, base: 250})
	}
	for _, value := range localizedTextSearchValues(definition.DescriptionI18n) {
		values = append(values, weightedValue{value: value, base: 150})
	}
	for _, value := range localizedLabelMapSearchValues(definition.TagLabelsI18n) {
		values = append(values, weightedValue{value: value, base: 200})
	}
	for _, value := range localizedLabelMapSearchValues(definition.CategoryLabelsI18n) {
		values = append(values, weightedValue{value: value, base: 200})
	}

	best := 0
	for _, candidate := range values {
		if score := actionSearchValueScore(query, candidate.value, candidate.base); score > best {
			best = score
		}
	}
	return best
}

func actionSearchValueScore(query, value string, base int) int {
	normalizedQuery := normalizeActionSearchText(query)
	normalizedValue := normalizeActionSearchText(value)
	if normalizedQuery == "" || normalizedValue == "" {
		return 0
	}
	compactQuery := strings.ReplaceAll(normalizedQuery, " ", "")
	compactValue := strings.ReplaceAll(normalizedValue, " ", "")
	distancePenalty := min(max(0, len([]rune(compactValue))-len([]rune(compactQuery))), 80)
	switch {
	case normalizedValue == normalizedQuery:
		return base + 100
	case compactValue == compactQuery:
		return base + 90
	case strings.Contains(normalizedValue, normalizedQuery):
		return base + 80 - distancePenalty
	case strings.Contains(compactValue, compactQuery):
		return base + 70 - distancePenalty
	case actionSearchTermsMatch(normalizedQuery, normalizedValue):
		return base + 60 - distancePenalty
	case base >= 900 && len([]rune(compactQuery)) >= 2 && actionSearchRuneSubsequence(compactQuery, compactValue):
		return base + 50 - distancePenalty
	default:
		return 0
	}
}

func normalizeActionSearchText(value string) string {
	var builder strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func actionSearchTermsMatch(query, value string) bool {
	terms := strings.Fields(query)
	if len(terms) < 2 {
		return false
	}
	for _, term := range terms {
		if !strings.Contains(value, term) {
			return false
		}
	}
	return true
}

func actionSearchRuneSubsequence(query, value string) bool {
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return false
	}
	index := 0
	for _, r := range []rune(value) {
		if r == queryRunes[index] {
			index++
			if index == len(queryRunes) {
				return true
			}
		}
	}
	return false
}

func (r *Registry) Registrations() []Registration {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]Registration, 0, len(r.registrations))
	for _, registration := range r.registrations {
		out = append(out, cloneRegistration(registration))
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].IntegrationID < out[j].IntegrationID })
	return out
}

// ValidateConnection implements ConnectionTester and dispatches to the tester
// owned by the target registration.
func (r *Registry) ValidateConnection(ctx context.Context, connection *ResolvedConnection) (*ConnectionProfile, error) {
	if connection == nil {
		return nil, invalidInput("integration connection is required", nil)
	}
	registration, ok := r.Registration(connection.IntegrationID)
	if !ok || !strings.EqualFold(registration.Definition.DriverID, connection.DriverID) {
		return nil, NewError(ErrorCodeDisabled, "integration driver is not enabled", nil)
	}
	if registration.ConnectionTester == nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection testing is unsupported", nil)
	}
	return registration.ConnectionTester.ValidateConnection(ctx, connection)
}

func (r *Registry) ValidateProviderCredentials(ctx context.Context, request CredentialValidationRequest) error {
	registration, ok := r.Registration(request.IntegrationID)
	if !ok || !strings.EqualFold(registration.Definition.DriverID, request.DriverID) {
		return NewError(ErrorCodeDisabled, "integration driver is not enabled", nil)
	}
	if err := validateCredentialFieldValues(registration.Definition, request); err != nil {
		return err
	}
	if registration.CredentialValidator == nil {
		return nil
	}
	return registration.CredentialValidator.ValidateCredentials(ctx, request)
}

func (r *Registry) ProbeConnection(ctx context.Context, connection *ResolvedConnection) (*HealthProbeReport, error) {
	if connection == nil {
		return nil, invalidInput("integration connection is required", nil)
	}
	registration, ok := r.Registration(connection.IntegrationID)
	if !ok || !strings.EqualFold(registration.Definition.DriverID, connection.DriverID) {
		return nil, NewError(ErrorCodeDisabled, "integration driver is not enabled", nil)
	}
	if registration.HealthProbe != nil {
		return registration.HealthProbe.ProbeConnection(ctx, connection)
	}
	if registration.ConnectionTester == nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration health diagnostics are unsupported", nil)
	}
	profile, err := registration.ConnectionTester.ValidateConnection(ctx, connection)
	if err != nil {
		return &HealthProbeReport{Status: HealthProbeStatusUnhealthy}, err
	}
	return &HealthProbeReport{Status: HealthProbeStatusHealthy, Profile: profile}, nil
}

func (r *Registry) ResolveDynamicActionGovernance(ctx context.Context, request ActionGovernanceRequest) (ActionDefinition, error) {
	registration, ok := r.Registration(request.IntegrationID)
	if !ok {
		return ActionDefinition{}, NewError(ErrorCodeDisabled, "integration is not enabled", nil)
	}
	baseline, found := r.ActionDetail(request.IntegrationID, request.ActionID)
	if !found {
		return ActionDefinition{}, invalidInput("unknown integration action", nil)
	}
	request.IntegrationID = registration.Definition.ID
	request.ActionID = baseline.ID
	request.Baseline = baseline
	if registration.GovernanceResolver == nil {
		return baseline, nil
	}
	resolved, err := registration.GovernanceResolver.ResolveActionGovernance(ctx, request)
	if err != nil {
		return ActionDefinition{}, err
	}
	return enforceGovernanceBaseline(baseline, resolved)
}

func enforceGovernanceBaseline(baseline, resolved ActionDefinition) (ActionDefinition, error) {
	if strings.TrimSpace(resolved.ID) == "" {
		resolved.ID = baseline.ID
	}
	if !strings.EqualFold(resolved.ID, baseline.ID) || (resolved.ToolName != "" && resolved.ToolName != baseline.ToolName) {
		return ActionDefinition{}, invalidInput("dynamic governance cannot change action identity", nil)
	}
	resolved = cloneAction(resolved)
	resolved.ID = baseline.ID
	resolved.ToolName = baseline.ToolName
	resolved.Name = baseline.Name
	resolved.NameI18n = cloneLocalizedText(baseline.NameI18n)
	resolved.Description = baseline.Description
	resolved.DescriptionI18n = cloneLocalizedText(baseline.DescriptionI18n)
	resolved.InputSchema = cloneJSONMap(baseline.InputSchema)
	resolved.OutputSchema = cloneJSONMap(baseline.OutputSchema)
	resolved.SchemaHash = baseline.SchemaHash
	resolved.SchemaRevision = baseline.SchemaRevision
	resolved.CatalogRevision = baseline.CatalogRevision
	resolved.SupportedCallers = append([]tools.ToolInvokeFrom(nil), baseline.SupportedCallers...)
	resolved.SupportedAuthMethodIDs = append([]string(nil), baseline.SupportedAuthMethodIDs...)
	resolved.Idempotent = baseline.Idempotent
	resolved.SuccessDeduplication = nil
	if baseline.SuccessDeduplication != nil {
		guard := *baseline.SuccessDeduplication
		guard.TargetArgumentPaths = append([]string(nil), baseline.SuccessDeduplication.TargetArgumentPaths...)
		resolved.SuccessDeduplication = &guard
	}
	if resolved.Effect == "" {
		resolved.Effect = baseline.Effect
	}
	resolved.Effect = toolgovernance.NormalizeEffect(resolved.Effect)
	if resolved.Effect != baseline.Effect {
		return ActionDefinition{}, invalidInput("dynamic governance cannot change action effect", nil)
	}
	if toolgovernance.RiskRank(resolved.RiskLevel) < toolgovernance.RiskRank(baseline.RiskLevel) {
		resolved.RiskLevel = baseline.RiskLevel
	} else {
		resolved.RiskLevel = toolgovernance.NormalizeRiskLevel(resolved.RiskLevel)
	}
	if baseline.DataEgress {
		resolved.DataEgress = true
		resolved.ExternalDestination = baseline.ExternalDestination
	} else {
		resolved.ExternalDestination = strings.ToLower(strings.TrimSpace(resolved.ExternalDestination))
		if resolved.DataEgress && resolved.ExternalDestination == "" {
			return ActionDefinition{}, invalidInput("dynamic data egress requires an external destination", nil)
		}
	}
	resolved.SensitiveDataAllowed = baseline.SensitiveDataAllowed && resolved.SensitiveDataAllowed
	resolved.RequiredScopes = unionCatalogStrings(baseline.RequiredScopes, resolved.RequiredScopes)
	// A dynamic resolver may add all-of requirements, but it cannot safely
	// broaden an any-of group or change the OAuth preference established by
	// the signed provider catalog.
	resolved.RequiredAnyScopes = append([]string(nil), baseline.RequiredAnyScopes...)
	resolved.PreferredScopes = append([]string(nil), baseline.PreferredScopes...)
	resolved.ScopeLabelsI18n = cloneLocalizedLabelMap(baseline.ScopeLabelsI18n)
	if resolved.DefaultPolicy == nil {
		policy := *baseline.DefaultPolicy
		resolved.DefaultPolicy = &policy
	} else {
		if approvalPolicyRank(resolved.DefaultPolicy.ApprovalPolicy) < approvalPolicyRank(baseline.DefaultPolicy.ApprovalPolicy) {
			resolved.DefaultPolicy.ApprovalPolicy = baseline.DefaultPolicy.ApprovalPolicy
		}
		resolved.DefaultPolicy.Enabled = baseline.DefaultPolicy.Enabled && resolved.DefaultPolicy.Enabled
		resolved.DefaultPolicy.DataEgressAllowed = baseline.DefaultPolicy.DataEgressAllowed && resolved.DefaultPolicy.DataEgressAllowed
	}
	return resolved, nil
}

func validateCredentialFieldValues(definition ProviderDefinition, request CredentialValidationRequest) error {
	authMethodID := strings.ToLower(strings.TrimSpace(request.AuthMethodID))
	var method *AuthMethodDefinition
	for index := range definition.AuthMethods {
		if definition.AuthMethods[index].ID == authMethodID {
			method = &definition.AuthMethods[index]
			break
		}
	}
	if method == nil {
		return invalidInput("integration auth method is unsupported", nil)
	}
	allowed := make(map[string]CredentialFieldDefinition, len(method.Fields))
	for _, field := range method.Fields {
		allowed[field.Key] = field
	}
	for rawKey, value := range request.Credentials {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if _, exists := allowed[key]; !exists || strings.TrimSpace(value) == "" {
			return invalidInput("integration credentials do not match the auth method", nil)
		}
	}
	for key, field := range allowed {
		if field.Required && strings.TrimSpace(request.Credentials[key]) == "" {
			return invalidInput("integration credentials are incomplete", nil)
		}
	}
	return nil
}

func actionSummary(definition ProviderDefinition, action ActionDefinition) ActionSummary {
	policy := DefaultActionPolicy{}
	if action.DefaultPolicy != nil {
		policy = *action.DefaultPolicy
	}
	return ActionSummary{
		IntegrationID: definition.ID, DriverID: definition.DriverID, ID: action.ID, ToolName: action.ToolName,
		Name: action.Name, NameI18n: cloneLocalizedText(action.NameI18n),
		Description: action.Description, DescriptionI18n: cloneLocalizedText(action.DescriptionI18n), Effect: action.Effect, RiskLevel: action.RiskLevel,
		DataEgress: action.DataEgress, ExternalDestination: action.ExternalDestination,
		RequiredScopes:         append([]string(nil), action.RequiredScopes...),
		RequiredAnyScopes:      append([]string(nil), action.RequiredAnyScopes...),
		PreferredScopes:        append([]string(nil), action.PreferredScopes...),
		SupportedAuthMethodIDs: append([]string(nil), action.SupportedAuthMethodIDs...),
		ScopeLabelsI18n:        cloneLocalizedLabelMap(action.ScopeLabelsI18n), DefaultPolicy: policy,
		SchemaHash: action.SchemaHash, SchemaRevision: action.SchemaRevision, CatalogRevision: action.CatalogRevision,
		SupportedCallers: append([]tools.ToolInvokeFrom(nil), action.SupportedCallers...),
		SupportsBatch:    action.SuccessDeduplication != nil,
	}
}

func hashActionSchema(action ActionDefinition) (string, error) {
	return hashJSON(struct {
		Input  map[string]interface{} `json:"input"`
		Output map[string]interface{} `json:"output"`
	}{Input: action.InputSchema, Output: action.OutputSchema})
}

func hashCatalogDefinition(definition ProviderDefinition) (string, error) {
	definition = cloneProviderDefinition(definition)
	definition.CatalogRevision = ""
	for index := range definition.Actions {
		definition.Actions[index].CatalogRevision = ""
	}
	return hashJSON(definition)
}

func hashJSON(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validRevision(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func providerDefinitionEmpty(definition ProviderDefinition) bool {
	return definition.ID == "" && definition.DriverID == "" && definition.Name == "" && definition.Description == "" &&
		definition.Author == "" && definition.Icon == "" && len(definition.Tags) == 0 && len(definition.TagLabelsI18n) == 0 &&
		len(definition.Categories) == 0 && len(definition.CategoryLabelsI18n) == 0 &&
		definition.DocumentationURL == "" && len(definition.DocumentationURLI18n) == 0 && definition.CatalogRevision == "" && len(definition.AuthMethods) == 0 &&
		!definition.HealthProbe.Supported && !definition.HealthProbe.MayIncurCost && definition.HealthProbe.Description == "" && len(definition.Scopes) == 0 && len(definition.Actions) == 0
}

func sameActionIdentities(left, right []ActionDefinition) bool {
	if len(left) != len(right) {
		return false
	}
	leftIDs := make([]string, 0, len(left))
	rightIDs := make([]string, 0, len(right))
	for _, action := range left {
		leftIDs = append(leftIDs, strings.ToLower(strings.TrimSpace(action.ID))+"/"+strings.TrimSpace(action.ToolName))
	}
	for _, action := range right {
		rightIDs = append(rightIDs, strings.ToLower(strings.TrimSpace(action.ID))+"/"+strings.TrimSpace(action.ToolName))
	}
	sort.Strings(leftIDs)
	sort.Strings(rightIDs)
	return strings.Join(leftIDs, "\x00") == strings.Join(rightIDs, "\x00")
}

func cloneRegistration(registration Registration) Registration {
	registration.Definition = cloneProviderDefinition(registration.Definition)
	registration.Actions = cloneActions(registration.Actions)
	return registration
}

func cloneProviderDefinition(definition ProviderDefinition) ProviderDefinition {
	definition.NameI18n = cloneLocalizedText(definition.NameI18n)
	definition.DescriptionI18n = cloneLocalizedText(definition.DescriptionI18n)
	definition.DocumentationURLI18n = cloneLocalizedText(definition.DocumentationURLI18n)
	definition.TagLabelsI18n = cloneLocalizedLabelMap(definition.TagLabelsI18n)
	definition.CategoryLabelsI18n = cloneLocalizedLabelMap(definition.CategoryLabelsI18n)
	definition.HealthProbe.DescriptionI18n = cloneLocalizedText(definition.HealthProbe.DescriptionI18n)
	definition.Tags = append([]string(nil), definition.Tags...)
	definition.Categories = append([]string(nil), definition.Categories...)
	definition.AuthMethods = cloneAuthMethods(definition.AuthMethods)
	definition.Scopes = cloneProviderScopes(definition.Scopes)
	definition.Actions = cloneActions(definition.Actions)
	return definition
}

func normalizeProviderScopeDefinitions(integrationID string, scopes []ProviderScopeDefinition) ([]ProviderScopeDefinition, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	if len(scopes) > 256 {
		return nil, fmt.Errorf("integration %s provider scope catalog is too large", integrationID)
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]ProviderScopeDefinition, 0, len(scopes))
	for _, scope := range scopes {
		scope.ID = strings.TrimSpace(scope.ID)
		scope.Label = strings.TrimSpace(scope.Label)
		scope.Description = strings.TrimSpace(scope.Description)
		if scope.ID == "" || len([]rune(scope.ID)) > 512 || strings.ContainsAny(scope.ID, "\r\n\t") {
			return nil, fmt.Errorf("integration %s provider scope id is invalid", integrationID)
		}
		if _, exists := seen[scope.ID]; exists {
			return nil, fmt.Errorf("integration %s provider scope %s is duplicated", integrationID, scope.ID)
		}
		seen[scope.ID] = struct{}{}
		switch scope.Category {
		case ProviderScopeCategoryIdentity, ProviderScopeCategoryLifecycle, ProviderScopeCategoryProvider, ProviderScopeCategoryInternal:
		default:
			return nil, fmt.Errorf("integration %s provider scope %s category is invalid", integrationID, scope.ID)
		}
		switch scope.Access {
		case ProviderScopeAccessUnknown, ProviderScopeAccessRead, ProviderScopeAccessWrite, ProviderScopeAccessManage, ProviderScopeAccessIdentity, ProviderScopeAccessSession:
		default:
			return nil, fmt.Errorf("integration %s provider scope %s access is invalid", integrationID, scope.ID)
		}
		var err error
		scope.LabelI18n, err = normalizeLocalizedText(scope.LabelI18n, scope.Label, 128)
		if err != nil {
			return nil, fmt.Errorf("integration %s provider scope %s localized label: %w", integrationID, scope.ID, err)
		}
		scope.DescriptionI18n, err = normalizeLocalizedText(scope.DescriptionI18n, scope.Description, 1000)
		if err != nil {
			return nil, fmt.Errorf("integration %s provider scope %s localized description: %w", integrationID, scope.ID, err)
		}
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneProviderScopes(scopes []ProviderScopeDefinition) []ProviderScopeDefinition {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]ProviderScopeDefinition, len(scopes))
	for index, scope := range scopes {
		scope.LabelI18n = cloneLocalizedText(scope.LabelI18n)
		scope.DescriptionI18n = cloneLocalizedText(scope.DescriptionI18n)
		out[index] = scope
	}
	return out
}

func cloneAuthMethods(methods []AuthMethodDefinition) []AuthMethodDefinition {
	if len(methods) == 0 {
		return nil
	}
	out := make([]AuthMethodDefinition, len(methods))
	for index, method := range methods {
		method.LabelI18n = cloneLocalizedText(method.LabelI18n)
		method.DescriptionI18n = cloneLocalizedText(method.DescriptionI18n)
		if method.SetupGuide != nil {
			guide := *method.SetupGuide
			guide.Steps = append([]AuthSetupStepDefinition(nil), method.SetupGuide.Steps...)
			for stepIndex := range guide.Steps {
				guide.Steps[stepIndex].TitleI18n = cloneLocalizedText(guide.Steps[stepIndex].TitleI18n)
				guide.Steps[stepIndex].DescriptionI18n = cloneLocalizedText(guide.Steps[stepIndex].DescriptionI18n)
			}
			guide.Notices = append([]AuthSetupNoticeDefinition(nil), method.SetupGuide.Notices...)
			for noticeIndex := range guide.Notices {
				guide.Notices[noticeIndex].TextI18n = cloneLocalizedText(guide.Notices[noticeIndex].TextI18n)
			}
			method.SetupGuide = &guide
		}
		if method.OAuth != nil {
			metadata := *method.OAuth
			metadata.IdentityScopes = append([]string(nil), method.OAuth.IdentityScopes...)
			metadata.DefaultActionIDs = append([]string(nil), method.OAuth.DefaultActionIDs...)
			metadata.ClientFields = append([]CredentialFieldDefinition(nil), method.OAuth.ClientFields...)
			for fieldIndex := range metadata.ClientFields {
				metadata.ClientFields[fieldIndex].LabelI18n = cloneLocalizedText(metadata.ClientFields[fieldIndex].LabelI18n)
				metadata.ClientFields[fieldIndex].DescriptionI18n = cloneLocalizedText(metadata.ClientFields[fieldIndex].DescriptionI18n)
				metadata.ClientFields[fieldIndex].PlaceholderI18n = cloneLocalizedText(metadata.ClientFields[fieldIndex].PlaceholderI18n)
				metadata.ClientFields[fieldIndex].Options = append([]CredentialFieldOption(nil), metadata.ClientFields[fieldIndex].Options...)
			}
			method.OAuth = &metadata
		}
		method.Fields = append([]CredentialFieldDefinition(nil), method.Fields...)
		for fieldIndex := range method.Fields {
			method.Fields[fieldIndex].LabelI18n = cloneLocalizedText(method.Fields[fieldIndex].LabelI18n)
			method.Fields[fieldIndex].DescriptionI18n = cloneLocalizedText(method.Fields[fieldIndex].DescriptionI18n)
			method.Fields[fieldIndex].PlaceholderI18n = cloneLocalizedText(method.Fields[fieldIndex].PlaceholderI18n)
			method.Fields[fieldIndex].Options = append([]CredentialFieldOption(nil), method.Fields[fieldIndex].Options...)
			for optionIndex := range method.Fields[fieldIndex].Options {
				method.Fields[fieldIndex].Options[optionIndex].LabelI18n = cloneLocalizedText(method.Fields[fieldIndex].Options[optionIndex].LabelI18n)
			}
		}
		out[index] = method
	}
	return out
}

func cloneActions(actions []ActionDefinition) []ActionDefinition {
	if len(actions) == 0 {
		return nil
	}
	out := make([]ActionDefinition, len(actions))
	for index, action := range actions {
		out[index] = cloneAction(action)
	}
	return out
}

func cloneAction(action ActionDefinition) ActionDefinition {
	action.NameI18n = cloneLocalizedText(action.NameI18n)
	action.DescriptionI18n = cloneLocalizedText(action.DescriptionI18n)
	action.ScopeLabelsI18n = cloneLocalizedLabelMap(action.ScopeLabelsI18n)
	action.InputSchema = cloneJSONMap(action.InputSchema)
	action.OutputSchema = cloneJSONMap(action.OutputSchema)
	action.RequiredScopes = append([]string(nil), action.RequiredScopes...)
	action.RequiredAnyScopes = append([]string(nil), action.RequiredAnyScopes...)
	action.PreferredScopes = append([]string(nil), action.PreferredScopes...)
	action.SupportedAuthMethodIDs = append([]string(nil), action.SupportedAuthMethodIDs...)
	action.SupportedCallers = append([]tools.ToolInvokeFrom(nil), action.SupportedCallers...)
	action.PreparationHints = append([]ActionPreparationHint(nil), action.PreparationHints...)
	for index := range action.PreparationHints {
		action.PreparationHints[index].TargetArguments = append([]string(nil), action.PreparationHints[index].TargetArguments...)
		action.PreparationHints[index].ResultPaths = append([]string(nil), action.PreparationHints[index].ResultPaths...)
		action.PreparationHints[index].DescriptionI18n = cloneLocalizedText(action.PreparationHints[index].DescriptionI18n)
	}
	if action.DefaultPolicy != nil {
		policy := *action.DefaultPolicy
		action.DefaultPolicy = &policy
	}
	if action.SuccessDeduplication != nil {
		guard := *action.SuccessDeduplication
		guard.TargetArgumentPaths = append([]string(nil), action.SuccessDeduplication.TargetArgumentPaths...)
		action.SuccessDeduplication = &guard
	}
	return action
}

func cloneJSONMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	out := make(map[string]interface{}, len(value))
	for key, item := range value {
		out[key] = cloneJSONValue(item)
	}
	return out
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneJSONMap(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, item := range typed {
			out[index] = cloneJSONValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func normalizeCatalogStringList(values []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(values), limit))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" || len(value) > 128 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

func unionCatalogStrings(left, right []string) []string {
	return normalizeCatalogStringList(append(append([]string(nil), left...), right...), 128)
}

func humanizeIdentifier(value string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(value))
	for index := range words {
		if words[index] == "" {
			continue
		}
		words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
	}
	return strings.Join(words, " ")
}

func approvalPolicyRank(policy toolgovernance.ApprovalPolicy) int {
	switch toolgovernance.NormalizeApprovalPolicy(policy) {
	case toolgovernance.ApprovalPolicyAlwaysAsk:
		return 3
	case toolgovernance.ApprovalPolicyAutoByPermissionTier:
		return 2
	default:
		return 1
	}
}
