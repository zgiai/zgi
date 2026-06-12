package filegenerator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/config"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveFormatSupportsOfficeAndPDF(t *testing.T) {
	tests := []struct {
		raw      string
		wantFmt  string
		wantExt  string
		wantMIME string
	}{
		{
			raw:      "docx",
			wantFmt:  "docx",
			wantExt:  ".docx",
			wantMIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			raw:      "word",
			wantFmt:  "docx",
			wantExt:  ".docx",
			wantMIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			raw:      "xlsx",
			wantFmt:  "xlsx",
			wantExt:  ".xlsx",
			wantMIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			raw:      "excel",
			wantFmt:  "xlsx",
			wantExt:  ".xlsx",
			wantMIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			raw:      "pdf",
			wantFmt:  "pdf",
			wantExt:  ".pdf",
			wantMIME: "application/pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			gotFmt, gotSpec, err := resolveFormat(tt.raw)
			require.NoError(t, err)
			require.Equal(t, tt.wantFmt, gotFmt)
			require.Equal(t, tt.wantExt, gotSpec.extension)
			require.Equal(t, tt.wantMIME, gotSpec.mimeType)
		})
	}
}

func TestRenderContentGeneratesValidOfficeAndPDF(t *testing.T) {
	t.Run("docx", func(t *testing.T) {
		data, err := renderContent("Hello\nCafé", "docx", "Report")
		require.NoError(t, err)
		requireZipEntries(t, data,
			"[Content_Types].xml",
			"_rels/.rels",
			"docProps/app.xml",
			"docProps/core.xml",
			"word/_rels/document.xml.rels",
			"word/document.xml",
			"word/fontTable.xml",
			"word/styles.xml",
			"word/theme/theme1.xml",
		)
		requireZipEntryContains(t, data, "word/document.xml", "Hello")
		requireZipEntryContains(t, data, "word/document.xml", "Café")
	})

	t.Run("xlsx", func(t *testing.T) {
		data, err := renderContent("Name,Score\nCafé,10\n", "xlsx", "Report")
		require.NoError(t, err)

		workbook, err := excelize.OpenReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer workbook.Close()

		rows, err := workbook.GetRows("Sheet1")
		require.NoError(t, err)
		require.Equal(t, [][]string{{"Name", "Score"}, {"Café", "10"}}, rows)
	})

	t.Run("pdf", func(t *testing.T) {
		data, err := renderContent("Café PDF", "pdf", "Report")
		require.NoError(t, err)
		require.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
		require.Contains(t, string(data), "00430061006600E9")
		require.NoError(t, api.Validate(bytes.NewReader(data), nil))
	})
}

func TestRenderContentGeneratesRunnableHTML(t *testing.T) {
	t.Run("preserves full document", func(t *testing.T) {
		content := "<!doctype html>\n<html><head><script>window.ready = true;</script></head><body><button>Run</button></body></html>"

		data, err := renderContent(content, "html", "Ignored")
		require.NoError(t, err)
		require.Equal(t, content, string(data))
		require.Contains(t, string(data), "<script>window.ready = true;</script>")
		require.NotContains(t, string(data), "&lt;script&gt;")
	})

	t.Run("wraps html fragment without escaping", func(t *testing.T) {
		data, err := renderContent("<main><h1>Hello</h1></main>", "html", "Report <Draft>")
		require.NoError(t, err)

		html := string(data)
		require.Contains(t, html, "<title>Report &lt;Draft&gt;</title>")
		require.Contains(t, html, "<main><h1>Hello</h1></main>")
		require.NotContains(t, html, "&lt;main&gt;")
	})
}

