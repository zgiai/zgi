package toolprovider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type Provider struct {
	integrationID string
	entity        tools.ToolProviderEntity
	tools         map[string]*Tool
}

// NewProvider preserves the phase-one assembly contract while avoiding any
// Web Search special case. Multi-provider installations must use NewProviders
// or NewProviderForIntegration and register each returned provider.
func NewProvider(registry *integrations.Registry, executor *integrations.Executor) (*Provider, error) {
	if registry == nil || executor == nil {
		return nil, fmt.Errorf("integration registry and executor are required")
	}
	registrations := registry.Registrations()
	if len(registrations) == 0 {
		return nil, fmt.Errorf("integration registry has no registered providers")
	}
	if len(registrations) > 1 {
		return nil, fmt.Errorf("integration registry has multiple providers; use NewProviders")
	}
	return newProvider(registrations[0], executor)
}

func NewProviderForIntegration(registry *integrations.Registry, executor *integrations.Executor, integrationID string) (*Provider, error) {
	if registry == nil || executor == nil {
		return nil, fmt.Errorf("integration registry and executor are required")
	}
	registration, ok := registry.Registration(integrationID)
	if !ok {
		return nil, fmt.Errorf("integration %s is not registered", strings.TrimSpace(integrationID))
	}
	return newProvider(registration, executor)
}

func NewProviders(registry *integrations.Registry, executor *integrations.Executor) ([]*Provider, error) {
	if registry == nil || executor == nil {
		return nil, fmt.Errorf("integration registry and executor are required")
	}
	registrations := registry.Registrations()
	providers := make([]*Provider, 0, len(registrations))
	for _, registration := range registrations {
		provider, err := newProvider(registration, executor)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func newProvider(registration integrations.Registration, executor *integrations.Executor) (*Provider, error) {
	definition := registration.Definition
	if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.DriverID) == "" || len(definition.Actions) == 0 {
		return nil, fmt.Errorf("integration provider definition is incomplete")
	}
	providerTags := append([]string(nil), definition.Tags...)
	providerTags = append(providerTags, definition.Categories...)
	providerTags = uniqueStrings(providerTags)
	provider := &Provider{
		integrationID: definition.ID,
		entity: tools.ToolProviderEntity{
			Identity: tools.ToolProviderIdentity{
				Name:        definition.ID,
				Author:      definition.Author,
				Label:       toolI18nText(definition.Name, definition.NameI18n),
				Description: toolI18nText(definition.Description, definition.DescriptionI18n),
				Icon:        definition.Icon,
				Tags:        providerTags,
			},
			ProviderType: tools.ToolProviderTypeConnector,
		},
		tools: make(map[string]*Tool, len(definition.Actions)),
	}
	for _, action := range definition.Actions {
		toolTags := uniqueStrings(append(append([]string(nil), providerTags...), "external"))
		tool := &Tool{
			entity: tools.ToolEntity{
				Identity: tools.ToolIdentity{
					Name:     action.ToolName,
					Author:   definition.Author,
					Provider: definition.ID,
					Label:    toolI18nText(action.Name, action.NameI18n),
					Icon:     definition.Icon,
				},
				Description:  tools.ToolDescription{Human: toolI18nText(action.Description, action.DescriptionI18n), LLM: action.Description},
				InputSchema:  action.InputSchema,
				OutputSchema: action.OutputSchema,
				Governance: &tools.ToolGovernanceMetadata{
					ToolID:               action.ID,
					Effect:               string(action.Effect),
					RiskLevel:            string(action.RiskLevel),
					DataEgress:           action.DataEgress,
					ExternalDestination:  action.ExternalDestination,
					SensitiveDataAllowed: action.SensitiveDataAllowed,
				},
				OutputType: "json",
				Tags:       toolTags,
			},
			action:      action,
			executor:    executor,
			integration: definition.ID,
		}
		provider.tools[action.ToolName] = tool
		provider.entity.Tools = append(provider.entity.Tools, tool.entity)
	}
	sort.Slice(provider.entity.Tools, func(i, j int) bool {
		return provider.entity.Tools[i].Identity.Name < provider.entity.Tools[j].Identity.Name
	})
	return provider, nil
}

func (p *Provider) IntegrationID() string {
	if p == nil {
		return ""
	}
	return p.integrationID
}

func (p *Provider) GetEntity() tools.ToolProviderEntity { return p.entity }

func (p *Provider) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }

func (p *Provider) GetTool(name string) (tools.Tool, error) {
	tool, ok := p.tools[name]
	if !ok {
		return nil, tools.ErrToolNotFound
	}
	return tool, nil
}

func (p *Provider) GetTools() []tools.Tool {
	names := make([]string, 0, len(p.tools))
	for name := range p.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tools.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, p.tools[name])
	}
	return out
}

// Connector credentials are resolved from ConnectionID at invocation time;
// the generic ToolProvider credential hook must never accept or retain them.
func (p *Provider) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func toolI18nText(fallback string, localized integrations.LocalizedText) tools.I18nText {
	out := tools.I18nText{}
	for locale, value := range localized {
		switch strings.ToLower(strings.ReplaceAll(locale, "_", "-")) {
		case "en", "en-us":
			out["en_US"] = value
		case "zh", "zh-cn", "zh-hans":
			out["zh_Hans"] = value
		}
	}
	if _, exists := out["en_US"]; !exists && strings.TrimSpace(fallback) != "" {
		out["en_US"] = strings.TrimSpace(fallback)
	}
	return out
}
