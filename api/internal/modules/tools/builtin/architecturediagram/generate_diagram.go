package architecturediagram

import (
	"context"
	"fmt"
	"strings"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// GenerateArchitectureDiagramTool creates SVG and HTML diagram files.
type GenerateArchitectureDiagramTool struct {
	*builtin.BuiltinTool
	runtime *tools.ToolRuntime
}

// NewGenerateArchitectureDiagramTool creates a generate_architecture_diagram tool.
func NewGenerateArchitectureDiagramTool(tenantID string) *GenerateArchitectureDiagramTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "generate_architecture_diagram",
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US":   "Generate Architecture Diagram",
				"zh_Hans": "Generate Architecture Diagram",
			},
			Icon: "workflow",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Generate downloadable SVG and HTML technical diagrams from structured data.",
				"zh_Hans": "Generate downloadable SVG and HTML technical diagrams from structured data.",
			},
			LLM: "Generate downloadable SVG and HTML technical diagram artifacts from structured data. Supports system_architecture, agent_architecture, data_flow, flowchart, comparison_matrix, sequence, state, and er. For casual, vague, incomplete, or non-structured diagram requests, use prompt-professionalizer before this tool. For generic diagram requests, do not infer the diagram type, title, scope, or style; ask the user to confirm missing decisions before calling.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "diagram_type",
				Label:            tools.I18nText{"en_US": "Diagram Type", "zh_Hans": "Diagram Type"},
				HumanDescription: tools.I18nText{"en_US": "Diagram type to generate.", "zh_Hans": "Diagram type to generate."},
				LLMDescription:   "Diagram type: system_architecture, agent_architecture, data_flow, flowchart, comparison_matrix, sequence, state, or er.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Default:          "system_architecture",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "system_architecture", Label: tools.I18nText{"en_US": "System Architecture", "zh_Hans": "System Architecture"}},
					{Value: "agent_architecture", Label: tools.I18nText{"en_US": "Agent Architecture", "zh_Hans": "Agent Architecture"}},
					{Value: "data_flow", Label: tools.I18nText{"en_US": "Data Flow", "zh_Hans": "Data Flow"}},
					{Value: "flowchart", Label: tools.I18nText{"en_US": "Flowchart", "zh_Hans": "Flowchart"}},
					{Value: "comparison_matrix", Label: tools.I18nText{"en_US": "Comparison Matrix", "zh_Hans": "Comparison Matrix"}},
					{Value: "sequence", Label: tools.I18nText{"en_US": "Sequence", "zh_Hans": "Sequence"}},
					{Value: "state", Label: tools.I18nText{"en_US": "State", "zh_Hans": "State"}},
					{Value: "er", Label: tools.I18nText{"en_US": "ER", "zh_Hans": "ER"}},
				},
			},
			{
				Name:             "data",
				Label:            tools.I18nText{"en_US": "Data", "zh_Hans": "Data"},
				HumanDescription: tools.I18nText{"en_US": "Diagram-specific structured data.", "zh_Hans": "Diagram-specific structured data."},
				LLMDescription:   "Structured diagram data object or JSON object string. Node-edge diagrams use nodes, edges, and optional groups; comparison_matrix uses rows, columns, and cells; sequence uses participants and messages; state uses states and transitions; er uses entities and relationships.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "title",
				Label:            tools.I18nText{"en_US": "Title", "zh_Hans": "Title"},
				HumanDescription: tools.I18nText{"en_US": "Optional diagram title.", "zh_Hans": "Optional diagram title."},
				LLMDescription:   "Optional diagram title.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "description",
				Label:            tools.I18nText{"en_US": "Description", "zh_Hans": "Description"},
				HumanDescription: tools.I18nText{"en_US": "Optional short subtitle or source summary.", "zh_Hans": "Optional short subtitle or source summary."},
				LLMDescription:   "Optional short subtitle or source summary shown under the title.",
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
				LLMDescription:   "Optional rendering options as an object or JSON object string: formats, width, height, style, direction, show_legend, show_labels. Supported styles are simple, business, technical, presentation, and paper.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "lifecycle",
				Label:            tools.I18nText{"en_US": "Lifecycle", "zh_Hans": "Lifecycle"},
				HumanDescription: tools.I18nText{"en_US": "Whether generated files are persistent or temporary.", "zh_Hans": "Whether generated files are persistent or temporary."},
				LLMDescription:   "File lifecycle: persistent or temporary. Defaults to persistent.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "persistent",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "persistent", Label: tools.I18nText{"en_US": "Persistent", "zh_Hans": "Persistent"}},
					{Value: "temporary", Label: tools.I18nText{"en_US": "Temporary", "zh_Hans": "Temporary"}},
				},
			},
		},
		OutputType: "file",
		Tags:       []string{"visualization", "diagram", "architecture"},
	}
	return &GenerateArchitectureDiagramTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *GenerateArchitectureDiagramTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewGenerateArchitectureDiagramTool(tenantID)
	fork.runtime = runtime
	return fork
}

// Invoke generates SVG and HTML diagram files and returns workflow file metadata.
func (t *GenerateArchitectureDiagramTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	diagramType := normalizeDiagramType(rawStringParam(toolParameters, "diagram_type"))
	if diagramType == "" {
		return nil, fmt.Errorf("diagram_type is required")
	}
	data, err := mapParam(toolParameters, "data")
	if err != nil {
		return nil, err
	}
	options, err := optionalMapParam(toolParameters, "options")
	if err != nil {
		return nil, err
	}
	spec, err := parseDiagramData(
		diagramType,
		rawStringParam(toolParameters, "title"),
		rawStringParam(toolParameters, "description"),
		data,
		options,
	)
	if err != nil {
		return nil, err
	}
	svg, htmlDoc, meta, err := renderDiagram(spec)
	if err != nil {
		return nil, err
	}
	if len(svg) > maxDiagramFileBytes || len(htmlDoc) > maxDiagramFileBytes {
		return nil, fmt.Errorf("generated diagram exceeds %d bytes", maxDiagramFileBytes)
	}
	lifecycle, err := resolveDiagramFileLifecycle(rawStringParam(toolParameters, "lifecycle"))
	if err != nil {
		return nil, err
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

	baseName := firstNonEmpty(rawStringParam(toolParameters, "output_filename"), rawStringParam(toolParameters, "filename"), defaultDiagramFilename)
	files := []map[string]interface{}{}
	messages := []tools.ToolInvokeMessage{}
	for _, format := range spec.Options.Formats {
		var data []byte
		var mimeType string
		var extension string
		var fileType workflowfile.FileType
		switch format {
		case "svg":
			data = []byte(svg)
			mimeType = svgMimeType
			extension = ".svg"
			fileType = workflowfile.FileTypeImage
		case "html":
			data = []byte(htmlDoc)
			mimeType = htmlMimeType
			extension = ".html"
			fileType = workflowfile.FileTypeDocument
		default:
			return nil, fmt.Errorf("unsupported output format: %s", format)
		}
		fileMeta, fileMessage, err := createDiagramFile(ctx, tenantID, userID, conversationID, data, mimeType, extension, buildDiagramFilename(baseName, extension), lifecycle, fileType, format)
		if err != nil {
			return nil, err
		}
		files = append(files, fileMeta)
		messages = append(messages, fileMessage)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("at least one output format is required")
	}
	primary := files[0]
	jsonPayload := map[string]interface{}{
		"file_id":      primary["file_id"],
		"filename":     primary["filename"],
		"format":       primary["format"],
		"mime_type":    primary["mime_type"],
		"url":          primary["url"],
		"download_url": primary["download_url"],
		"diagram_type": meta.DiagramType,
		"node_count":   meta.NodeCount,
		"edge_count":   meta.EdgeCount,
		"files":        files,
	}
	messages = append(messages, builtin.CreateJSONMessage(jsonPayload))
	return messages, nil
}

func createDiagramFile(
	ctx context.Context,
	tenantID string,
	userID string,
	conversationID *string,
	data []byte,
	mimeType string,
	extension string,
	filename string,
	lifecycle tool_file.ToolFileLifecycle,
	fileType workflowfile.FileType,
	format string,
) (map[string]interface{}, tools.ToolInvokeMessage, error) {
	toolFile, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:         userID,
		TenantID:       tenantID,
		ConversationID: conversationID,
		FileData:       data,
		MimeType:       mimeType,
		Filename:       &filename,
		Lifecycle:      lifecycle,
	})
	if err != nil {
		return nil, tools.ToolInvokeMessage{}, fmt.Errorf("failed to create generated diagram: %w", err)
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, tools.ToolInvokeMessage{}, fmt.Errorf("failed to sign generated diagram: %w", err)
	}
	downloadURL := appendDownloadQuery(url)
	fileObj := workflowfile.NewFile(
		tenantID,
		fileType,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	workflowMeta := fileObj.ToDict()
	workflowMeta["url"] = url
	workflowMeta["download_url"] = downloadURL
	workflowMeta["lifecycle"] = toolFile.Lifecycle
	if toolFile.ExpiresAt != nil {
		workflowMeta["expires_at"] = toolFile.ExpiresAt.Unix()
	}
	fileMeta := map[string]interface{}{
		"file_id":      toolFile.ID,
		"filename":     toolFile.Name,
		"format":       format,
		"mime_type":    mimeType,
		"size":         toolFile.Size,
		"url":          url,
		"download_url": downloadURL,
		"lifecycle":    toolFile.Lifecycle,
	}
	if toolFile.ExpiresAt != nil {
		fileMeta["expires_at"] = toolFile.ExpiresAt.Unix()
	}
	return fileMeta, tools.ToolInvokeMessage{
		Type: tools.ToolInvokeMessageTypeFile,
		Text: downloadURL,
		Meta: map[string]interface{}{"file": workflowMeta},
	}, nil
}

var _ tools.Tool = (*GenerateArchitectureDiagramTool)(nil)
