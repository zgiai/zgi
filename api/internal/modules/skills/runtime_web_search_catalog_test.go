package skills

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestWebSearchSkillCatalogMetadataAndGovernance(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWebSearch})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}

	doc, ok := resolved.Get(SkillWebSearch)
	if !ok {
		t.Fatalf("web search skill was not resolved")
	}
	if !reflect.DeepEqual(doc.Metadata.SupportedCallers, []string{SkillCallerAIChat, SkillCallerAgent}) {
		t.Fatalf("supported callers = %#v", doc.Metadata.SupportedCallers)
	}
	if !reflect.DeepEqual(doc.Metadata.RequiredConfig, []string{SkillRequiredConfigWebSearch}) {
		t.Fatalf("required config = %#v", doc.Metadata.RequiredConfig)
	}
	if doc.Metadata.DependencyType != SkillDependencyIntegration {
		t.Fatalf("dependency type = %q, want %q", doc.Metadata.DependencyType, SkillDependencyIntegration)
	}
	wantRequirements := []SkillIntegrationRequirement{{
		IntegrationID: "web-search",
		ActionIDs:     []string{"web.fetch", "web.search"},
		Required:      true,
	}}
	if !reflect.DeepEqual(doc.Metadata.IntegrationRequirements, wantRequirements) {
		t.Fatalf("integration requirements = %#v, want %#v", doc.Metadata.IntegrationRequirements, wantRequirements)
	}
	if len(doc.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(doc.Tools))
	}

	wantToolIDs := map[string]string{
		"search_web":    "web.search",
		"fetch_webpage": "web.fetch",
	}
	for _, definition := range doc.Tools {
		if definition.ProviderType != tools.ToolProviderTypeConnector {
			t.Errorf("tool %s provider type = %q, want connector", definition.Name, definition.ProviderType)
		}
		if definition.ProviderID != SkillWebSearch {
			t.Errorf("tool %s provider ID = %q, want %q", definition.Name, definition.ProviderID, SkillWebSearch)
		}
		wantToolID, exists := wantToolIDs[definition.Name]
		if !exists {
			t.Errorf("unexpected web search tool %q", definition.Name)
			continue
		}
		manifest := definition.Governance
		if manifest == nil {
			t.Errorf("tool %s governance manifest is nil", definition.Name)
			continue
		}
		if manifest.ToolID != wantToolID || manifest.SkillID != SkillWebSearch {
			t.Errorf("tool %s governance identity = %s/%s", definition.Name, manifest.SkillID, manifest.ToolID)
		}
		if manifest.Domain != "web" || manifest.Effect != toolgovernance.EffectRead || manifest.RiskLevel != toolgovernance.RiskLevelLow {
			t.Errorf("tool %s governance classification = domain %q, effect %q, risk %q", definition.Name, manifest.Domain, manifest.Effect, manifest.RiskLevel)
		}
		if manifest.ExternalSideEffect || !manifest.DataEgress || manifest.ExternalDestination != "configured-web-search-provider" || manifest.SensitiveDataAllowed {
			t.Errorf("tool %s data-egress governance = side_effect %v, egress %v, destination %q, sensitive %v", definition.Name, manifest.ExternalSideEffect, manifest.DataEgress, manifest.ExternalDestination, manifest.SensitiveDataAllowed)
		}
		if !manifest.AuditRequired || manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
			t.Errorf("tool %s audit/approval = %v/%q", definition.Name, manifest.AuditRequired, manifest.DefaultApprovalPolicy)
		}
		if !reflect.DeepEqual(manifest.AllowedPermissionTiers, []toolgovernance.PermissionTier{
			toolgovernance.PermissionTierBasic,
			toolgovernance.PermissionTierAdvanced,
			toolgovernance.PermissionTierFull,
		}) {
			t.Errorf("tool %s permission tiers = %#v", definition.Name, manifest.AllowedPermissionTiers)
		}
		delete(wantToolIDs, definition.Name)
	}
	if len(wantToolIDs) != 0 {
		t.Fatalf("missing web search tools: %#v", wantToolIDs)
	}
}

