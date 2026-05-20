package time

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

// TimeProvider is the builtin provider for time-related tools
type TimeProvider struct {
	*builtin.BuiltinProvider
}

// NewTimeProvider creates a new TimeProvider
func NewTimeProvider() *TimeProvider {
	identity := tools.ToolProviderIdentity{
		Name:   "time",
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Time Tools",
			"zh_Hans": "时间工具",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for time operations",
			"zh_Hans": "内置时间操作工具",
		},
		Icon: "clock",
		Tags: []string{"utilities"},
	}

	provider := &TimeProvider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}

	// Register time tools
	provider.RegisterTool(NewCurrentTimeTool(""))
	provider.RegisterTool(NewDateCalculateTool(""))

	return provider
}

// GetEntity returns the provider entity (explicitly implementing the interface)
func (p *TimeProvider) GetEntity() tools.ToolProviderEntity {
	return p.BuiltinProvider.GetEntity()
}

// GetProviderType returns the provider type
func (p *TimeProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

// GetTool returns a tool by name
func (p *TimeProvider) GetTool(name string) (tools.Tool, error) {
	return p.BuiltinProvider.GetTool(name)
}

// GetTools returns all tools
func (p *TimeProvider) GetTools() []tools.Tool {
	return p.BuiltinProvider.GetTools()
}

// ValidateCredentials validates the provider credentials
func (p *TimeProvider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

// Ensure TimeProvider implements tools.ToolProvider interface
var _ tools.ToolProvider = (*TimeProvider)(nil)
