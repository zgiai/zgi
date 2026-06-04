package filegenerator

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// Provider is the builtin provider for generating workflow files from text.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates a file generator provider.
func NewProvider() *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   "file_generator",
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "File Generator",
			"zh_Hans": "文件生成器",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for generating text-based workflow files",
			"zh_Hans": "用于生成文本类工作流文件的内置工具",
		},
		Icon: "file-plus",
		Tags: []string{"utilities", "file"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewGenerateFileTool(""))
	provider.RegisterTool(NewGenerateDocxTool(""))
	provider.RegisterTool(NewGeneratePDFTool(""))
	provider.RegisterTool(NewGeneratePPTXTool(""))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
