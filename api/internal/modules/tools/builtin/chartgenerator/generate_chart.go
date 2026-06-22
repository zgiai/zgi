package chartgenerator

import (
	"context"
	"fmt"
	"strings"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	defaultChartFilename = "chart"
	maxGeneratedSVGBytes = 1024 * 1024
	svgMimeType          = "image/svg+xml"
)

// GenerateChartTool creates chart files in the workflow tool file store.
type GenerateChartTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGenerateChartTool creates a generate_chart tool.
func NewGenerateChartTool(tenantID string) *GenerateChartTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_chart",
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US":   "Generate Chart",
				"zh_Hans": "Generate Chart",
			},
			Icon: "chart-no-axes-combined",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate a downloadable SVG chart from structured data.",
				"zh_Hans": "Generate a downloadable SVG chart from structured data.",
			},
			LLM: "Generate a temporary downloadable SVG chart artifact from structured data. This tool does not write to File Management. Supports radar, bar, line, pie, doughnut, scatter, and score_distribution charts. For casual, vague, incomplete, or non-structured chart requests, use prompt-professionalizer before this tool. For generic requests such as 'generate a chart' or 'generate visualization', do not infer the chart type, title, or style; ask the user to confirm those decisions before calling. When the user asks to save the chart into File Management, generate the chart first and then use file-manager/save_file_to_management.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "chart_type",
				Label:            tools.I18nText{"en_US": "Chart Type", "zh_Hans": "Chart Type"},
				HumanDescription: tools.I18nText{"en_US": "Chart type to generate.", "zh_Hans": "Chart type to generate."},
				LLMDescription:   "Chart type: radar, bar, line, pie, doughnut, scatter, or score_distribution.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Default:          "radar",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "radar", Label: tools.I18nText{"en_US": "Radar", "zh_Hans": "Radar"}},
					{Value: "bar", Label: tools.I18nText{"en_US": "Bar", "zh_Hans": "Bar"}},
					{Value: "line", Label: tools.I18nText{"en_US": "Line", "zh_Hans": "Line"}},
					{Value: "pie", Label: tools.I18nText{"en_US": "Pie", "zh_Hans": "Pie"}},
					{Value: "doughnut", Label: tools.I18nText{"en_US": "Doughnut", "zh_Hans": "Doughnut"}},
					{Value: "scatter", Label: tools.I18nText{"en_US": "Scatter", "zh_Hans": "Scatter"}},
					{Value: "score_distribution", Label: tools.I18nText{"en_US": "Score Distribution", "zh_Hans": "Score Distribution"}},
				},
			},
			{
				Name:             "data",
				Label:            tools.I18nText{"en_US": "Data", "zh_Hans": "Data"},
				HumanDescription: tools.I18nText{"en_US": "Chart-specific structured data.", "zh_Hans": "Chart-specific structured data."},
				LLMDescription:   "Structured chart data object or JSON object string. Radar uses dimensions; bar uses categories; line uses x_axis; pie and doughnut use items; scatter uses points; score_distribution uses bands with counts or scores.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "title",
				Label:            tools.I18nText{"en_US": "Title", "zh_Hans": "Title"},
				HumanDescription: tools.I18nText{"en_US": "Optional chart title.", "zh_Hans": "Optional chart title."},
				LLMDescription:   "Optional chart title.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "output_filename",
				Label:            tools.I18nText{"en_US": "Output Filename", "zh_Hans": "Output Filename"},
				HumanDescription: tools.I18nText{"en_US": "Optional output filename without path separators.", "zh_Hans": "Optional output filename without path separators."},
				LLMDescription:   "Optional output filename. Do not include path separators or an extension.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "options",
				Label:            tools.I18nText{"en_US": "Options", "zh_Hans": "Options"},
				HumanDescription: tools.I18nText{"en_US": "Optional rendering options.", "zh_Hans": "Optional rendering options."},
				LLMDescription:   "Optional rendering options as an object or JSON object string: width, height, style, show_values, show_labels, legend, grid.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "lifecycle",
				Label:            tools.I18nText{"en_US": "Lifecycle", "zh_Hans": "Lifecycle"},
				HumanDescription: tools.I18nText{"en_US": "Whether the generated chart is persistent or temporary.", "zh_Hans": "Whether the generated chart is persistent or temporary."},
				LLMDescription:   "Temporary artifact lifecycle: persistent or temporary. Defaults to temporary.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "temporary",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "persistent", Label: tools.I18nText{"en_US": "Persistent", "zh_Hans": "Persistent"}},
					{Value: "temporary", Label: tools.I18nText{"en_US": "Temporary", "zh_Hans": "Temporary"}},
				},
			},
		},
		OutputType: "file",
		Tags:       []string{"visualization", "chart"},
	}
	return &GenerateChartTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}
