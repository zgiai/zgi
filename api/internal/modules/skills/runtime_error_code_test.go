package skills_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestCallSkillToolPublishesTypedIntegrationErrorCodeWithoutProviderMessage(t *testing.T) {
	providerMessage := "GitHub rejected secret credential ghp_do_not_expose"
	upstreamCause := errors.New("authorization header contained sensitive provider response")
	integrationErr := integrations.NewError(integrations.ErrorCodeAuthInvalid, providerMessage, upstreamCause)
	runtime, resolved := runtimeWithFailingTool(t, fmt.Errorf("connector invoke failed: %w", integrationErr))

	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"external-apps-test",
		"execute_action",
		map[string]interface{}{},
		skills.ExecutionContext{OrganizationID: "organization-1", UserID: "account-1"},
		"call-1",
	)
	if err == nil {
		t.Fatal("CallSkillTool() error = nil, want typed integration failure")
	}
	if invocation == nil {
		t.Fatal("CallSkillTool() invocation = nil")
	}
	if got := invocation.Trace.ErrorCode; got != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("trace error_code = %q, want %q", got, integrations.ErrorCodeAuthInvalid)
	}
	if got := invocation.Trace.Error; got != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("trace error = %q, want stable public code", got)
	}
	if invocation.Trace.Error == providerMessage || invocation.Trace.Error == upstreamCause.Error() {
		t.Fatalf("trace exposed provider detail: %#v", invocation.Trace)
	}
}

func TestCallSkillToolPreservesOrdinaryErrorMessageWithoutErrorCode(t *testing.T) {
	runtime, resolved := runtimeWithFailingTool(t, errors.New("ordinary tool failure"))

	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"external-apps-test",
		"execute_action",
		map[string]interface{}{},
		skills.ExecutionContext{OrganizationID: "organization-1", UserID: "account-1"},
		"call-1",
	)
	if err == nil || invocation == nil {
		t.Fatalf("CallSkillTool() invocation=%#v error=%v", invocation, err)
	}
	if got := invocation.Trace.Error; got != "ordinary tool failure" {
		t.Fatalf("trace error = %q, want ordinary tool failure", got)
	}
	if invocation.Trace.ErrorCode != "" {
		t.Fatalf("trace error_code = %q, want empty", invocation.Trace.ErrorCode)
	}
}

func runtimeWithFailingTool(t *testing.T, invokeErr error) (*skills.Runtime, *skills.ResolvedSkills) {
	t.Helper()
	tool := &failingTool{invokeErr: invokeErr}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&failingProvider{tool: tool}); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	doc := skills.SkillDocument{
		Metadata: skills.SkillMetadata{ID: "external-apps-test"},
		Tools: []skills.SkillToolDefinition{{
			Name:         "execute_action",
			ProviderType: tools.ToolProviderTypeConnector,
			ProviderID:   "external-apps-test",
			InputSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
			},
		}},
	}
	return skills.NewRuntime(tools.NewToolEngine(manager), manager), &skills.ResolvedSkills{Skills: []skills.SkillDocument{doc}}
}

type failingProvider struct {
	tool tools.Tool
}

func (p *failingProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "external-apps-test",
			Label:       tools.I18nText{"en_US": "External Apps Test"},
			Description: tools.I18nText{"en_US": "External apps error contract test"},
		},
		ProviderType: tools.ToolProviderTypeConnector,
	}
}

func (p *failingProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (p *failingProvider) GetTool(name string) (tools.Tool, error) {
	if name != "execute_action" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *failingProvider) GetTools() []tools.Tool { return []tools.Tool{p.tool} }

func (p *failingProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type failingTool struct {
	runtime   *tools.ToolRuntime
	invokeErr error
}

func (t *failingTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "execute_action",
			Provider: "external-apps-test",
			Label:    tools.I18nText{"en_US": "Execute action"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Execute a test action"},
			LLM:   "Execute a test action.",
		},
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
		},
	}
}

func (t *failingTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (t *failingTool) GetTenantID() string {
	if t.runtime == nil {
		return ""
	}
	return t.runtime.TenantID
}

func (t *failingTool) Invoke(context.Context, string, map[string]interface{}, *string, *string, *string) ([]tools.ToolInvokeMessage, error) {
	return nil, t.invokeErr
}

func (t *failingTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *failingTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &failingTool{runtime: runtime, invokeErr: t.invokeErr}
}

func (t *failingTool) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }
