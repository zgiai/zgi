package filegenerator

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// GeneratePPTXTool creates editable PowerPoint presentations in the workflow tool file store.
type GeneratePPTXTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGeneratePPTXTool creates a generate_pptx tool.
func NewGeneratePPTXTool(tenantID string) *GeneratePPTXTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_pptx",
			Author:   "System",
			Provider: "file_generator",
			Label: tools.I18nText{
				"en_US":   "Generate Presentation",
				"zh_Hans": "生成演示文稿",
			},
			Icon: "file-plus",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate an editable PPTX presentation from a structured specification.",
				"zh_Hans": "根据结构化规格生成可编辑的 PPTX 演示文稿。",
			},
			LLM: "Generate an editable static PPTX presentation from a JSON presentation specification. Use this when the user asks for PowerPoint, slides, or a presentation deck. Supports text, title, basic table, and simple shape elements; readable content must use non-overlapping boxes. Animations and speaker notes are not supported.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "presentation",
				Label:            tools.I18nText{"en_US": "Presentation", "zh_Hans": "演示文稿规格"},
				HumanDescription: tools.I18nText{"en_US": "PPTX presentation specification as JSON.", "zh_Hans": "JSON 格式的 PPTX 演示文稿规格。"},
				LLMDescription:   "JSON string describing the PPTX presentation. Include slides with elements of type title, text, table, or shape. Use non-overlapping boxes for readable content; omitted boxes use simple auto layout.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": `{"slides":[...]}`, "zh_Hans": `{"slides":[...]}`},
				SupportVariable:  true,
			},
			{
				Name:             "filename",
				Label:            tools.I18nText{"en_US": "Filename", "zh_Hans": "文件名"},
				HumanDescription: tools.I18nText{"en_US": "Optional display filename. The .pptx extension is added or corrected automatically.", "zh_Hans": "可选展示文件名，扩展名会自动补齐或修正为 .pptx。"},
				LLMDescription:   "Optional display filename. Do not include path separators or an extension.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "deck", "zh_Hans": "deck"},
				SupportVariable:  true,
			},
			{
				Name:             "title",
				Label:            tools.I18nText{"en_US": "Title", "zh_Hans": "标题"},
				HumanDescription: tools.I18nText{"en_US": "Optional title hint. Visible content must be included in the presentation specification.", "zh_Hans": "可选标题提示。可见内容必须写入演示文稿规格。"},
				LLMDescription:   "Optional title hint. The visible slide content must be included in presentation.slides.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "Deck", "zh_Hans": "Deck"},
				SupportVariable:  true,
			},
			{
				Name:             "lifecycle",
				Label:            tools.I18nText{"en_US": "Lifecycle", "zh_Hans": "生命周期"},
				HumanDescription: tools.I18nText{"en_US": "Whether the generated file is persistent or temporary.", "zh_Hans": "生成文件是持久保存还是临时保存。"},
				LLMDescription:   "File lifecycle: persistent or temporary. Defaults to persistent.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "persistent",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "persistent", Label: tools.I18nText{"en_US": "Persistent", "zh_Hans": "持久保存"}},
					{Value: "temporary", Label: tools.I18nText{"en_US": "Temporary", "zh_Hans": "临时文件"}},
				},
			},
		},
		OutputType: "file",
		Tags:       []string{"utilities", "file", "pptx", "presentation"},
	}
	return &GeneratePPTXTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *GeneratePPTXTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGeneratePPTXTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke generates a static editable PPTX document and returns it as a workflow file.
func (t *GeneratePPTXTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	if err := enforceRuntimeFilePolicy(t.runtime, "pptx"); err != nil {
		return nil, err
	}
	_, normalizedSpec, err := parsePPTXDocumentSpec(rawStringParam(toolParameters, "presentation"))
	if err != nil {
		return nil, err
	}
	data, err := renderPPTXInSandbox(ctx, t.runtime, t.GetTenantID(), normalizedSpec)
	if err != nil {
		return nil, err
	}
	if len(data) > maxGeneratedFileBytes {
		return nil, fmt.Errorf("generated file exceeds %d bytes", maxGeneratedFileBytes)
	}
	lifecycle, err := resolveToolFileLifecycle(rawStringParam(toolParameters, "lifecycle"))
	if err != nil {
		return nil, err
	}
	filename := buildFilename(rawStringParam(toolParameters, "filename"), ".pptx")
	return createGeneratedFileForRuntime(ctx, t.GetTenantID(), t.runtime, generatedFileParams{
		userID:         userID,
		conversationID: conversationID,
		data:           data,
		mimeType:       pptxMimeType,
		extension:      ".pptx",
		filename:       filename,
		lifecycle:      lifecycle,
		format:         "pptx",
	})
}

func renderPPTXInSandbox(ctx context.Context, runtime *tools.ToolRuntime, tenantID string, normalizedSpec string) ([]byte, error) {
	return renderFileGeneratorPPTX(ctx, runtime, tenantID, normalizedSpec)
}

var _ tools.Tool = (*GeneratePPTXTool)(nil)
