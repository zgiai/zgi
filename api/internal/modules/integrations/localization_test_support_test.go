package integrations

import "github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"

// localizedTestProviderDefinition keeps non-localization unit tests explicit
// about the production catalog's supported-language contract. The Chinese
// strings are deliberate test copy, not English values mirrored into another
// locale.
func localizedTestProviderDefinition(integrationID, driverID string, actions []ActionDefinition) ProviderDefinition {
	localizedActions := cloneActions(actions)
	for index := range localizedActions {
		localizeTestAction(&localizedActions[index])
	}
	definition := ProviderDefinition{
		ID:          integrationID,
		DriverID:    driverID,
		Name:        "Test external application",
		NameI18n:    LocalizedText{LocaleEnglishUS: "Test external application", LocaleSimplifiedChinese: "测试外部应用"},
		Description: "External application used by integration tests.",
		DescriptionI18n: LocalizedText{
			LocaleEnglishUS:         "External application used by integration tests.",
			LocaleSimplifiedChinese: "集成测试使用的外部应用。",
		},
		AuthMethods: []AuthMethodDefinition{{
			ID:               string(AuthMethodTypeNone),
			Type:             AuthMethodTypeNone,
			CredentialSource: ConnectionCredentialSourceOrganization,
			Label:            "No authentication",
			LabelI18n:        LocalizedText{LocaleEnglishUS: "No authentication", LocaleSimplifiedChinese: "无需认证"},
			Available:        true,
		}},
		Actions: localizedActions,
	}
	addTestProviderScopeDefinitions(&definition)
	return definition
}

func localizedTestRegistration(integrationID string, adapter Adapter, actions []ActionDefinition) Registration {
	driverID := ""
	if adapter != nil {
		driverID = adapter.DriverID()
	}
	return Registration{
		Definition:    localizedTestProviderDefinition(integrationID, driverID, actions),
		IntegrationID: integrationID,
		Adapter:       adapter,
		Actions:       cloneActions(actions),
	}
}

func localizeTestProviderFixture(definition *ProviderDefinition) {
	if definition.Name == "" {
		definition.Name = "Test external application"
	}
	if definition.Description == "" {
		definition.Description = "External application used by integration tests."
	}
	definition.NameI18n = LocalizedText{
		LocaleEnglishUS:         definition.Name,
		LocaleSimplifiedChinese: "测试外部应用",
	}
	definition.DescriptionI18n = LocalizedText{
		LocaleEnglishUS:         definition.Description,
		LocaleSimplifiedChinese: "集成测试使用的外部应用。",
	}
	for _, tag := range definition.Tags {
		if definition.TagLabelsI18n == nil {
			definition.TagLabelsI18n = LocalizedLabelMap{}
		}
		definition.TagLabelsI18n[tag] = LocalizedText{LocaleEnglishUS: "Test tag", LocaleSimplifiedChinese: "测试标签"}
	}
	for _, category := range definition.Categories {
		if definition.CategoryLabelsI18n == nil {
			definition.CategoryLabelsI18n = LocalizedLabelMap{}
		}
		definition.CategoryLabelsI18n[category] = LocalizedText{LocaleEnglishUS: "Test category", LocaleSimplifiedChinese: "测试分类"}
	}
	for methodIndex := range definition.AuthMethods {
		method := &definition.AuthMethods[methodIndex]
		method.LabelI18n = LocalizedText{LocaleEnglishUS: method.Label, LocaleSimplifiedChinese: "测试认证方式"}
		if method.Description != "" {
			method.DescriptionI18n = LocalizedText{LocaleEnglishUS: method.Description, LocaleSimplifiedChinese: "测试认证方式说明。"}
		}
		for fieldIndex := range method.Fields {
			field := &method.Fields[fieldIndex]
			field.LabelI18n = LocalizedText{LocaleEnglishUS: field.Label, LocaleSimplifiedChinese: "测试凭据"}
			if field.Description != "" {
				field.DescriptionI18n = LocalizedText{LocaleEnglishUS: field.Description, LocaleSimplifiedChinese: "测试凭据说明。"}
			}
			if field.Placeholder != "" {
				field.PlaceholderI18n = LocalizedText{LocaleEnglishUS: field.Placeholder, LocaleSimplifiedChinese: "请输入测试凭据"}
			}
			for optionIndex := range field.Options {
				option := &field.Options[optionIndex]
				option.LabelI18n = LocalizedText{LocaleEnglishUS: option.Label, LocaleSimplifiedChinese: "测试选项"}
			}
		}
	}
	if definition.HealthProbe.Description != "" {
		definition.HealthProbe.DescriptionI18n = LocalizedText{
			LocaleEnglishUS:         definition.HealthProbe.Description,
			LocaleSimplifiedChinese: "测试连接健康检查。",
		}
	}
	for index := range definition.Actions {
		localizeTestAction(&definition.Actions[index])
	}
	addTestProviderScopeDefinitions(definition)
}

func localizeTestAction(action *ActionDefinition) {
	if action.Name == "" {
		action.Name = "Test action"
	}
	if action.Description == "" {
		action.Description = "Execute an integration test action."
	}
	action.NameI18n = LocalizedText{LocaleEnglishUS: action.Name, LocaleSimplifiedChinese: "测试操作"}
	action.DescriptionI18n = LocalizedText{
		LocaleEnglishUS:         action.Description,
		LocaleSimplifiedChinese: "执行集成测试操作。",
	}
	for _, scope := range ActionRequiredScopeIDs(*action) {
		if action.ScopeLabelsI18n == nil {
			action.ScopeLabelsI18n = LocalizedLabelMap{}
		}
		action.ScopeLabelsI18n[scope] = LocalizedText{LocaleEnglishUS: "Test scope", LocaleSimplifiedChinese: "测试权限"}
	}
}

func addTestProviderScopeDefinitions(definition *ProviderDefinition) {
	if definition == nil {
		return
	}
	seen := make(map[string]struct{}, len(definition.Scopes))
	for _, scope := range definition.Scopes {
		seen[scope.ID] = struct{}{}
	}
	addScope := func(scopeID string, category ProviderScopeCategory, access ProviderScopeAccess) {
		if scopeID == "" {
			return
		}
		if _, exists := seen[scopeID]; exists {
			return
		}
		seen[scopeID] = struct{}{}
		definition.Scopes = append(definition.Scopes, ProviderScopeDefinition{
			ID: scopeID, Label: scopeID,
			LabelI18n: LocalizedText{LocaleEnglishUS: scopeID, LocaleSimplifiedChinese: "测试权限"},
			Category:  category, Access: access,
		})
	}
	for _, method := range definition.AuthMethods {
		if method.OAuth == nil {
			continue
		}
		for _, scopeID := range method.OAuth.IdentityScopes {
			addScope(scopeID, ProviderScopeCategoryIdentity, ProviderScopeAccessIdentity)
		}
	}
	for _, action := range definition.Actions {
		access := ProviderScopeAccessRead
		if action.Effect != toolgovernance.EffectRead && action.Effect != toolgovernance.EffectNone {
			access = ProviderScopeAccessWrite
		}
		for _, scopeID := range ActionRequiredScopeIDs(action) {
			addScope(scopeID, ProviderScopeCategoryProvider, access)
		}
	}
}
