package intentrouter

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "intent_router"

// Provider is the builtin provider for intent classification routing.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates the intent router provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US": "Intent Router",
		},
		Description: tools.I18nText{
			"en_US": "Built-in tools for validating and normalizing intent routing results",
		},
		Icon: "route",
		Tags: []string{"intent", "routing", "classification"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewRouteIntentTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
