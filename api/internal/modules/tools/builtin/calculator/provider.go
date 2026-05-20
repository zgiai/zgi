package calculator

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// Provider is the builtin provider for deterministic calculator tools.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates a calculator provider with all builtin calculator tools.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   "calculator",
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Calculator Tools",
			"zh_Hans": "计算器工具",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for deterministic arithmetic, expressions, and percentage calculations",
			"zh_Hans": "内置精确计算、表达式计算和百分比计算工具",
		},
		Icon: "calculator",
		Tags: []string{"utilities", "math"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewCalculateTool(""))
	provider.RegisterTool(NewPercentageTool(""))
	provider.RegisterTool(NewExpressionTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