func TestWebSearchConnectorSchemasFeedResolvedSkillAndMetaTool(t *testing.T) {
	manager := tools.NewToolManager(nil)
	provider := newWebSearchSchemaProviderForTest()
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWebSearch})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillWebSearch)
	if !ok {
		t.Fatalf("web search skill was not resolved")
	}

	for _, definition := range doc.Tools {
		if len(definition.InputSchema) == 0 {
			t.Errorf("tool %s did not receive provider input schema", definition.Name)
		}
		if len(definition.OutputSchema) == 0 {
			t.Errorf("tool %s did not receive provider output schema", definition.Name)
		}
		if definition.RuntimeDescription == "" {
			t.Errorf("tool %s did not receive provider description", definition.Name)
		}
		if definition.Governance == nil ||
			definition.Governance.Effect != toolgovernance.EffectRead ||
			definition.Governance.RiskLevel != toolgovernance.RiskLevelLow ||
			!definition.Governance.DataEgress ||
			definition.Governance.ExternalDestination != "api.exa.ai" ||
			definition.Governance.SensitiveDataAllowed {
			t.Errorf("tool %s did not receive provider governance classification: %#v", definition.Name, definition.Governance)
		}
		if _, hardcoded := SkillToolArgumentContractFor(SkillWebSearch, definition.Name); hardcoded {
			t.Errorf("tool %s unexpectedly has a hardcoded skill argument contract", definition.Name)
		}
	}

	metaTools := MetaToolsForSkillState(resolved, map[string]struct{}{SkillWebSearch: {}})
	callTool := findMetaTool(metaTools, MetaToolCallSkillTool)
	if callTool == nil {
		t.Fatal("call_skill_tool meta tool not found")
	}
	params, ok := callTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", callTool.Function.Parameters)
	}
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties missing")
	}
	if _, ok := properties["arguments"].(map[string]interface{}); !ok {
		t.Fatalf("arguments schema missing")
	}
	argumentSchemas := argumentSchemasFromPairBranches(params)
	if len(argumentSchemas) != 2 {
		t.Fatalf("pair branches = %#v, want two connector schemas", params["oneOf"])
	}
	if findSchemaWithRequired(argumentSchemas, "query") == nil {
		t.Fatalf("search schema requiring query not found in %#v", argumentSchemas)
	}
	if findSchemaWithRequired(argumentSchemas, "urls") == nil {
		t.Fatalf("fetch schema requiring urls not found in %#v", argumentSchemas)
	}
}

func TestConnectorProviderGovernanceOverridesStaticSkillClassification(t *testing.T) {
	manager := tools.NewToolManager(nil)
	provider := newWebSearchSchemaProviderForTest()
	providerGovernance := &tools.ToolGovernanceMetadata{
		ToolID:               "web.dynamic-delete",
		Effect:               string(toolgovernance.EffectDelete),
		RiskLevel:            string(toolgovernance.RiskLevelCritical),
		DataEgress:           false,
		ExternalDestination:  "",
		SensitiveDataAllowed: true,
	}
	for _, tool := range provider.tools {
		tool.entity.Governance = providerGovernance
	}
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWebSearch})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillWebSearch)
	if !ok {
		t.Fatal("web search skill was not resolved")
	}
	for _, definition := range doc.Tools {
		manifest := definition.Governance
		if manifest == nil {
			t.Fatalf("tool %s governance manifest is nil", definition.Name)
		}
		if manifest.Effect != toolgovernance.EffectDelete ||
			manifest.ToolID != "web.dynamic-delete" ||
			manifest.RiskLevel != toolgovernance.RiskLevelCritical ||
			manifest.DataEgress || manifest.ExternalDestination != "" ||
			!manifest.SensitiveDataAllowed {
			t.Errorf("tool %s retained downgraded static governance: %#v", definition.Name, manifest)
		}
	}
}

func TestConnectorProviderGovernanceCannotBeSkippedWhenStaticManifestIsMissing(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(newWebSearchSchemaProviderForTest()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWebSearch})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillWebSearch)
	if !ok {
		t.Fatal("web search skill was not resolved")
	}
	for index := range doc.Tools {
		doc.Tools[index].Governance = nil
	}
	runtime.attachProviderToolSchemas(context.Background(), doc)
	for _, definition := range doc.Tools {
		manifest := definition.Governance
		if manifest == nil {
			t.Fatalf("tool %s skipped provider governance", definition.Name)
		}
		if manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || !manifest.AuditRequired ||
			manifest.Effect != toolgovernance.EffectRead || !manifest.DataEgress || manifest.ExternalDestination != "api.exa.ai" {
			t.Fatalf("tool %s fallback governance = %#v", definition.Name, manifest)
		}
		if _, err := toolgovernance.ValidateManifest(*manifest); err != nil {
			t.Fatalf("tool %s fallback governance is invalid: %v", definition.Name, err)
		}
	}
}

func TestCallSkillToolSchemaDiscriminatesSameShapeArgumentsBySkillAndTool(t *testing.T) {
	querySchema := func() map[string]interface{} {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
			"required":             []string{"query"},
		}
	}
	metaTool := callSkillToolMetaTool(
		[]string{SkillWebSearch, SkillAgentKnowledge},
		[]string{"search_web", "retrieve_agent_knowledge"},
		nil,
		[]SkillToolArgumentContract{
			{SkillID: SkillWebSearch, ToolName: "search_web", Schema: querySchema()},
			{SkillID: SkillAgentKnowledge, ToolName: "retrieve_agent_knowledge", Schema: querySchema()},
		},
	)
	parameters, ok := metaTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T", metaTool.Function.Parameters)
	}
	if err := tools.ValidateJSONSchema(parameters); err != nil {
		t.Fatalf("meta tool schema is invalid: %v", err)
	}
	for _, payload := range []map[string]interface{}{
		{"skill_id": SkillWebSearch, "tool_name": "search_web", "arguments": map[string]interface{}{"query": "current release"}},
		{"skill_id": SkillAgentKnowledge, "tool_name": "retrieve_agent_knowledge", "arguments": map[string]interface{}{"query": "internal policy"}},
	} {
		if err := tools.ValidateJSONSchemaValue(parameters, payload); err != nil {
			t.Fatalf("discriminated payload %#v was rejected: %v", payload, err)
		}
	}
}

