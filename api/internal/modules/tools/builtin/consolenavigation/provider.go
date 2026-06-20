package consolenavigation

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// Provider exposes safe console navigation helpers for AIChat.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates the console navigation builtin provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   "console_navigation",
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Console Navigation",
			"zh_Hans": "Console Navigation",
		},
		Description: tools.I18nText{
			"en_US":   "Safe internal ZGI console page navigation requests",
			"zh_Hans": "Safe internal ZGI console page navigation requests",
		},
		Icon: "route",
		Tags: []string{"console", "navigation", "aichat"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewNavigateTool(""))
	return provider
}

func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
