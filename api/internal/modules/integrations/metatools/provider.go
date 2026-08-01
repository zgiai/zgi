package metatools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	ProviderID             = integrations.MetaProviderExternalIntegrations
	ToolListConnections    = "list_connections"
	ToolSearchActions      = "search_actions"
	ToolGetActionGuide     = "get_action_guide"
	ToolExecuteAction      = "execute_action"
	preferredSelector      = "preferred"
	maxSelectedConnections = 64
	maxRuntimeSelections   = 640
)

// ActionExecutor is deliberately narrower than *integrations.Executor so the
// hidden provider cannot reach credential or audit internals.
type ActionExecutor interface {
	Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error)
}

type ConnectionLookup interface {
	GetByID(context.Context, uuid.UUID, uuid.UUID) (*integrations.IntegrationConnection, error)
}

// ConnectionAuthorizer covers both discovery-time visibility and action-time
// authorization. The Executor performs the same action authorization again at
// the final execution boundary when assembled with this service.
type ConnectionAuthorizer interface {
	integrations.ConnectionPreferenceAccessChecker
	integrations.ConnectionAccessAuthorizer
}

type Provider struct {
	registry    *integrations.Registry
	executor    ActionExecutor
	connections ConnectionLookup
	access      ConnectionAuthorizer
	policies    integrations.ActionPolicyResolver
	entity      tools.ToolProviderEntity
	tools       map[string]*Tool
}

func NewProvider(
	registry *integrations.Registry,
	executor ActionExecutor,
	connections ConnectionLookup,
	access ConnectionAuthorizer,
	policies integrations.ActionPolicyResolver,
) (*Provider, error) {
	if registry == nil || executor == nil || connections == nil || access == nil || policies == nil {
		return nil, fmt.Errorf("external integration meta tools require registry, executor, connection lookup, access authorizer, and action policy resolver")
	}
	provider := &Provider{
		registry: registry, executor: executor, connections: connections, access: access, policies: policies,
		tools: make(map[string]*Tool, 4),
		entity: tools.ToolProviderEntity{
			Identity: tools.ToolProviderIdentity{
				Name:        ProviderID,
				Author:      "ZGI",
				Label:       tools.I18nText{"en_US": "Connected Apps", "zh_Hans": "已连接应用"},
				Description: tools.I18nText{"en_US": "Discover and use actions from connections selected for this chat.", "zh_Hans": "发现并使用当前聊天已选择连接中的操作。"},
				Icon:        "plug",
				Tags:        []string{"external", "runtime"},
			},
			ProviderType: tools.ToolProviderTypeConnector,
		},
	}
	for _, spec := range toolSpecs() {
		tool := &Tool{
			name: spec.name, entity: spec.entity,
			registry: registry, executor: executor, connections: connections, access: access, policies: policies,
		}
		provider.tools[spec.name] = tool
		provider.entity.Tools = append(provider.entity.Tools, spec.entity)
	}
	sort.Slice(provider.entity.Tools, func(i, j int) bool {
		return provider.entity.Tools[i].Identity.Name < provider.entity.Tools[j].Identity.Name
	})
	return provider, nil
}

func (p *Provider) GetEntity() tools.ToolProviderEntity { return p.entity }

func (p *Provider) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }

func (p *Provider) GetTool(name string) (tools.Tool, error) {
	tool, ok := p.tools[strings.ToLower(strings.TrimSpace(name))]
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

// Meta tools never own provider credentials. Connection secrets are resolved
// request-locally by integrations.Executor.
func (p *Provider) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

type toolSpec struct {
	name   string
	entity tools.ToolEntity
}

func toolSpecs() []toolSpec {
	return []toolSpec{
		newToolSpec(ToolListConnections, "List selected app connections", "列出已选择的应用连接", "List the app connections explicitly selected for this chat that the current account may access.", "列出当前聊天已明确选择且当前账户有权访问的应用连接。", listConnectionsInputSchema(), listConnectionsOutputSchema(), "integration.catalog.list_connections", "read", "low", false, ""),
		newToolSpec(ToolSearchActions, "Search connected-app actions", "搜索已连接应用的操作", "Search actions exposed by the app connections explicitly selected for this chat.", "搜索当前聊天已明确选择的应用连接所提供的操作。", searchActionsInputSchema(), searchActionsOutputSchema(), "integration.catalog.search_actions", "read", "low", false, ""),
		newToolSpec(ToolGetActionGuide, "Get an action guide", "获取操作指南", "Read the current schema and governance summary for one available connected-app action before executing it.", "执行前读取某个可用应用操作的当前参数结构与治理摘要。", actionGuideInputSchema(), actionGuideOutputSchema(), "integration.catalog.get_action_guide", "read", "low", false, ""),
		// The static classification is intentionally conservative. The shared
		// GovernanceManifestResolver replaces it with the selected action's
		// provider-authoritative classification before a decision is made.
		newToolSpec(ToolExecuteAction, "Execute a connected-app action", "执行已连接应用的操作", "Execute one action through its selected connection after dynamic governance and authorization checks.", "通过选定连接执行一项操作，并在执行前完成动态治理与授权检查。", executeActionInputSchema(), executeActionOutputSchema(), "integration.execute_dynamic", "invoke", "high", true, "external-provider"),
	}
}

func newToolSpec(name, enLabel, zhLabel, enDescription, zhDescription string, input, output map[string]interface{}, toolID, effect, risk string, dataEgress bool, destination string) toolSpec {
	return toolSpec{name: name, entity: tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name: name, Provider: ProviderID, Author: "ZGI", Icon: "plug",
			Label: tools.I18nText{"en_US": enLabel, "zh_Hans": zhLabel},
		},
		Description: tools.ToolDescription{Human: tools.I18nText{"en_US": enDescription, "zh_Hans": zhDescription}, LLM: enDescription},
		InputSchema: input, OutputSchema: output,
		Governance: &tools.ToolGovernanceMetadata{
			ToolID: toolID, Effect: effect, RiskLevel: risk, DataEgress: dataEgress,
			ExternalDestination: destination, SensitiveDataAllowed: false,
		},
		OutputType: "json", Tags: []string{"external", "runtime"},
	}}
}
