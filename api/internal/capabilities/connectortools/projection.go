// Package connectortools adapts native Connector discovery and safe results
// for host presentation. Execution is owned by the Connector dispatcher.
package connectortools

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type Provider struct {
	entity  tools.ToolProviderEntity
	entries []tools.Tool
}
type catalogEntry struct {
	entity tools.ToolEntity
	tenant string
}

func NewProvider(definition catalog.Definition) (*Provider, error) {
	if catalog.Validate(definition) != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	p := &Provider{entity: tools.ToolProviderEntity{ProviderType: tools.ToolProviderTypeConnector, Identity: tools.ToolProviderIdentity{Name: definition.ID, Author: definition.PluginID, Icon: definition.Icon, Label: labels(definition.Name, definition.NameI18n), Description: labels(definition.Description, definition.DescriptionI18n), Tags: []string{"connector", string(definition.Classification)}}}}
	for _, operation := range definition.Operations {
		entity, err := OperationEntity(definition, operation.ID)
		if err != nil {
			return nil, err
		}
		p.entries = append(p.entries, &catalogEntry{entity: entity})
		p.entity.Tools = append(p.entity.Tools, entity)
	}
	return p, nil
}
func OperationEntity(definition catalog.Definition, id string) (tools.ToolEntity, error) {
	for _, op := range definition.Operations {
		if op.ID == id {
			parameters, err := tools.JSONSchemaToToolParameters(op.InputSchema)
			if err != nil {
				return tools.ToolEntity{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
			}
			callers := []tools.ToolInvokeFrom{}
			for _, surface := range definition.Surfaces {
				switch surface {
				case catalog.SurfaceChat:
					callers = append(callers, tools.ToolInvokeFromAIChat)
				case catalog.SurfaceAgent:
					callers = append(callers, tools.ToolInvokeFromAgent)
				case catalog.SurfaceWorkflow:
					callers = append(callers, tools.ToolInvokeFromWorkflow)
				}
			}
			return tools.ToolEntity{Identity: tools.ToolIdentity{Name: op.ToolName, Provider: definition.ID, Author: definition.PluginID, Icon: definition.Icon, Label: labels(op.Name, op.NameI18n)}, ConnectorOperationID: op.ID, ConnectorOperationEffect: string(op.Effect), Description: tools.ToolDescription{Human: labels(op.Description, op.DescriptionI18n), LLM: op.Description}, Parameters: parameters, InputSchema: op.InputSchema, OutputSchema: op.OutputSchema, WorkflowOutputs: tools.CompileWorkflowOutputs(op.OutputSchema), SupportedCallers: callers, OutputType: "json", Tags: []string{"connector", string(op.Effect)}}, nil
		}
	}
	return tools.ToolEntity{}, tools.ErrToolNotFound
}
func labels(fallback string, localized catalog.LocalizedText) tools.I18nText {
	result := tools.I18nText{"en_US": fallback, "zh_Hans": fallback}
	for locale, text := range localized {
		result[strings.ReplaceAll(locale, "-", "_")] = text
	}
	return result
}
func (p *Provider) GetEntity() tools.ToolProviderEntity   { return p.entity }
func (*Provider) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }
func (p *Provider) GetTools() []tools.Tool                { return append([]tools.Tool(nil), p.entries...) }
func (p *Provider) GetTool(name string) (tools.Tool, error) {
	for _, entry := range p.entries {
		if entry.GetEntity().Identity.Name == name {
			return entry, nil
		}
	}
	return nil, tools.ErrToolNotFound
}
func (*Provider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return core.NewError(core.ErrorCodeInvalidInput, false, nil)
}
func (entry *catalogEntry) GetEntity() tools.ToolEntity       { return entry.entity }
func (*catalogEntry) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }
func (entry *catalogEntry) GetTenantID() string               { return entry.tenant }
func (*catalogEntry) Invoke(context.Context, string, map[string]interface{}, *string, *string, *string) ([]tools.ToolInvokeMessage, error) {
	return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
}
func (entry *catalogEntry) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return entry.entity.Parameters, nil
}
func (entry *catalogEntry) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	copy := *entry
	if runtime != nil {
		copy.tenant = runtime.TenantID
	}
	return &copy
}
func (*catalogEntry) ValidateCredentials(context.Context, map[string]interface{}) error {
	return core.NewError(core.ErrorCodeInvalidInput, false, nil)
}
