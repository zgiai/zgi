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

// EditImageTool creates reference-image variants or edit-style regenerated images.
type EditImageTool struct {
	*builtin.BuiltinTool
	imageToolBase
}

// NewEditImageTool creates an edit_image tool.
func NewEditImageTool(tenantID string, fileService ReferenceImageFileService, llmClient llmclient.LLMClient, defaultModels defaultmodelservice.DefaultModelResolver) *EditImageTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "edit_image",
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": "Edit Image", "zh_Hans": "Edit Image"},
			Icon:     "wand-sparkles",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Create reference-image variants or edit-style regenerated images.", "zh_Hans": "Create reference-image variants or edit-style regenerated images."},
			LLM:   "Create prompt-plus-reference-URL variants or edit-style regenerated images from a current-user reference image and instruction. Supports background, color, add_element, remove_element, style_transfer, and variant requests as regeneration guidance only. This is not precise in-place editing and does not pass structured image input to the provider. For ambiguous edits, call request_user_input before this tool. Do not use for OCR, image understanding, illegal/sexual/graphic/infringing imagery, or specific living-person likeness generation.",
		},
		Parameters: imageGenerationParameters(true),
		OutputType: "file",
		Tags:       []string{"image", "edit", "generation"},
	}
	return &EditImageTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, tenantID),
		imageToolBase: imageToolBase{
			fileService:   fileService,
			llmClient:     llmClient,
			defaultModels: defaultModels,
		},
	}
}

func (t *EditImageTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewEditImageTool(tenantID, t.fileService, t.llmClient, t.defaultModels)
	fork.runtime = runtime
	return fork
}

// Invoke creates the requested reference-image edit/variant and returns workflow file messages.
func (t *EditImageTool) Invoke(ctx context.Context, userID string, toolParameters map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = messageID
	req, err := parseGenerationRequest(toolParameters, "edit_instruction")
	if err != nil {
		return nil, err
	}
	editType, err := normalizeEditType(rawStringParam(toolParameters, "edit_type"))
	if err != nil {
		return nil, err
	}
	req.EditType = editType
	tenantID := firstNonEmpty(t.GetTenantID(), runtimeTenantID(t.runtime))
	refURL, refName, err := resolveReferenceImage(ctx, t.fileService, toolParameters, tenantID, userID, "image")
	if err != nil {
		return nil, err
	}
	req.ReferenceURL = refURL
	req.ReferenceName = refName
	return t.invokeGeneration(ctx, tenantID, userID, conversationID, appID, toolParameters, req)
}

func normalizeEditType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return "auto", nil
	case "variant", "background", "color", "add_element", "remove_element", "style_transfer":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	case "style-transfer":
		return "style_transfer", nil
	default:
		return "", fmt.Errorf("unsupported edit_type: %s", raw)
	}
}
