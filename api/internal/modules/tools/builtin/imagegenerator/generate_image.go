package imagegenerator

import (
	"context"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// GenerateImageTool generates image files from a text prompt.
type GenerateImageTool struct {
	*builtin.BuiltinTool
	imageToolBase
}

// NewGenerateImageTool creates a generate_image tool.
func NewGenerateImageTool(tenantID string, fileService ReferenceImageFileService, llmClient llmclient.LLMClient, defaultModels defaultmodelservice.DefaultModelResolver) *GenerateImageTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_image",
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": "Generate Image", "zh_Hans": "Generate Image"},
			Icon:     "image-plus",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Generate downloadable image files from a text prompt.", "zh_Hans": "Generate downloadable image files from a text prompt."},
			LLM:   "Generate downloadable image files from a prompt. Supports broad styles, aspect ratios 1:1/16:9/9:16/4:3, 1-4 candidates, optional negative prompt, and optional current-user reference image URL guidance. Reference images are passed as signed URLs in the prompt, not as structured image inputs. For casual, vague, incomplete, or non-professional image requests, use prompt-professionalizer before this tool; for generic image requests, call request_user_input before this tool. Do not use for OCR, image understanding, table extraction, screenshot diagnosis, illegal/sexual/graphic/infringing imagery, or specific living-person likeness generation.",
		},
		Parameters: imageGenerationParameters(false),
		OutputType: "file",
		Tags:       []string{"image", "generation", "creative"},
	}
	return &GenerateImageTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, tenantID),
		imageToolBase: imageToolBase{
			fileService:   fileService,
			llmClient:     llmClient,
			defaultModels: defaultModels,
		},
	}
}

func (t *GenerateImageTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGenerateImageTool(tenantID, t.fileService, t.llmClient, t.defaultModels)
	fork.runtime = runtime
	return fork
}

// Invoke generates the requested images and returns workflow file messages.
func (t *GenerateImageTool) Invoke(ctx context.Context, userID string, toolParameters map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = messageID
	req, err := parseGenerationRequest(toolParameters, "prompt")
	if err != nil {
		return nil, err
	}
	if hasReferenceImage(toolParameters) {
		refURL, refName, err := resolveReferenceImage(ctx, t.fileService, toolParameters, firstNonEmpty(t.GetTenantID(), runtimeTenantID(t.runtime)), userID, "reference_image")
		if err != nil {
			return nil, err
		}
		req.ReferenceURL = refURL
		req.ReferenceName = refName
	}
	return t.invokeGeneration(ctx, firstNonEmpty(t.GetTenantID(), runtimeTenantID(t.runtime)), userID, conversationID, appID, toolParameters, req)
}

func hasReferenceImage(params map[string]interface{}) bool {
	return strings.TrimSpace(fileIDFromAny(rawAnyParam(params, "reference_image"))) != ""
}

