package chartgenerator

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "chart_generator"

// Provider is the builtin provider for generating chart files.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates a chart generator provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Chart Generator",
			"zh_Hans": "图表生成器",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for generating downloadable chart files",
			"zh_Hans": "用于生成可下载图表文件的内置工具",
		},
		Icon: "chart-no-axes-combined",
		Tags: []string{"visualization", "chart"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewGenerateChartTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
