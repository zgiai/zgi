package sensitiveredaction

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "sensitive_redaction"

// Provider is the builtin provider for sensitive information redaction.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates a sensitive redaction provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Sensitive Redaction",
			"zh_Hans": "Sensitive Redaction",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for detecting and redacting sensitive text.",
			"zh_Hans": "Built-in tools for detecting and redacting sensitive text.",
		},
		Icon: "shield-alert",
		Tags: []string{"privacy", "security", "redaction"},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewRedactTextTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
