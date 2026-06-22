package filegenerator

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	htmlPDFRenderTimeout = 30 * time.Second
	maxHTMLPDFInputBytes = 1 * 1024 * 1024
)

var htmlPDFCSSImportPattern = regexp.MustCompile(`(?i)@import`)

// GeneratePDFTool creates PDF documents from HTML in the workflow tool file store.
type GeneratePDFTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGeneratePDFTool creates a generate_pdf tool.
func NewGeneratePDFTool(tenantID string) *GeneratePDFTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_pdf",
			Author:   "System",
			Provider: "file_generator",
			Label: tools.I18nText{
				"en_US":   "Generate PDF from HTML",
				"zh_Hans": "从 HTML 生成 PDF",
			},
			Icon: "file-plus",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate a styled PDF file from HTML and inline CSS.",
				"zh_Hans": "根据 HTML 和内联 CSS 生成带排版的 PDF 文件。",
			},
			LLM: "Generate a styled PDF temporary artifact from HTML and optional inline CSS. This tool does not write to File Management. When the user asks to save the result into File Management, generate the artifact first and then use file-manager/save_file_to_management. Use this instead of generate_file when the PDF needs layout, tables, colors, page styles, or richer visual formatting. Use self-contained HTML/CSS only; do not reference external URLs or scripts.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "html",
				Label:            tools.I18nText{"en_US": "HTML", "zh_Hans": "HTML"},
				HumanDescription: tools.I18nText{"en_US": "Self-contained HTML body or full HTML document.", "zh_Hans": "自包含的 HTML 正文或完整 HTML 文档。"},
				LLMDescription:   "Self-contained HTML body or full HTML document. Use semantic HTML and inline styles/classes. Do not include external URLs, scripts, iframes, or remote assets.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": "<main>...</main>", "zh_Hans": "<main>...</main>"},
				SupportVariable:  true,
			},
			{
				Name:             "css",
				Label:            tools.I18nText{"en_US": "CSS", "zh_Hans": "CSS"},
				HumanDescription: tools.I18nText{"en_US": "Optional inline CSS appended to the HTML document.", "zh_Hans": "可选内联 CSS，会追加到 HTML 文档中。"},
				LLMDescription:   "Optional inline CSS. Prefer @page for page size and margins. Do not include external imports or remote fonts.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "@page { size: A4; margin: 20mm; }", "zh_Hans": "@page { size: A4; margin: 20mm; }"},
				SupportVariable:  true,
			},
			{
				Name:             "filename",
				Label:            tools.I18nText{"en_US": "Filename", "zh_Hans": "文件名"},
				HumanDescription: tools.I18nText{"en_US": "Optional display filename. The .pdf extension is added or corrected automatically.", "zh_Hans": "可选展示文件名，扩展名会自动补齐或修正为 .pdf。"},
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
				HumanDescription: tools.I18nText{"en_US": "Optional title used when wrapping an HTML fragment.", "zh_Hans": "包装 HTML 片段时使用的可选标题。"},
				LLMDescription:   "Optional title used when wrapping an HTML fragment. Visible title text must still be included in html.",
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
		Tags:       []string{"utilities", "file", "pdf", "html"},
	}
	return &GeneratePDFTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *GeneratePDFTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGeneratePDFTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke renders a self-contained HTML document to PDF and returns it as a workflow file.
func (t *GeneratePDFTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	if err := enforceRuntimeFilePolicy(t.runtime, "pdf"); err != nil {
		return nil, err
	}
	htmlContent := rawStringParam(toolParameters, "html")
	css := rawStringParam(toolParameters, "css")
	if strings.TrimSpace(htmlContent) == "" {
		return nil, fmt.Errorf("html is required")
	}
	if len(htmlContent)+len(css) > maxHTMLPDFInputBytes {
		return nil, fmt.Errorf("html and css exceed %d bytes", maxHTMLPDFInputBytes)
	}
	data, err := renderHTMLToPDFInSandbox(ctx, t.runtime, t.GetTenantID(), htmlContent, css, rawStringParam(toolParameters, "title"))
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
	filename := buildFilename(rawStringParam(toolParameters, "filename"), ".pdf")
	return createGeneratedFileForRuntime(ctx, t.GetTenantID(), t.runtime, generatedFileParams{
		userID:         userID,
		conversationID: conversationID,
		data:           data,
		mimeType:       pdfMimeType,
		extension:      ".pdf",
		filename:       filename,
		lifecycle:      lifecycle,
		format:         "pdf",
	})
}

func renderHTMLToPDFInSandbox(ctx context.Context, runtime *tools.ToolRuntime, tenantID string, htmlContent string, css string, title string) ([]byte, error) {
	document := buildHTMLPDFDocument(htmlContent, css, title)
	return renderFileGeneratorHTMLPDF(ctx, runtime, tenantID, document)
}

func buildHTMLPDFDocument(htmlContent string, css string, title string) string {
	htmlContent = normalizeLineEndings(htmlContent)
	css = sanitizeHTMLPDFCSS(css)
	csp := `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; font-src data:; base-uri 'none'; form-action 'none'; frame-src 'none'; script-src 'none'">`
	baseStyle := `<style>
@page { size: A4; margin: 20mm; }
* { box-sizing: border-box; }
body { margin: 0; font-family: "Noto Sans CJK SC", "Microsoft YaHei", "PingFang SC", "DejaVu Sans", Arial, sans-serif; color: #111827; line-height: 1.55; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #d1d5db; padding: 6px 8px; vertical-align: top; }
thead { display: table-header-group; }
tr { break-inside: avoid; page-break-inside: avoid; }
.page-break { break-before: page; page-break-before: always; }
</style>`
	if css != "" {
		baseStyle += "\n<style>" + css + "</style>"
	}
	if isFullHTMLDocument(htmlContent) {
		return injectHTMLPDFHead(htmlContent, csp+"\n"+baseStyle)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Generated PDF"
	}
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n" + csp + "\n<title>" + html.EscapeString(title) + "</title>\n" + baseStyle + "\n</head>\n<body>\n" + htmlContent + "\n</body>\n</html>\n"
}

func sanitizeHTMLPDFCSS(css string) string {
	css = normalizeLineEndings(css)
	return htmlPDFCSSImportPattern.ReplaceAllString(css, "/* blocked import */")
}

func injectHTMLPDFHead(document string, headExtras string) string {
	lower := strings.ToLower(document)
	if idx := strings.Index(lower, "</head>"); idx >= 0 {
		return document[:idx] + headExtras + "\n" + document[idx:]
	}
	if idx := strings.Index(lower, "<html"); idx >= 0 {
		if closeIdx := strings.Index(document[idx:], ">"); closeIdx >= 0 {
			insert := idx + closeIdx + 1
			return document[:insert] + "\n<head>\n<meta charset=\"utf-8\">\n" + headExtras + "\n</head>\n" + document[insert:]
		}
	}
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n" + headExtras + "\n</head>\n<body>\n" + document + "\n</body>\n</html>\n"
}

var _ tools.Tool = (*GeneratePDFTool)(nil)