func TestRenderRichDocxGeneratesStyledDocument(t *testing.T) {
	raw := `{
  "page": {"size":"a4","orientation":"landscape","margins":{"top":36,"right":36,"bottom":36,"left":36}},
  "default_style": {"font_family":"SimSun","font_size":12,"color":"333333"},
  "blocks": [
    {"type":"heading","level":1,"text":"Styled Report","style":{"font_family":"Microsoft YaHei","font_size":18,"bold":true,"alignment":"center"}},
    {"type":"paragraph","style":{"alignment":"center","line_spacing":1.5,"space_after":12},"runs":[
      {"text":"Total: "},
      {"text":"113.47","bold":true,"underline":true,"color":"#C00000"}
    ]},
    {"type":"table","headers":["Item","Amount"],"rows":[["Electricity",{"text":"113.47","background_color":"FFF2CC","vertical_align":"center"}]],"table_style":{"alignment":"center","border_color":"000000"}},
    {"type":"page_break"},
    {"type":"paragraph","text":"Second page"}
  ]
}`
	spec, err := parseDocxDocumentSpec(raw)
	require.NoError(t, err)

	data, err := renderRichDocx(spec)
	require.NoError(t, err)
	requireZipEntries(t, data,
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/styles.xml",
	)
	requireZipEntryContains(t, data, "word/document.xml", "Styled Report")
	requireZipEntryContains(t, data, "word/document.xml", `w:jc w:val="center"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:rFonts w:ascii="Microsoft YaHei"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:sz w:val="36"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:color w:val="C00000"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:u w:val="single"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:after="240"`)
	requireZipEntryContains(t, data, "word/document.xml", "<w:tbl>")
	requireZipEntryContains(t, data, "word/document.xml", `w:fill="FFF2CC"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:type="page"`)
	requireZipEntryContains(t, data, "word/document.xml", `w:pgSz w:w="16838" w:h="11906"`)
}

func TestParseDocxDocumentSpecCompactsEmptyRuns(t *testing.T) {
	raw := `{
  "blocks": [
    {"type":"paragraph","runs":[
      {"text":""},
      {"text":"Visible text","bold":true},
      {"text":"   "}
    ]},
    {"type":"table","headers":[{"runs":[{"text":""},{"text":"Header"}]}],"rows":[[{"runs":[{"text":"Cell"},{"text":""}]}]]}
  ]
}`
	spec, err := parseDocxDocumentSpec(raw)
	require.NoError(t, err)
	require.Len(t, spec.Blocks[0].Runs, 1)
	require.Equal(t, "Visible text", spec.Blocks[0].Runs[0].Text)
	require.Len(t, spec.Blocks[1].Headers[0].Runs, 1)
	require.Equal(t, "Header", spec.Blocks[1].Headers[0].Runs[0].Text)
	require.Len(t, spec.Blocks[1].Rows[0][0].Runs, 1)
	require.Equal(t, "Cell", spec.Blocks[1].Rows[0][0].Runs[0].Text)

	data, err := renderRichDocx(spec)
	require.NoError(t, err)
	requireZipEntryContains(t, data, "word/document.xml", "Visible text")
	requireZipEntryContains(t, data, "word/document.xml", "Header")
	requireZipEntryContains(t, data, "word/document.xml", "Cell")
}

func TestParseDocxDocumentSpecRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "invalid json", raw: `not-json`, want: "document must be valid DOCX JSON"},
		{name: "empty blocks", raw: `{"blocks":[]}`, want: "document.blocks must include at least one block"},
		{name: "unknown block", raw: `{"blocks":[{"type":"chart","text":"x"}]}`, want: "unsupported DOCX block type"},
		{name: "bad color", raw: `{"blocks":[{"type":"paragraph","text":"x","style":{"color":"red"}}]}`, want: "expected RRGGBB hex color"},
		{name: "bad alignment", raw: `{"blocks":[{"type":"paragraph","text":"x","style":{"alignment":"middle"}}]}`, want: "unsupported alignment"},
		{name: "only empty runs", raw: `{"blocks":[{"type":"paragraph","runs":[{"text":""},{"text":"   "}]}]}`, want: "paragraph requires text or non-empty runs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDocxDocumentSpec(tt.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParsePPTXDocumentSpecNormalizesAndRejectsInvalidInput(t *testing.T) {
	raw := `{
  "layout": "wide",
  "language": "EN-gb",
  "default_style": {"font_family": "Microsoft YaHei", "font_size": 18, "color": "#111827"},
  "slides": [
    {"background_color": "#ffffff", "elements": [
      {"type": "title", "text": "Deck", "line_spacing": 1.15, "style": {"align": "center", "font_weight": "700", "line_spacing": 1.2, "underline": true}, "ignored_by_future_schema": true},
      {"type": "table", "headers": ["Metric", "Value"], "rows": [["Total", "113.47"]], "border_color": "#d1d5db", "header_fill_color": "#f3f4f6", "header_color": "#111827", "row_fill_color": "#ffffff", "column_widths": [3.2, 2.4]}
    ]}
  ]
}`
	spec, normalized, err := parsePPTXDocumentSpec(raw)
	require.NoError(t, err)
	require.Equal(t, "wide", spec.Layout)
	require.Equal(t, "en-GB", spec.Language)
	require.Equal(t, "Microsoft YaHei", spec.DefaultStyle.FontFace)
	require.Equal(t, "FFFFFF", spec.Slides[0].BackgroundColor)
	require.NotNil(t, spec.Slides[0].Elements[0].Style.Bold)
	require.True(t, *spec.Slides[0].Elements[0].Style.Bold)
	require.Contains(t, normalized, `"background_color":"FFFFFF"`)
	require.Contains(t, normalized, `"language":"en-GB"`)
	require.Contains(t, normalized, `"font_face":"Microsoft YaHei"`)
	require.Contains(t, normalized, `"line_spacing":1.2`)
	require.Contains(t, normalized, `"underline":true`)
	require.Contains(t, normalized, `"border_color":"D1D5DB"`)
	require.Contains(t, normalized, `"header_fill_color":"F3F4F6"`)
	require.Contains(t, normalized, `"column_widths":[3.2,2.4]`)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "invalid json", raw: `not-json`, want: "presentation must be valid PPTX JSON"},
		{name: "empty slides", raw: `{"slides":[]}`, want: "presentation.slides must include at least one slide"},
		{name: "unknown element", raw: `{"slides":[{"elements":[{"type":"video"}]}]}`, want: "unsupported PPTX element type"},
		{name: "bad color", raw: `{"slides":[{"elements":[{"type":"text","text":"x","style":{"color":"red"}}]}]}`, want: "expected RRGGBB hex color"},
		{name: "bad table", raw: `{"slides":[{"elements":[{"type":"table"}]}]}`, want: "table requires headers or rows"},
		{name: "bad line spacing", raw: `{"slides":[{"elements":[{"type":"text","text":"x","style":{"line_spacing":4}}]}]}`, want: "line_spacing must be between 0.5 and 3"},
		{name: "bad language", raw: `{"language":"not a language","slides":[{"elements":[{"type":"text","text":"x"}]}]}`, want: "presentation.language must be a valid BCP 47 language tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parsePPTXDocumentSpec(tt.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParsePPTXDocumentSpecAppliesSafeAutoLayout(t *testing.T) {
	raw := `{
  "slides": [
    {"elements": [
      {"type": "title", "text": "活动背景", "style": {"font_size": 30}},
      {"type": "text", "text": "围绕线下活动的参与动机、传播节奏和现场转化进行说明。", "style": {"font_size": 20}},
      {"type": "text", "text": "通过分层触达提升报名质量，并在活动后沉淀可复用内容。", "style": {"font_size": 20}}
    ]}
  ]
}`
	spec, normalized, err := parsePPTXDocumentSpec(raw)
	require.NoError(t, err)
	require.Contains(t, normalized, `"x":0.6`)

	elements := spec.Slides[0].Elements
	require.Len(t, elements, 3)
	for idx := range elements {
		require.NotNil(t, elements[idx].X, "element %d x", idx)
		require.NotNil(t, elements[idx].Y, "element %d y", idx)
		require.NotNil(t, elements[idx].W, "element %d w", idx)
		require.NotNil(t, elements[idx].H, "element %d h", idx)
	}
	require.Greater(t, *elements[1].Y, *elements[0].Y+*elements[0].H)
	require.Greater(t, *elements[2].Y, *elements[1].Y+*elements[1].H)
}

func TestParsePPTXDocumentSpecAllowsVisibleBleedShapes(t *testing.T) {
	_, normalized, err := parsePPTXDocumentSpec(`{
  "slides": [
    {"elements": [
      {"type": "shape", "fill_color": "F59E0B", "x": -0.4, "y": -0.2, "w": 2.2, "h": 0.35}
    ]}
  ]
}`)
	require.NoError(t, err)
	require.Contains(t, normalized, `"x":-0.4`)
	require.Contains(t, normalized, `"y":-0.2`)
}

func TestParsePPTXDocumentSpecWrapsLongChineseText(t *testing.T) {
	raw := `{
  "slides": [
    {"elements": [
      {"type": "text", "text": "` + strings.Repeat("雨天社交新主张", 20) + `", "x": 0.8, "y": 1, "w": 7.5, "h": 4, "style": {"font_size": 16}}
    ]}
  ]
}`
	spec, normalized, err := parsePPTXDocumentSpec(raw)
	require.NoError(t, err)

	element := spec.Slides[0].Elements[0]
	require.Contains(t, element.Text, "\n")
	require.Contains(t, normalized, `\n`)
	measure := measurePPTXText(element.Text, &element, spec.DefaultStyle, *element.W, false)
	require.LessOrEqual(t, measure.LongestLineUnits, measure.LineCapacity+pptxLayoutEpsilon)
	require.LessOrEqual(t, measure.EstimatedHeight, *element.H+pptxFitHeightSlack(*element.H))
}

func TestParsePPTXDocumentSpecRejectsBadLayout(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "off canvas",
			raw:  `{"slides":[{"elements":[{"type":"shape","fill_color":"F59E0B","x":-1.5,"y":0,"w":0.5,"h":0.5}]}]}`,
			want: "must be at least partially visible",
		},
		{
			name: "overlap",
			raw:  `{"slides":[{"elements":[{"type":"text","text":"first","x":1,"y":1,"w":4,"h":1},{"type":"text","text":"second","x":1.5,"y":1.2,"w":4,"h":1}]}]}`,
			want: "overlaps",
		},
		{
			name: "content outside slide",
			raw:  `{"slides":[{"elements":[{"type":"text","text":"too low","x":1,"y":7.1,"w":4,"h":0.8}]}]}`,
			want: "content box must fit within the slide bounds",
		},
		{
			name: "explicit text overflow",
			raw:  `{"slides":[{"elements":[{"type":"text","text":"` + strings.Repeat("雨天社交新主张", 40) + `","x":0.8,"y":1,"w":3.5,"h":0.6,"style":{"font_size":18}}]}]}`,
			want: "text does not fit",
		},
		{
			name: "auto text overflow",
			raw:  `{"slides":[{"elements":[{"type":"text","text":"` + strings.Repeat("超长中文内容用于验证自动布局无法塞进单页时会提示拆分。", 80) + `","style":{"font_size":24}}]}]}`,
			want: "content box must fit within the slide bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parsePPTXDocumentSpec(tt.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestBuildHTMLPDFDocumentWrapsHTMLAndCSS(t *testing.T) {
	document := buildHTMLPDFDocument("<main><h1>Hello</h1></main>", "@page { size: A4; }", "Report <Draft>")
	require.Contains(t, document, `Content-Security-Policy`)
	require.Contains(t, document, "<title>Report &lt;Draft&gt;</title>")
	require.Contains(t, document, "<main><h1>Hello</h1></main>")
	require.Contains(t, document, "@page { size: A4; }")
	require.Contains(t, document, "script-src 'none'")
}

func TestBuildHTMLPDFDocumentBlocksCSSImportsCaseInsensitive(t *testing.T) {
	document := buildHTMLPDFDocument("<main>Hello</main>", `@Import url("https://example.com/style.css");`, "")
	require.Contains(t, document, "/* blocked import */")
	require.NotContains(t, document, "@Import")
}

func TestBuildHTMLPDFDocumentInjectsFullHTMLHead(t *testing.T) {
	document := buildHTMLPDFDocument("<!doctype html><html><head><title>Existing</title></head><body><main>Hello</main></body></html>", "h1 { color: red; }", "")
	require.Contains(t, document, "<title>Existing</title>")
	require.Contains(t, document, `Content-Security-Policy`)
	require.Contains(t, document, "h1 { color: red; }")
	require.Contains(t, document, "<main>Hello</main>")
}

func TestRenderHTMLToPDFUsesSandbox(t *testing.T) {
	server := newFakeFileGeneratorSandbox(t)
	data, err := renderHTMLToPDFInSandbox(
		context.Background(),
		&tools.ToolRuntime{TenantID: "tenant-1"},
		"",
		`<main><h1>Styled PDF</h1><table><thead><tr><th>Item</th><th>Amount</th></tr></thead><tbody><tr><td>Electricity</td><td>113.47</td></tr></tbody></table></main>`,
		`@page { size: A4; margin: 18mm; } h1 { color: #c00000; text-align: center; }`,
		"Styled PDF",
	)
	require.NoError(t, err)
	require.NoError(t, api.Validate(bytes.NewReader(data), nil))
	require.Equal(t, 1, server.profileRequests)
	require.Equal(t, 1, server.createRequests)
	require.Equal(t, 1, server.uploadRequests)
	require.Equal(t, 1, server.commandRequests)
	require.Equal(t, 1, server.downloadRequests)
	require.Equal(t, 1, server.deleteRequests)
	require.Contains(t, server.uploadedHTML, "Styled PDF")
}

func TestRenderPPTXUsesSandbox(t *testing.T) {
	server := newFakeFileGeneratorSandbox(t)
	_, normalized, err := parsePPTXDocumentSpec(`{"slides":[{"elements":[{"type":"title","text":"Styled Deck"}]}]}`)
	require.NoError(t, err)

	data, err := renderPPTXInSandbox(context.Background(), &tools.ToolRuntime{TenantID: "tenant-1"}, "", normalized)
	require.NoError(t, err)
	requireZipEntries(t, data, "[Content_Types].xml", "_rels/.rels", "ppt/presentation.xml")
	require.Equal(t, 1, server.profileRequests)
	require.Equal(t, 1, server.createRequests)
	require.Equal(t, 2, server.uploadRequests)
	require.Equal(t, 1, server.commandRequests)
	require.Equal(t, 1, server.downloadRequests)
	require.Equal(t, 1, server.deleteRequests)
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXSpecPath], "Styled Deck")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "pptxgenjs")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "createRequire")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "wrapTextForBox")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "opts.lineSpacingMultiple = merged.line_spacing")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "opts.lineSpacingMultiple = element.line_spacing")
	require.NotContains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "opts.lineSpacing = merged.line_spacing")
	require.NotContains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], "opts.lineSpacing = element.line_spacing")
	require.Contains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], `pptx.lang = spec.language || "en-US"`)
	require.NotContains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], `pptx.lang = "zh-CN"`)
	require.NotContains(t, server.uploadedFiles[fileGeneratorPPTXScriptPath], `import pptxgen from "pptxgenjs"`)
}

func TestRenderPPTXReportsSandboxErrorField(t *testing.T) {
	server := newFakeFileGeneratorSandbox(t)
	server.commandError = "Error [ERR_MODULE_NOT_FOUND]: Cannot find package 'pptxgenjs'"
	_, normalized, err := parsePPTXDocumentSpec(`{"slides":[{"elements":[{"type":"title","text":"Styled Deck"}]}]}`)
	require.NoError(t, err)

	_, err = renderPPTXInSandbox(context.Background(), &tools.ToolRuntime{TenantID: "tenant-1"}, "", normalized)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ERR_MODULE_NOT_FOUND")
	require.Equal(t, 1, server.commandRequests)
	require.Equal(t, 1, server.deleteRequests)
}

func TestGenerateFileToolReturnsDownloadableOfficeFileMetadata(t *testing.T) {
	db, mock, cleanup := openFileGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGenerateFileTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"content":  "Name,Score\nAlice,10\n",
			"format":   "xlsx",
			"filename": "report",
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)

	require.Equal(t, tools.ToolInvokeMessageTypeJSON, messages[1].Type)
	jsonPayload := messages[1].Data
	require.Equal(t, "report.xlsx", jsonPayload["filename"])
	require.Equal(t, "xlsx", jsonPayload["format"])
	require.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["url"])
	require.NotEmpty(t, jsonPayload["download_url"])

	parsed, err := url.Parse(jsonPayload["download_url"].(string))
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("download"))

	fileID := jsonPayload["file_id"].(string)
	require.NotEmpty(t, fileID)
	data := fileStorage.onlyFileData(t)
	require.NotEmpty(t, data)

	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer workbook.Close()

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateDocxToolReturnsDownloadableRichDocxMetadata(t *testing.T) {
	db, mock, cleanup := openFileGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGenerateDocxTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"document": `{"blocks":[{"type":"heading","text":"Report","style":{"font_size":18,"bold":true,"alignment":"center"}}]}`,
			"filename": "styled-report",
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)

	jsonPayload := messages[1].Data
	require.Equal(t, "styled-report.docx", jsonPayload["filename"])
	require.Equal(t, "docx", jsonPayload["format"])
	require.Equal(t, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["download_url"])

	data := fileStorage.onlyFileData(t)
	requireZipEntryContains(t, data, "word/document.xml", "Report")
	requireZipEntryContains(t, data, "word/document.xml", `w:sz w:val="36"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGeneratePDFToolReturnsDownloadableHTMLPDFMetadata(t *testing.T) {
	newFakeFileGeneratorSandbox(t)
	db, mock, cleanup := openFileGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGeneratePDFTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"html":     `<main><h1>Report</h1><p>Total: <strong>113.47</strong></p></main>`,
			"css":      `@page { size: A4; margin: 18mm; } h1 { text-align: center; }`,
			"filename": "styled-report",
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)

	jsonPayload := messages[1].Data
	require.Equal(t, "styled-report.pdf", jsonPayload["filename"])
	require.Equal(t, "pdf", jsonPayload["format"])
	require.Equal(t, "application/pdf", jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["download_url"])

	data := fileStorage.onlyFileData(t)
	require.NoError(t, api.Validate(bytes.NewReader(data), nil))
	require.Contains(t, string(data), "/Type /Page")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGeneratePPTXToolReturnsDownloadablePresentationMetadata(t *testing.T) {
	newFakeFileGeneratorSandbox(t)
	db, mock, cleanup := openFileGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGeneratePPTXTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"presentation": `{"slides":[{"elements":[{"type":"title","text":"Deck","style":{"align":"center","font_size":30}}]}]}`,
			"filename":     "styled-deck",
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)

	jsonPayload := messages[1].Data
	require.Equal(t, "styled-deck.pptx", jsonPayload["filename"])
	require.Equal(t, "pptx", jsonPayload["format"])
	require.Equal(t, "application/vnd.openxmlformats-officedocument.presentationml.presentation", jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["download_url"])

	data := fileStorage.onlyFileData(t)
	requireZipEntries(t, data, "[Content_Types].xml", "_rels/.rels", "ppt/presentation.xml")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateFileToolRejectsFormatOutsideRuntimePolicy(t *testing.T) {
	tool := NewGenerateFileTool("tenant-1").ForkToolRuntime(&tools.ToolRuntime{
		TenantID: "tenant-1",
		RuntimeParameters: map[string]interface{}{
			"file_generation_policies": []map[string]interface{}{
				{
					"source_skill_id": "custom-settlement",
					"output_formats":  []string{"pdf", "docx"},
				},
			},
		},
	})

	_, err := tool.Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"content":  "Name,Score\nAlice,10\n",
			"format":   "xlsx",
			"filename": "report",
		},
		nil,
		nil,
		nil,
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "format xlsx is not allowed by current Skill file policy")
	require.Contains(t, err.Error(), "docx")
	require.Contains(t, err.Error(), "pdf")
}

func TestRuntimeAllowedOutputFormatsNormalizesSkillPolicies(t *testing.T) {
	allowed := runtimeAllowedOutputFormats(&tools.ToolRuntime{
		RuntimeParameters: map[string]interface{}{
			"file_generation_policies": []interface{}{
				map[string]interface{}{
					"output_formats":         []interface{}{"PDF", "word", "excel"},
					"allowed_output_formats": []interface{}{"powerpoint"},
				},
			},
		},
	})

	for _, format := range []string{"pdf", "docx", "xlsx", "pptx"} {
		if _, ok := allowed[format]; !ok {
			t.Fatalf("allowed formats = %#v, missing %s", allowed, format)
		}
	}
}

type memoryStorage struct {
	files map[string][]byte
}

type fakeFileGeneratorSandbox struct {
	server           *httptest.Server
	profileRequests  int
	createRequests   int
	uploadRequests   int
	commandRequests  int
	downloadRequests int
	deleteRequests   int
	uploadedHTML     string
	uploadedFiles    map[string]string
	commandError     string
}

func newFakeFileGeneratorSandbox(t *testing.T) *fakeFileGeneratorSandbox {
	t.Helper()
	fake := &fakeFileGeneratorSandbox{uploadedFiles: make(map[string]string)}
	pdfData, err := renderPDF("sandbox pdf", "Sandbox PDF")
	require.NoError(t, err)
	pptxData := minimalPPTXZip(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sandbox/dependencies", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		fake.profileRequests++
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{
			"profiles": []map[string]interface{}{
				{
					"name":    defaultSystemOfficeProfile,
					"version": "2026.05.31",
					"status":  "ready",
					"enabled": true,
				},
			},
		})
	})
	mux.HandleFunc("/v1/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		fake.createRequests++
		var req map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "session", req["runtime_profile"])
		require.Equal(t, defaultSystemOfficeProfile, req["dependency_profile"])
		require.Equal(t, "tenant-1", req["organization_id"])
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_pdf"})
	})
	mux.HandleFunc("/v1/files/upload", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		fake.uploadRequests++
		var req map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "sbx_pdf", req["sandbox_id"])
		require.Equal(t, "base64", req["encoding"])
		raw, err := base64.StdEncoding.DecodeString(req["content"].(string))
		require.NoError(t, err)
		path := req["path"].(string)
		fake.uploadedFiles[path] = string(raw)
		if path == fileGeneratorHTMLInputPath {
			fake.uploadedHTML = string(raw)
		}
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{"path": path, "size": len(raw)})
	})
	mux.HandleFunc("/v1/exec/command", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		fake.commandRequests++
		var req map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "sbx_pdf", req["sandbox_id"])
		require.Equal(t, "skill-node", req["profile"])
		args, ok := req["args"].([]interface{})
		require.True(t, ok)
		switch req["command"] {
		case "sh":
			require.Len(t, args, 2)
			require.Contains(t, args[1].(string), "-env:UserInstallation=file://")
			require.Contains(t, args[1].(string), "libreoffice")
			require.Contains(t, args[1].(string), "--headless --convert-to pdf")
		case "node":
			require.Len(t, args, 1)
			require.Equal(t, fileGeneratorPPTXScriptPath, args[0].(string))
		default:
			t.Fatalf("unexpected command: %v", req["command"])
		}
		if fake.commandError != "" {
			writeFakeSandboxEnvelope(t, w, map[string]interface{}{"exit_code": 1, "stdout": "", "error": fake.commandError})
			return
		}
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{"exit_code": 0, "stdout": "ok", "stderr": ""})
	})
	mux.HandleFunc("/v1/files/download", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		fake.downloadRequests++
		require.Equal(t, "sbx_pdf", r.URL.Query().Get("sandbox_id"))
		require.Equal(t, "base64", r.URL.Query().Get("encoding"))
		path := r.URL.Query().Get("path")
		data := pdfData
		if path == fileGeneratorPPTXOutputPath {
			data = pptxData
		} else {
			require.Equal(t, fileGeneratorPDFOutputPath, path)
		}
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{
			"path":     path,
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString(data),
			"size":     len(data),
		})
	})
	mux.HandleFunc("/v1/sandboxes/sbx_pdf", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		fake.deleteRequests++
		writeFakeSandboxEnvelope(t, w, map[string]interface{}{"deleted": true})
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)

	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		CodeExec: config.CodeExecConfig{
			Endpoint:                     fake.server.URL,
			ConnectTimeoutSeconds:        1,
			CreateTimeoutSeconds:         1,
			UploadTimeoutSeconds:         1,
			CommandTimeoutPaddingSeconds: 1,
			ArtifactTimeoutSeconds:       1,
			CleanupTimeoutSeconds:        1,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
	return fake
}

func writeFakeSandboxEnvelope(t *testing.T, w http.ResponseWriter, data map[string]interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "ok",
		"data":    data,
	}))
}

func minimalPPTXZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml":  `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`,
		"_rels/.rels":          `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/presentation.xml": `<?xml version="1.0" encoding="UTF-8"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst/></p:presentation>`,
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{files: make(map[string][]byte)}
}

func (s *memoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- data
	close(ch)
	return ch, nil
}

func (s *memoryStorage) Download(filename string, targetPath string) error {
	return nil
}

func (s *memoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *memoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *memoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	return nil, nil
}

func (s *memoryStorage) onlyFileData(t *testing.T) []byte {
	t.Helper()
	require.Len(t, s.files, 1)
	for _, data := range s.files {
		return append([]byte(nil), data...)
	}
	return nil
}

func openFileGeneratorMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func requireZipEntryContains(t *testing.T, data []byte, entryName string, want string) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	for _, file := range reader.File {
		if file.Name != entryName {
			continue
		}
		handle, err := file.Open()
		require.NoError(t, err)
		defer handle.Close()

		var buf bytes.Buffer
		_, err = buf.ReadFrom(handle)
		require.NoError(t, err)
		require.Contains(t, buf.String(), want)
		return
	}

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	require.Failf(t, "missing zip entry", "entry %s not found in %s", entryName, strings.Join(names, ", "))
}

func requireZipEntries(t *testing.T, data []byte, entryNames ...string) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	entries := make(map[string]struct{}, len(reader.File))
	for _, file := range reader.File {
		entries[file.Name] = struct{}{}
	}

	for _, entryName := range entryNames {
		if _, ok := entries[entryName]; !ok {
			names := make([]string, 0, len(reader.File))
			for _, file := range reader.File {
				names = append(names, file.Name)
			}
			require.Failf(t, "missing zip entry", "entry %s not found in %s", entryName, strings.Join(names, ", "))
		}
	}
}
