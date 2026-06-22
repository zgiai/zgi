package filegenerator

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// GenerateDocxTool creates rich Word documents in the workflow tool file store.
type GenerateDocxTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGenerateDocxTool creates a generate_docx tool.
func NewGenerateDocxTool(tenantID string) *GenerateDocxTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_docx",
			Author:   "System",
			Provider: "file_generator",
			Label: tools.I18nText{
				"en_US":   "Generate Rich Word Document",
				"zh_Hans": "生成富文本 Word 文档",
			},
			Icon: "file-plus",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate a styled DOCX file from a structured document specification.",
				"zh_Hans": "根据结构化文档规格生成带样式的 DOCX 文件。",
			},
			LLM: "Generate a styled DOCX temporary artifact from a JSON document specification. This tool does not write to File Management. When the user asks to save the result into File Management, generate the artifact first and then use file-manager/save_file_to_management. Use this instead of generate_file when the Word document needs headings, fonts, font sizes, bold or colored text, paragraph alignment, spacing, page margins, page breaks, or simple tables. Every runs item must include non-empty text; omit empty runs.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "document",
				Label:            tools.I18nText{"en_US": "Document", "zh_Hans": "文档规格"},
				HumanDescription: tools.I18nText{"en_US": "DOCX document specification as JSON.", "zh_Hans": "JSON 格式的 DOCX 文档规格。"},
				LLMDescription:   "JSON string describing the DOCX document. Include blocks with type heading, paragraph, table, or page_break. Do not include runs entries with empty text.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": `{"blocks":[...]}`, "zh_Hans": `{"blocks":[...]}`},
				SupportVariable:  true,
			},
			{
				Name:             "filename",
				Label:            tools.I18nText{"en_US": "Filename", "zh_Hans": "文件名"},
				HumanDescription: tools.I18nText{"en_US": "Optional display filename. The .docx extension is added or corrected automatically.", "zh_Hans": "可选展示文件名，扩展名会自动补齐或修正为 .docx。"},
				LLMDescription:   "Optional display filename. Do not include path separators or an extension.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "report", "zh_Hans": "报告"},
				SupportVariable:  true,
			},
			{
				Name:             "title",
				Label:            tools.I18nText{"en_US": "Title", "zh_Hans": "标题"},
				HumanDescription: tools.I18nText{"en_US": "Optional title hint. Visible content must be included in the document specification.", "zh_Hans": "可选标题提示。可见内容必须写入文档规格。"},
				LLMDescription:   "Optional title hint. The visible document content must be included in document.blocks.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "Report", "zh_Hans": "报告"},
				SupportVariable:  true,
			},
			{
				Name:             "lifecycle",
				Label:            tools.I18nText{"en_US": "Lifecycle", "zh_Hans": "生命周期"},
				HumanDescription: tools.I18nText{"en_US": "Whether the generated file is persistent or temporary.", "zh_Hans": "生成文件是持久保存还是临时保存。"},
				LLMDescription:   "Temporary artifact lifecycle: persistent or temporary. Defaults to temporary.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "temporary",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "persistent", Label: tools.I18nText{"en_US": "Persistent", "zh_Hans": "持久保存"}},
					{Value: "temporary", Label: tools.I18nText{"en_US": "Temporary", "zh_Hans": "临时文件"}},
				},
			},
		},
		OutputType: "file",
		Tags:       []string{"utilities", "file", "docx"},
	}
	return &GenerateDocxTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *GenerateDocxTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGenerateDocxTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke generates a rich DOCX document and returns it as a workflow file.
func (t *GenerateDocxTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	if err := enforceRuntimeFilePolicy(t.runtime, "docx"); err != nil {
		return nil, err
	}
	spec, err := parseDocxDocumentSpec(rawStringParam(toolParameters, "document"))
	if err != nil {
		return nil, err
	}
	data, err := renderRichDocx(spec)
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
	filename := buildFilename(rawStringParam(toolParameters, "filename"), ".docx")
	return createGeneratedFileForRuntime(ctx, t.GetTenantID(), t.runtime, generatedFileParams{
		userID:         userID,
		conversationID: conversationID,
		data:           data,
		mimeType:       docxMimeType,
		extension:      ".docx",
		filename:       filename,
		lifecycle:      lifecycle,
		format:         "docx",
	})
}

var _ tools.Tool = (*GenerateDocxTool)(nil)
