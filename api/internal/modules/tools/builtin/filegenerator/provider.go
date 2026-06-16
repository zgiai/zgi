package filegenerator

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// Provider is the builtin provider for generating workflow files from text.
type Provider struct {
	*builtin.BuiltinProvider
	services fileGeneratorServices
}

type ProviderOption func(*Provider)

func WithManagedFileServices(fileService ManagedFileService, workspacePerms WorkspacePermissionService, folders ManagedFileFolderService) ProviderOption {
	return func(p *Provider) {
		p.services.managedFiles = fileService
		p.services.workspacePerms = workspacePerms
		p.services.folders = folders
	}
}

// NewProvider creates a file generator provider.
func NewProvider(options ...ProviderOption) *Provider {
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
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	provider.RegisterTool(NewGenerateFileTool("").withServices(provider.services))
	provider.RegisterTool(NewGenerateDocxTool("").withServices(provider.services))
	provider.RegisterTool(NewGeneratePDFTool("").withServices(provider.services))
	provider.RegisterTool(NewGeneratePPTXTool("").withServices(provider.services))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