func TestWebSearchInvalidSensitiveArgumentsAreRedactedFromTraceAndError(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(newWebSearchSchemaProviderForTest()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWebSearch})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	secret := "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		SkillWebSearch,
		"search_web",
		map[string]interface{}{"query": "current information", secret: "x"},
		ExecutionContext{OrganizationID: "organization", UserID: "user"},
		"call_search",
	)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("CallSkillTool() error = %v, want sanitized validation failure", err)
	}
	if invocation == nil || invocation.Trace.Arguments["data_egress_redacted"] != true || invocation.Trace.Arguments["argument_count"] != 2 {
		t.Fatalf("trace arguments = %#v, want redacted data-egress values", invocation)
	}
	if encoded, marshalErr := json.Marshal(invocation.Trace); marshalErr != nil || strings.Contains(string(encoded), secret) {
		t.Fatalf("trace leaked sensitive arguments: %s, %v", encoded, marshalErr)
	}
}

type webSearchSchemaProviderForTest struct {
	tools map[string]*webSearchSchemaToolForTest
}

func newWebSearchSchemaProviderForTest() *webSearchSchemaProviderForTest {
	resultSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"results": map[string]interface{}{"type": "array"},
		},
		"required": []string{"results"},
	}
	return &webSearchSchemaProviderForTest{tools: map[string]*webSearchSchemaToolForTest{
		"search_web": {
			entity: tools.ToolEntity{
				Identity:    tools.ToolIdentity{Name: "search_web", Provider: SkillWebSearch, Label: tools.I18nText{"en_US": "Search Web"}},
				Description: tools.ToolDescription{LLM: "Search the public web."},
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query":       map[string]interface{}{"type": "string"},
						"num_results": map[string]interface{}{"type": "integer"},
					},
					"required":             []string{"query"},
					"additionalProperties": false,
				},
				OutputSchema: resultSchema,
				Governance:   webSearchGovernanceMetadataForTest(),
			},
		},
		"fetch_webpage": {
			entity: tools.ToolEntity{
				Identity:    tools.ToolIdentity{Name: "fetch_webpage", Provider: SkillWebSearch, Label: tools.I18nText{"en_US": "Fetch Webpage"}},
				Description: tools.ToolDescription{LLM: "Read selected public webpages."},
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"urls": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
					"required":             []string{"urls"},
					"additionalProperties": false,
				},
				OutputSchema: resultSchema,
				Governance:   webSearchGovernanceMetadataForTest(),
			},
		},
	}}
}

func webSearchGovernanceMetadataForTest() *tools.ToolGovernanceMetadata {
	return &tools.ToolGovernanceMetadata{
		ToolID:               "web.test",
		Effect:               string(toolgovernance.EffectRead),
		RiskLevel:            string(toolgovernance.RiskLevelLow),
		DataEgress:           true,
		ExternalDestination:  "api.exa.ai",
		SensitiveDataAllowed: false,
	}
}

func (p *webSearchSchemaProviderForTest) GetEntity() tools.ToolProviderEntity {
	entities := make([]tools.ToolEntity, 0, len(p.tools))
	for _, tool := range p.tools {
		entities = append(entities, tool.GetEntity())
	}
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        SkillWebSearch,
			Label:       tools.I18nText{"en_US": "Web Search"},
			Description: tools.I18nText{"en_US": "Web search test connector"},
		},
		ProviderType: tools.ToolProviderTypeConnector,
		Tools:        entities,
	}
}

func (p *webSearchSchemaProviderForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (p *webSearchSchemaProviderForTest) GetTool(name string) (tools.Tool, error) {
	tool, ok := p.tools[name]
	if !ok {
		return nil, tools.ErrToolNotFound
	}
	return tool, nil
}

func (p *webSearchSchemaProviderForTest) GetTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(p.tools))
	for _, tool := range p.tools {
		result = append(result, tool)
	}
	return result
}

func (p *webSearchSchemaProviderForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type webSearchSchemaToolForTest struct {
	entity  tools.ToolEntity
	runtime *tools.ToolRuntime
}

func (t *webSearchSchemaToolForTest) GetEntity() tools.ToolEntity {
	return t.entity
}

func (t *webSearchSchemaToolForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (t *webSearchSchemaToolForTest) GetTenantID() string {
	if t.runtime == nil {
		return ""
	}
	return t.runtime.TenantID
}

func (t *webSearchSchemaToolForTest) Invoke(context.Context, string, map[string]interface{}, *string, *string, *string) ([]tools.ToolInvokeMessage, error) {
	return nil, nil
}

func (t *webSearchSchemaToolForTest) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *webSearchSchemaToolForTest) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	clone := *t
	clone.runtime = runtime
	return &clone
}

func (t *webSearchSchemaToolForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}