func imageGenerationParameters(edit bool) []tools.ToolParameter {
	params := []tools.ToolParameter{}
	if edit {
		params = append(params,
			tools.ToolParameter{Name: "image", Label: tools.I18nText{"en_US": "Image", "zh_Hans": "Image"}, HumanDescription: tools.I18nText{"en_US": "Reference image file object.", "zh_Hans": "Reference image file object."}, LLMDescription: "Required current-user reference image file object or file ID. It may contain upload_file_id, file_id, id, or related_id. The tool places a signed URL in the prompt; it is not a structured image input.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true},
			tools.ToolParameter{Name: "edit_instruction", Label: tools.I18nText{"en_US": "Edit Instruction", "zh_Hans": "Edit Instruction"}, HumanDescription: tools.I18nText{"en_US": "Requested edit or variant instruction.", "zh_Hans": "Requested edit or variant instruction."}, LLMDescription: "Required edit or variant instruction. State what to change, what to preserve, and what to avoid.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true},
			tools.ToolParameter{Name: "edit_type", Label: tools.I18nText{"en_US": "Edit Type", "zh_Hans": "Edit Type"}, HumanDescription: tools.I18nText{"en_US": "Edit type.", "zh_Hans": "Edit type."}, LLMDescription: "Edit type: auto, variant, background, color, add_element, remove_element, or style_transfer.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: "auto", SupportVariable: true, Options: editTypeOptions()},
		)
	} else {
		params = append(params,
			tools.ToolParameter{Name: "prompt", Label: tools.I18nText{"en_US": "Prompt", "zh_Hans": "Prompt"}, HumanDescription: tools.I18nText{"en_US": "Image description prompt.", "zh_Hans": "Image description prompt."}, LLMDescription: "Required image description prompt. Include subject, scene, composition, style, and intended use.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true},
			tools.ToolParameter{Name: "reference_image", Label: tools.I18nText{"en_US": "Reference Image", "zh_Hans": "Reference Image"}, HumanDescription: tools.I18nText{"en_US": "Optional reference image file object.", "zh_Hans": "Optional reference image file object."}, LLMDescription: "Optional current-user reference image file object or file ID. The tool places a signed URL in the prompt for loose visual guidance; it is not a structured image input.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
		)
	}
	params = append(params,
		tools.ToolParameter{Name: "style", Label: tools.I18nText{"en_US": "Style", "zh_Hans": "Style"}, HumanDescription: tools.I18nText{"en_US": "Visual style.", "zh_Hans": "Visual style."}, LLMDescription: "Visual style: auto, realistic, illustration, flat, 3d, guofeng, tech, poster, product, icon, or cover.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: "auto", SupportVariable: true, Options: styleOptions()},
		tools.ToolParameter{Name: "aspect_ratio", Label: tools.I18nText{"en_US": "Aspect Ratio", "zh_Hans": "Aspect Ratio"}, HumanDescription: tools.I18nText{"en_US": "Image aspect ratio.", "zh_Hans": "Image aspect ratio."}, LLMDescription: "Aspect ratio: 1:1, 16:9, 9:16, or 4:3. Defaults to 1:1.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: defaultAspectRatio, SupportVariable: true, Options: ratioOptions()},
		tools.ToolParameter{Name: "count", Label: tools.I18nText{"en_US": "Count", "zh_Hans": "Count"}, HumanDescription: tools.I18nText{"en_US": "Number of candidate images.", "zh_Hans": "Number of candidate images."}, LLMDescription: "Number of candidate images to generate. Must be between 1 and 4. Defaults to 1.", Type: tools.ToolParameterTypeNumber, Form: tools.ToolParameterFormLLM, Required: false, Default: fmt.Sprintf("%d", defaultImageCount), SupportVariable: true},
		tools.ToolParameter{Name: "negative_prompt", Label: tools.I18nText{"en_US": "Negative Prompt", "zh_Hans": "Negative Prompt"}, HumanDescription: tools.I18nText{"en_US": "Elements to avoid.", "zh_Hans": "Elements to avoid."}, LLMDescription: "Optional prohibited elements or undesired visual traits.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
		tools.ToolParameter{Name: "filename", Label: tools.I18nText{"en_US": "Filename", "zh_Hans": "Filename"}, HumanDescription: tools.I18nText{"en_US": "Optional base filename.", "zh_Hans": "Optional base filename."}, LLMDescription: "Optional base filename. Do not include path separators or an extension.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
		tools.ToolParameter{Name: "lifecycle", Label: tools.I18nText{"en_US": "Lifecycle", "zh_Hans": "Lifecycle"}, HumanDescription: tools.I18nText{"en_US": "File lifecycle.", "zh_Hans": "File lifecycle."}, LLMDescription: "File lifecycle: persistent or temporary. Defaults to persistent.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: "persistent", SupportVariable: true, Options: lifecycleOptions()},
		tools.ToolParameter{Name: "provider", Label: tools.I18nText{"en_US": "Provider", "zh_Hans": "Provider"}, HumanDescription: tools.I18nText{"en_US": "Optional explicit image model provider.", "zh_Hans": "Optional explicit image model provider."}, LLMDescription: "Optional explicit image model provider. Usually omit this and use the default image generation model.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
		tools.ToolParameter{Name: "model", Label: tools.I18nText{"en_US": "Model", "zh_Hans": "Model"}, HumanDescription: tools.I18nText{"en_US": "Optional explicit image model.", "zh_Hans": "Optional explicit image model."}, LLMDescription: "Optional explicit image generation model. Usually omit this and use the default image generation model.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
	)
	return params
}

func styleOptions() []tools.ToolParameterOption {
	values := []string{"auto", "realistic", "illustration", "flat", "3d", "guofeng", "tech", "poster", "product", "icon", "cover"}
	out := make([]tools.ToolParameterOption, 0, len(values))
	for _, value := range values {
		out = append(out, tools.ToolParameterOption{Value: value, Label: tools.I18nText{"en_US": value, "zh_Hans": value}})
	}
	return out
}

func ratioOptions() []tools.ToolParameterOption {
	return []tools.ToolParameterOption{
		{Value: "1:1", Label: tools.I18nText{"en_US": "1:1", "zh_Hans": "1:1"}},
		{Value: "16:9", Label: tools.I18nText{"en_US": "16:9", "zh_Hans": "16:9"}},
		{Value: "9:16", Label: tools.I18nText{"en_US": "9:16", "zh_Hans": "9:16"}},
		{Value: "4:3", Label: tools.I18nText{"en_US": "4:3", "zh_Hans": "4:3"}},
	}
}

func lifecycleOptions() []tools.ToolParameterOption {
	return []tools.ToolParameterOption{
		{Value: "persistent", Label: tools.I18nText{"en_US": "persistent", "zh_Hans": "persistent"}},
		{Value: "temporary", Label: tools.I18nText{"en_US": "temporary", "zh_Hans": "temporary"}},
	}
}

func editTypeOptions() []tools.ToolParameterOption {
	values := []string{"auto", "variant", "background", "color", "add_element", "remove_element", "style_transfer"}
	out := make([]tools.ToolParameterOption, 0, len(values))
	for _, value := range values {
		out = append(out, tools.ToolParameterOption{Value: value, Label: tools.I18nText{"en_US": value, "zh_Hans": value}})
	}
	return out
}
