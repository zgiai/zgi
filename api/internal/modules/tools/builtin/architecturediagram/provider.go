package architecturediagram

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "architecture_diagram_generator"

// Provider is the builtin provider for generating technical diagram files.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates an architecture diagram provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Architecture Diagram Generator",
			"zh_Hans": "Architecture Diagram Generator",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for generating downloadable SVG and HTML technical diagram files",
			"zh_Hans": "Built-in tools for generating downloadable SVG and HTML technical diagram files",
		},
		Icon: "workflow",
		Tags: []string{"visualization", "diagram", "architecture"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewGenerateArchitectureDiagramTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
