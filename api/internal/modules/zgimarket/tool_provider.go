package zgimarket

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// MarketToolProvider resolves the market tool provider a runtime tool definition names. The
// provider name names the subject and the listing; whether the subject may use the listing is
// checked against its binding on every call.
func (t *Tools) MarketToolProvider(_ context.Context, organizationID, name string) (tools.ToolProvider, error) {
	if t == nil || t.gateway == nil {
		return nil, ErrMarketToolsUnavailable
	}
	organization, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization: %w", err)
	}
	if _, _, _, ok := parseProviderName(name); !ok {
		return nil, ErrListingNotBindable
	}
	return &marketProvider{tools: t, organizationID: organization, name: name}, nil
}

// ToolNamePrefix is what the runtime puts before a tool name that two bound listings share.
func ToolNamePrefix(slug string) string {
	return strings.ReplaceAll(slug, "-", "_") + "__"
}

type marketProvider struct {
	tools          *Tools
	organizationID uuid.UUID
	name           string
}

func (p *marketProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: p.name, Author: "zgi-market", Tags: []string{"zgi-market"}},
		ProviderType: tools.ToolProviderTypeMarket,
	}
}

func (*marketProvider) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeMarket }

// GetTool returns the tool without asking the gateway: the gateway refuses a tool the listing
// does not have, and the runtime only offers the tools the gateway listed.
func (p *marketProvider) GetTool(name string) (tools.Tool, error) {
	// The runtime tells two bound MCP listings with the same tool name apart by a listing prefix.
	// The tool of an API operation already names its offering.
	if _, _, slug, ok := parseProviderName(p.name); ok && !strings.HasPrefix(slug, apiProviderPrefix) {
		name = strings.TrimPrefix(name, ToolNamePrefix(slug))
	}
	if !toolNamePattern.MatchString(name) {
		return nil, fmt.Errorf("market tool %q is not valid", name)
	}
	return &marketTool{provider: p, name: name}, nil
}

func (*marketProvider) GetTools() []tools.Tool { return nil }

func (*marketProvider) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

type marketTool struct {
	provider *marketProvider
	name     string
	runtime  *tools.ToolRuntime
}

func (t *marketTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{Identity: tools.ToolIdentity{Name: t.name, Provider: t.provider.name, Author: "zgi-market"}}
}

func (*marketTool) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeMarket }

func (t *marketTool) GetTenantID() string { return t.provider.organizationID.String() }

func (t *marketTool) Invoke(
	ctx context.Context, _ string, parameters map[string]interface{}, _ *string, _ *string, _ *string,
) ([]tools.ToolInvokeMessage, error) {
	result, err := t.provider.tools.callTool(ctx, t.provider.organizationID, t.provider.name, t.name, parameters)
	if err != nil {
		return nil, fmt.Errorf("the ZGI Market tool could not be called: %w", err)
	}
	message := tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeText, Text: result.Text}
	if result.IsError {
		return []tools.ToolInvokeMessage{message}, fmt.Errorf("the ZGI Market tool answered an error: %s", result.Text)
	}
	return []tools.ToolInvokeMessage{message}, nil
}

func (*marketTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *marketTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &marketTool{provider: t.provider, name: t.name, runtime: runtime}
}

func (*marketTool) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }
