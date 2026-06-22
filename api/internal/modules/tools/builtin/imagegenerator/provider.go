package imagegenerator

import (
	"context"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const ProviderID = "image_generator"

// Provider is the builtin provider for image generation tools.
type Provider struct {
	*builtin.BuiltinProvider
}

// NewProvider creates an image generator provider.
func NewProvider(fileService ReferenceImageFileService, llmClient llmclient.LLMClient, defaultModels defaultmodelservice.DefaultModelResolver) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Image Generator",
			"zh_Hans": "Image Generator",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for generating image files from prompts",
			"zh_Hans": "Built-in tools for generating image files from prompts",
		},
		Icon: "image-plus",
		Tags: []string{"image", "generation", "creative"},
	}

	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
	}
	provider.RegisterTool(NewGenerateImageTool("", fileService, llmClient, defaultModels))
	provider.RegisterTool(NewEditImageTool("", fileService, llmClient, defaultModels))
	return provider
}

// ValidateCredentials validates provider credentials.
func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
