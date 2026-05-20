package filegenerator

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

const (
	defaultGeneratedFilename = "generated-file"
	maxGeneratedFileBytes    = 2 * 1024 * 1024
)

var filenameUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

// GenerateFileTool creates text-based files in the workflow tool file store.
type GenerateFileTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGenerateFileTool creates a generate_file tool.
func NewGenerateFileTool(tenantID string) *GenerateFileTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_file",
			Author:   "System",
			Provider: "file_generator",
			Label: tools.I18nText{
				"en_US":   "Generate File",
				"zh_Hans": "生成文件",
			},
			Icon: "file-plus",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate a downloadable text-based file from provided content.",
				"zh_Hans": "根据提供的文本内容生成可下载的文本类文件。",
			},
			LLM: "Generate a text-based file from provided content. Supported formats: txt, md, html, json, and csv.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "content",
				Label:            tools.I18nText{"en_US": "Content", "zh_Hans": "内容"},
				HumanDescription: tools.I18nText{"en_US": "Text content to write into the generated file.", "zh_Hans": "要写入生成文件的文本内容。"},
				LLMDescription:   "Text content to write into the generated file.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": "File content", "zh_Hans": "请输入文件内容"},
				SupportVariable:  true,
			},
			{
				Name:             "format",
				Label:            tools.I18nText{"en_US": "Format", "zh_Hans": "格式"},
				HumanDescription: tools.I18nText{"en_US": "Output file format.", "zh_Hans": "生成文件的输出格式。"},
				LLMDescription:   "Output format: txt, md, html, json, or csv.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Default:          "txt",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "txt", Label: tools.I18nText{"en_US": "Text", "zh_Hans": "纯文本"}},
					{Value: "md", Label: tools.I18nText{"en_US": "Markdown", "zh_Hans": "Markdown 文档"}},
					{Value: "html", Label: tools.I18nText{"en_US": "HTML", "zh_Hans": "HTML 网页"}},
					{Value: "json", Label: tools.I18nText{"en_US": "JSON", "zh_Hans": "JSON 文件"}},
					{Value: "csv", Label: tools.I18nText{"en_US": "CSV", "zh_Hans": "CSV 表格"}},
				},
			},
			{
				Name:             "filename",
				Label:            tools.I18nText{"en_US": "Filename", "zh_Hans": "文件名"},
				HumanDescription: tools.I18nText{"en_US": "Optional display filename. The extension is added or corrected automatically.", "zh_Hans": "可选的展示文件名。扩展名会自动补齐或修正。"},
				LLMDescription:   "Optional display filename. Do not include path separators.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "report", "zh_Hans": "例如：报告"},
				SupportVariable:  true,
			},
			{
				Name:             "title",
				Label:            tools.I18nText{"en_US": "Title", "zh_Hans": "标题"},
				HumanDescription: tools.I18nText{"en_US": "Optional document title used by generated HTML files.", "zh_Hans": "可选文档标题，生成 HTML 文件时使用。"},
				LLMDescription:   "Optional title for generated HTML files.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "Report", "zh_Hans": "例如：报告"},
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
		Tags:       []string{"utilities", "file"},
	}
	return &GenerateFileTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *GenerateFileTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGenerateFileTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke generates the requested file and returns it as a workflow file.
func (t *GenerateFileTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	content := rawStringParam(toolParameters, "content")
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	format, spec, err := resolveFormat(rawStringParam(toolParameters, "format"))
	if err != nil {
		return nil, err
	}

	data, err := renderContent(content, format, rawStringParam(toolParameters, "title"))
	if err != nil {
		return nil, err
	}
	if len(data) > maxGeneratedFileBytes {
		return nil, fmt.Errorf("generated file exceeds %d bytes", maxGeneratedFileBytes)
	}

	tenantID := t.GetTenantID()
	if tenantID == "" && t.runtime != nil {
		tenantID = t.runtime.TenantID
	}
	if tenantID == "" {
		return nil, fmt.Errorf("tenant id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}

	lifecycle, err := resolveToolFileLifecycle(rawStringParam(toolParameters, "lifecycle"))
	if err != nil {
		return nil, err
	}

	filename := buildFilename(rawStringParam(toolParameters, "filename"), spec.extension)
	toolFile, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:         userID,
		TenantID:       tenantID,
		ConversationID: conversationID,
		FileData:       data,
		MimeType:       spec.mimeType,
		Filename:       &filename,
		Lifecycle:      lifecycle,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create generated file: %w", err)
	}

	url, err := tool_file.SignToolFileGlobal(toolFile.ID, spec.extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign generated file: %w", err)
	}
	downloadURL := appendDownloadQuery(url)

	fileObj := workflowfile.NewFile(
		tenantID,
		workflowfile.FileTypeDocument,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(spec.extension),
		workflowfile.WithMimeType(spec.mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL

	return []tools.ToolInvokeMessage{
		{
			Type: tools.ToolInvokeMessageTypeFile,
			Text: downloadURL,
			Meta: map[string]interface{}{
				"file": fileMeta,
			},
		},
		builtin.CreateJSONMessage(map[string]interface{}{
			"file_id":      toolFile.ID,
			"filename":     toolFile.Name,
			"format":       format,
			"mime_type":    spec.mimeType,
			"size":         toolFile.Size,
			"url":          url,
			"download_url": downloadURL,
		}),
	}, nil
}

type formatSpec struct {
	extension string
	mimeType  string
}

func resolveFormat(raw string) (string, formatSpec, error) {
	format := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "."))
	if format == "" {
		format = "txt"
	}
	switch format {
	case "txt", "text":
		return "txt", formatSpec{extension: ".txt", mimeType: "text/plain"}, nil
	case "md", "markdown":
		return "md", formatSpec{extension: ".md", mimeType: "text/markdown"}, nil
	case "html", "htm":
		return "html", formatSpec{extension: ".html", mimeType: "text/html"}, nil
	case "json":
		return "json", formatSpec{extension: ".json", mimeType: "application/json"}, nil
	case "csv":
		return "csv", formatSpec{extension: ".csv", mimeType: "text/csv"}, nil
	default:
		return "", formatSpec{}, fmt.Errorf("unsupported format: %s", format)
	}
}

func renderContent(content string, format string, title string) ([]byte, error) {
	switch format {
	case "html":
		return []byte(renderSafeHTML(content, title)), nil
	case "json":
		var parsed interface{}
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			return nil, fmt.Errorf("content must be valid JSON: %w", err)
		}
		data, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to format JSON content: %w", err)
		}
		return append(data, '\n'), nil
	case "csv":
		reader := csv.NewReader(strings.NewReader(content))
		reader.FieldsPerRecord = -1
		if _, err := reader.ReadAll(); err != nil {
			return nil, fmt.Errorf("content must be valid CSV: %w", err)
		}
		return []byte(content), nil
	default:
		return []byte(content), nil
	}
}

func renderSafeHTML(content string, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Generated File"
	}
	escapedTitle := html.EscapeString(title)
	escapedContent := html.EscapeString(content)
	escapedContent = strings.ReplaceAll(escapedContent, "\r\n", "\n")
	escapedContent = strings.ReplaceAll(escapedContent, "\n", "<br>\n")
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>" + escapedTitle + "</title>\n</head>\n<body>\n" + escapedContent + "\n</body>\n</html>\n"
}

func resolveToolFileLifecycle(raw string) (tool_file.ToolFileLifecycle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "persistent":
		return tool_file.ToolFileLifecyclePersistent, nil
	case "temporary":
		return tool_file.ToolFileLifecycleTemporary, nil
	default:
		return "", fmt.Errorf("unsupported lifecycle: %s", raw)
	}
}

func buildFilename(raw string, extension string) string {
	name := sanitizeFilename(raw)
	if name == "" {
		name = defaultGeneratedFilename
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = filenameUnsafePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func rawStringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func appendDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

var _ tools.Tool = (*GenerateFileTool)(nil)