func (t *GenerateChartTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGenerateChartTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke generates the requested chart and returns it as a workflow file.
func (t *GenerateChartTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	chartType := normalizeChartType(chartRawStringParam(toolParameters, "chart_type"))
	data, err := chartMapParam(toolParameters, "data")
	if err != nil {
		return nil, err
	}
	options, err := optionalChartMapParam(toolParameters, "options")
	if err != nil {
		return nil, err
	}

	svg, meta, err := renderChart(chartType, chartRawStringParam(toolParameters, "title"), data, options)
	if err != nil {
		return nil, err
	}
	if len(svg) > maxGeneratedSVGBytes {
		return nil, fmt.Errorf("generated chart exceeds %d bytes", maxGeneratedSVGBytes)
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

	lifecycle, err := resolveChartFileLifecycle(chartRawStringParam(toolParameters, "lifecycle"))
	if err != nil {
		return nil, err
	}

	filename := buildChartFilename(firstNonEmpty(
		chartRawStringParam(toolParameters, "output_filename"),
		chartRawStringParam(toolParameters, "filename"),
	), ".svg")
	toolFile, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:         userID,
		TenantID:       tenantID,
		ConversationID: conversationID,
		FileData:       []byte(svg),
		MimeType:       svgMimeType,
		Filename:       &filename,
		Lifecycle:      lifecycle,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create generated chart: %w", err)
	}

	url, err := tool_file.SignToolFileGlobal(toolFile.ID, ".svg")
	if err != nil {
		return nil, fmt.Errorf("failed to sign generated chart: %w", err)
	}
	downloadURL := appendChartDownloadQuery(url)

	fileObj := workflowfile.NewFile(
		tenantID,
		workflowfile.FileTypeImage,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(".svg"),
		workflowfile.WithMimeType(svgMimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	fileMeta["target"] = "temporary_artifact"

	return []tools.ToolInvokeMessage{
		{
			Type: tools.ToolInvokeMessageTypeFile,
			Text: downloadURL,
			Meta: map[string]interface{}{"file": fileMeta},
		},
		builtin.CreateJSONMessage(map[string]interface{}{
			"file_id":      toolFile.ID,
			"tool_file_id": toolFile.ID,
			"filename":     toolFile.Name,
			"chart_type":   meta.ChartType,
			"format":       "svg",
			"mime_type":    svgMimeType,
			"size":         toolFile.Size,
			"url":          url,
			"download_url": downloadURL,
			"target":       "temporary_artifact",
			"x_count":      meta.XCount,
			"series_count": meta.SeriesCount,
		}),
	}, nil
}

func normalizeChartType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "radar", "spider":
		return "radar"
	case "bar", "column":
		return "bar"
	case "line", "trend":
		return "line"
	case "pie", "pie_chart":
		return "pie"
	case "doughnut", "donut", "ring", "doughnut_chart", "donut_chart":
		return "doughnut"
	case "scatter", "scatter_plot":
		return "scatter"
	case "score_distribution", "score-distribution", "distribution", "score_band_distribution":
		return "score_distribution"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
func renderChart(chartType string, title string, data map[string]interface{}, options map[string]interface{}) (string, chartRenderMeta, error) {
	switch chartType {
	case "radar":
		return renderRadarChart(title, data, options)
	case "bar":
		return renderBarChart(title, data, options)
	case "line":
		return renderLineChart(title, data, options)
	case "pie":
		return renderPieChart(title, data, options)
	case "doughnut":
		return renderDoughnutChart(title, data, options)
	case "scatter":
		return renderScatterChart(title, data, options)
	case "score_distribution":
		return renderScoreDistributionChart(title, data, options)
	default:
		return "", chartRenderMeta{}, fmt.Errorf("unsupported chart_type: %s", chartType)
	}
}

var _ tools.Tool = (*GenerateChartTool)(nil)
