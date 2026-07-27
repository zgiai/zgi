package integrations

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestRegistryRejectsIncompleteSupportedLanguageContractsWithFieldPaths(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Registration)
		want   string
	}{
		{
			name: "provider name",
			mutate: func(value *Registration) {
				delete(value.Definition.NameI18n, LocaleSimplifiedChinese)
			},
			want: "provider.name_i18n.zh-Hans is required",
		},
		{
			name: "provider description",
			mutate: func(value *Registration) {
				delete(value.Definition.DescriptionI18n, LocaleSimplifiedChinese)
			},
			want: "provider.description_i18n.zh-Hans is required",
		},
		{
			name: "declared tag",
			mutate: func(value *Registration) {
				delete(value.Definition.TagLabelsI18n["external"], LocaleSimplifiedChinese)
			},
			want: "provider.tag_labels_i18n.external.zh-Hans is required",
		},
		{
			name: "declared category",
			mutate: func(value *Registration) {
				delete(value.Definition.CategoryLabelsI18n, "developer_tools")
			},
			want: "provider.category_labels_i18n.developer_tools is required",
		},
		{
			name: "auth label",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].LabelI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].label_i18n.zh-Hans is required",
		},
		{
			name: "auth description",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].DescriptionI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].description_i18n.zh-Hans is required",
		},
		{
			name: "localized-only English auth description",
			mutate: func(value *Registration) {
				value.Definition.AuthMethods[0].Description = ""
				value.Definition.AuthMethods[0].DescriptionI18n = LocalizedText{LocaleEnglishUS: "Use a token."}
			},
			want: "provider.auth[pat].description_i18n.zh-Hans is required",
		},
		{
			name: "credential field label",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].Fields[0].LabelI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].fields[mode].label_i18n.zh-Hans is required",
		},
		{
			name: "credential field description",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].Fields[0].DescriptionI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].fields[mode].description_i18n.zh-Hans is required",
		},
		{
			name: "credential field placeholder",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].Fields[0].PlaceholderI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].fields[mode].placeholder_i18n.zh-Hans is required",
		},
		{
			name: "localized-only English credential placeholder",
			mutate: func(value *Registration) {
				field := &value.Definition.AuthMethods[0].Fields[0]
				field.Placeholder = ""
				field.PlaceholderI18n = LocalizedText{LocaleEnglishUS: "Select a mode"}
			},
			want: "provider.auth[pat].fields[mode].placeholder_i18n.zh-Hans is required",
		},
		{
			name: "credential option label",
			mutate: func(value *Registration) {
				delete(value.Definition.AuthMethods[0].Fields[0].Options[0].LabelI18n, LocaleSimplifiedChinese)
			},
			want: "provider.auth[pat].fields[mode].options[read].label_i18n.zh-Hans is required",
		},
		{
			name: "health probe description",
			mutate: func(value *Registration) {
				delete(value.Definition.HealthProbe.DescriptionI18n, LocaleSimplifiedChinese)
			},
			want: "provider.health_probe.description_i18n.zh-Hans is required",
		},
		{
			name: "localized-only English health probe description",
			mutate: func(value *Registration) {
				value.Definition.HealthProbe.Description = ""
				value.Definition.HealthProbe.DescriptionI18n = LocalizedText{LocaleEnglishUS: "Check provider access."}
			},
			want: "provider.health_probe.description_i18n.zh-Hans is required",
		},
		{
			name: "action name",
			mutate: func(value *Registration) {
				delete(value.Definition.Actions[0].NameI18n, LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].name_i18n.zh-Hans is required",
		},
		{
			name: "action description",
			mutate: func(value *Registration) {
				delete(value.Definition.Actions[0].DescriptionI18n, LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].description_i18n.zh-Hans is required",
		},
		{
			name: "declared scope",
			mutate: func(value *Registration) {
				delete(value.Definition.Actions[0].ScopeLabelsI18n["records:read"], LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].scope_labels_i18n.records:read.zh-Hans is required",
		},
		{
			name: "top-level input title",
			mutate: func(value *Registration) {
				property := localizationTestProperty(value, "mode")
				property["title_i18n"] = LocalizedText{LocaleSimplifiedChinese: "模式"}
			},
			want: "provider.actions[test.run].input_schema.properties.mode.title_i18n.en-US is required",
		},
		{
			name: "nested object input title",
			mutate: func(value *Registration) {
				filter := localizationTestProperty(value, "filter")
				state := filter["properties"].(map[string]interface{})["state"].(map[string]interface{})
				delete(state["title_i18n"].(LocalizedText), LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].input_schema.properties.filter.properties.state.title_i18n.zh-Hans is required",
		},
		{
			name: "array item object input title",
			mutate: func(value *Registration) {
				rows := localizationTestProperty(value, "rows")
				item := rows["items"].(map[string]interface{})
				name := item["properties"].(map[string]interface{})["name"].(map[string]interface{})
				delete(name["title_i18n"].(LocalizedText), LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].input_schema.properties.rows.items.properties.name.title_i18n.zh-Hans is required",
		},
		{
			name: "enum locale",
			mutate: func(value *Registration) {
				mode := localizationTestProperty(value, "mode")
				delete(mode["enum_labels_i18n"].(map[string]map[string]string), LocaleSimplifiedChinese)
			},
			want: "provider.actions[test.run].input_schema.properties.mode.enum_labels_i18n.zh-Hans is required",
		},
		{
			name: "enum declared value",
			mutate: func(value *Registration) {
				mode := localizationTestProperty(value, "mode")
				delete(mode["enum_labels_i18n"].(map[string]map[string]string)[LocaleSimplifiedChinese], "write")
			},
			want: "provider.actions[test.run].input_schema.properties.mode.enum_labels_i18n.zh-Hans.write is required",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			registration := completeLocalizationContractRegistration()
			testCase.mutate(&registration)
			err := NewRegistry().Register(registration)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Register() error = %v, want field path %q", err, testCase.want)
			}
		})
	}
}

func TestRegistryAcceptsCompleteSupportedLanguageContract(t *testing.T) {
	if err := NewRegistry().Register(completeLocalizationContractRegistration()); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestRegistryLegacyRegistrationCannotBypassSupportedLanguageContract(t *testing.T) {
	registration := Registration{
		IntegrationID: "test-provider",
		Adapter:       &testAdapter{driverID: "test-driver"},
		Actions:       []ActionDefinition{testAction("test.run", "run_test")},
	}
	err := NewRegistry().Register(registration)
	if err == nil || !strings.Contains(err.Error(), "provider.name_i18n.zh-Hans is required") {
		t.Fatalf("Register() error = %v, want explicit provider localization rejection", err)
	}
}

func completeLocalizationContractRegistration() Registration {
	localizedTitle := func(english, chinese string) LocalizedText {
		return LocalizedText{LocaleEnglishUS: english, LocaleSimplifiedChinese: chinese}
	}
	action := ActionDefinition{
		ID:          "test.run",
		ToolName:    "run_test",
		Name:        "Run test",
		NameI18n:    localizedTitle("Run test", "运行测试"),
		Description: "Run one test action.",
		DescriptionI18n: LocalizedText{
			LocaleEnglishUS: "Run one test action.", LocaleSimplifiedChinese: "运行一个测试操作。",
		},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type": "string", "enum": []string{"read", "write"},
					"title_i18n": localizedTitle("Mode", "模式"),
					"enum_labels_i18n": map[string]map[string]string{
						LocaleEnglishUS:         {"read": "Read", "write": "Write"},
						LocaleSimplifiedChinese: {"read": "读取", "write": "写入"},
					},
				},
				"filter": map[string]interface{}{
					"type": "object", "title_i18n": localizedTitle("Filter", "筛选条件"),
					"properties": map[string]interface{}{
						"state": map[string]interface{}{"type": "string", "title_i18n": localizedTitle("State", "状态")},
					},
				},
				"rows": map[string]interface{}{
					"type": "array", "title_i18n": localizedTitle("Rows", "数据行"),
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{"type": "string", "title_i18n": localizedTitle("Name", "名称")},
						},
					},
				},
			},
			"additionalProperties": false,
		},
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}},
		},
		Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		RequiredScopes: []string{"records:read"},
		ScopeLabelsI18n: LocalizedLabelMap{
			"records:read": {LocaleEnglishUS: "Read records", LocaleSimplifiedChinese: "读取记录"},
		},
	}
	definition := ProviderDefinition{
		ID:          "test-provider",
		DriverID:    "test-driver",
		Name:        "Test provider",
		NameI18n:    localizedTitle("Test provider", "测试提供方"),
		Description: "Provider used to validate localization contracts.",
		DescriptionI18n: LocalizedText{
			LocaleEnglishUS: "Provider used to validate localization contracts.", LocaleSimplifiedChinese: "用于验证本地化合同的提供方。",
		},
		Tags: []string{"external"},
		TagLabelsI18n: LocalizedLabelMap{
			"external": {LocaleEnglishUS: "External", LocaleSimplifiedChinese: "外部应用"},
		},
		Categories: []string{"developer_tools"},
		CategoryLabelsI18n: LocalizedLabelMap{
			"developer_tools": {LocaleEnglishUS: "Developer tools", LocaleSimplifiedChinese: "开发工具"},
		},
		AuthMethods: []AuthMethodDefinition{{
			ID: "pat", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization,
			Label: "Personal access token", LabelI18n: localizedTitle("Personal access token", "个人访问令牌"),
			Description: "Choose an access mode.",
			DescriptionI18n: LocalizedText{
				LocaleEnglishUS: "Choose an access mode.", LocaleSimplifiedChinese: "选择访问模式。",
			},
			Available: true,
			Fields: []CredentialFieldDefinition{{
				Key: "mode", Label: "Access mode", LabelI18n: localizedTitle("Access mode", "访问模式"),
				Description: "Controls credential access.",
				DescriptionI18n: LocalizedText{
					LocaleEnglishUS: "Controls credential access.", LocaleSimplifiedChinese: "控制凭据访问方式。",
				},
				Input: CredentialFieldInputSelect, Required: true,
				Placeholder: "Select an access mode",
				PlaceholderI18n: LocalizedText{
					LocaleEnglishUS: "Select an access mode", LocaleSimplifiedChinese: "请选择访问模式",
				},
				Options: []CredentialFieldOption{
					{Value: "read", Label: "Read", LabelI18n: localizedTitle("Read", "读取")},
					{Value: "write", Label: "Write", LabelI18n: localizedTitle("Write", "写入")},
				},
			}},
		}},
		HealthProbe: HealthProbeDefinition{
			Supported: true, Description: "Check provider access.",
			DescriptionI18n: LocalizedText{
				LocaleEnglishUS: "Check provider access.", LocaleSimplifiedChinese: "检查提供方访问状态。",
			},
		},
		Actions: []ActionDefinition{action},
	}
	return Registration{
		Definition: definition,
		Adapter:    &testAdapter{driverID: "test-driver"},
	}
}

func localizationTestProperty(registration *Registration, name string) map[string]interface{} {
	properties := registration.Definition.Actions[0].InputSchema["properties"].(map[string]interface{})
	return properties[name].(map[string]interface{})
}
