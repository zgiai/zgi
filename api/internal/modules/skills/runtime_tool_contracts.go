package skills

import (
	"fmt"
	"strings"
)

func SkillToolArgumentContractFor(skillID string, toolName string) (SkillToolArgumentContract, bool) {
	skillID = normalizeSkillID(skillID)
	toolName = strings.TrimSpace(toolName)
	key := skillID + "/" + toolName
	contract, ok := skillToolArgumentContracts()[key]
	return contract, ok
}

func skillToolArgumentContracts() map[string]SkillToolArgumentContract {
	return map[string]SkillToolArgumentContract{
		SkillCalculator + "/evaluate_expression": {
			SkillID:     SkillCalculator,
			ToolName:    "evaluate_expression",
			Description: "Evaluate one deterministic arithmetic expression.",
			Schema: objectSchema(
				map[string]interface{}{
					"expression": map[string]interface{}{
						"type":        "string",
						"description": "Arithmetic expression to evaluate, such as 23*17+9. Only numbers, parentheses, +, -, *, /, %, and ^ are allowed.",
					},
					"precision": precisionSchema(),
				},
				[]string{"expression"},
			),
			Example: map[string]interface{}{"expression": "23*17+9"},
		},
		SkillCalculator + "/calculate": {
			SkillID:     SkillCalculator,
			ToolName:    "calculate",
			Description: "Perform deterministic binary arithmetic between two numbers.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation": enumStringSchema("Arithmetic operation.", []string{"add", "subtract", "multiply", "divide", "power", "mod"}),
					"left":      numberSchema("Left operand."),
					"right":     numberSchema("Right operand."),
					"precision": precisionSchema(),
				},
				[]string{"operation", "left", "right"},
			),
			Example: map[string]interface{}{"operation": "multiply", "left": 23, "right": 17},
		},
		SkillCalculator + "/percentage": {
			SkillID:     SkillCalculator,
			ToolName:    "percentage",
			Description: "Calculate percent-of, percentage change, or apply a percentage increase/decrease.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation": enumStringSchema("Percentage operation. percent_of/apply_* require value and percent; change requires from and to.", []string{"percent_of", "change", "apply_increase", "apply_decrease"}),
					"value":     numberSchema("Base value for percent_of, apply_increase, and apply_decrease."),
					"percent":   numberSchema("Percentage value, such as 15 for 15 percent."),
					"from":      numberSchema("Original value for change."),
					"to":        numberSchema("New value for change."),
					"precision": precisionSchema(),
				},
				[]string{"operation"},
			),
			Example: map[string]interface{}{"operation": "percent_of", "value": 200, "percent": 15},
		},
		SkillConsoleNavigator + "/navigate": {
			SkillID:     SkillConsoleNavigator,
			ToolName:    "navigate",
			Description: "Request navigation to a whitelisted internal ZGI console page. This only changes the visible page and does not mutate assets.",
			Schema: objectSchema(
				map[string]interface{}{
					"href":   stringValueSchema("Required whitelisted internal /console route, such as /console/files, /console/agents, /console/workflows, /console/dataset, /console/db, /console/work/task, /console/prompts, /console/work/chat, /console/work/image, /console/work/app, /console/workspace, or /console/settings. Do not use external URLs."),
					"reason": stringValueSchema("Optional short user-facing reason for why the route is relevant."),
				},
				[]string{"href"},
			),
			Example: map[string]interface{}{"href": "/console/files", "reason": "The user asked to open file management."},
		},
		SkillFileGenerator + "/generate_file": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_file",
			Description: "Generate a downloadable temporary file artifact from provided content. This does not write to File Management; use file-manager/save_file_to_management after generation when the user explicitly asks to save into File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Text content to write into the generated file. Use valid CSV content for xlsx, runnable HTML content for html, and a complete self-contained <svg> document for svg."),
					"format":    enumStringSchema("Output format.", []string{"txt", "md", "html", "json", "csv", "svg", "docx", "xlsx", "pdf"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated HTML, XLSX, and PDF files. For XLSX, this becomes a merged title row above the table."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{"content": "# Report\n\nSummary...", "format": "md", "filename": "report"},
		},
		SkillFileReader + "/read_file": {
			SkillID:     SkillFileReader,
			ToolName:    "read_file",
			Description: "Read extracted text content from one file available in the current AIChat context.",
			Schema: objectSchema(
				map[string]interface{}{
					"file_id":   stringValueSchema("Required file ID from the current page context, attachment context, or governed asset resolution. Do not invent IDs."),
					"max_chars": numberSchema("Optional maximum returned content characters. Defaults to 4000 and is capped at 12000."),
				},
				[]string{"file_id"},
			),
			Example: map[string]interface{}{"file_id": "file_123", "max_chars": 4000},
		},
		SkillFileReader + "/list_visible_files": {
			SkillID:     SkillFileReader,
			ToolName:    "list_visible_files",
			Description: "List files visible in the current Console Files page context without reading file contents.",
			Schema: objectSchema(
				map[string]interface{}{},
				nil,
			),
			Example: map[string]interface{}{},
		},
		SkillFileManager + "/delete_file": {
			SkillID:     SkillFileManager,
			ToolName:    "delete_file",
			Description: "Delete one resolved File Management file after tool governance approval.",
			Schema: objectSchema(
				map[string]interface{}{
					"file_id": stringValueSchema("Required file ID from the current Files page context or governed asset resolution. Do not invent IDs."),
				},
				[]string{"file_id"},
			),
			Example: map[string]interface{}{"file_id": "file_123"},
		},
		SkillFileManager + "/save_file_to_management": {
			SkillID:     SkillFileManager,
			ToolName:    "save_file_to_management",
			Description: "Save a generated tool file or public external file URL into File Management after file.create governance allows it.",
			Schema: objectSchema(
				map[string]interface{}{
					"source_type":  enumStringSchema("Required source type. Use tool_file for a file generated by another tool; use url for a public external file URL.", []string{"tool_file", "url"}),
					"tool_file_id": stringValueSchema("Required when source_type is tool_file. Use the file_id/tool_file_id returned by the generation tool. Do not invent IDs."),
					"url":          stringValueSchema("Required when source_type is url. Must be an absolute public http or https URL supplied by the user."),
					"filename":     stringValueSchema("Required destination filename shown in File Management. Include a suitable extension and do not include path separators."),
					"workspace_id": stringValueSchema("Optional target workspace ID. Usually omit so current AIChat workspace context is used. Do not invent IDs."),
				},
				[]string{"source_type", "filename"},
			),
			Example: map[string]interface{}{"source_type": "tool_file", "tool_file_id": "tool_file_123", "filename": "report.pdf"},
		},
		SkillFileGenerator + "/generate_docx": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_docx",
			Description: "Generate a styled DOCX temporary artifact from a structured JSON document specification. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"document":  stringValueSchema("JSON string describing the DOCX document. Include blocks with type heading, paragraph, table, or page_break."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title hint; visible content must be included in document.blocks."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"document"},
			),
			Example: map[string]interface{}{
				"document": `{"blocks":[{"type":"heading","level":1,"text":"Report","style":{"alignment":"center","font_size":18,"bold":true}},{"type":"paragraph","runs":[{"text":"Total: "},{"text":"113.47","bold":true,"color":"C00000"}]}]}`,
				"filename": "styled-report",
			},
		},
		SkillFileGenerator + "/generate_pdf": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_pdf",
			Description: "Generate a styled PDF temporary artifact from self-contained HTML and inline CSS. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"html":      stringValueSchema("Self-contained HTML body or full HTML document. Do not include external URLs, scripts, iframes, or remote assets."),
					"css":       stringValueSchema("Optional inline CSS appended to the HTML document. Prefer @page for page size and margins."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title used when wrapping an HTML fragment. Visible content must be included in html."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"html"},
			),
			Example: map[string]interface{}{
				"html":     `<main><h1>Report</h1><p>Total: <strong class="amount">113.47</strong></p></main>`,
				"css":      `@page { size: A4; margin: 18mm; } h1 { text-align: center; } .amount { color: #c00000; }`,
				"filename": "styled-report",
			},
		},
		SkillFileGenerator + "/generate_pptx": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_pptx",
			Description: "Generate an editable static PPTX temporary artifact from a structured JSON presentation specification. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"presentation": stringValueSchema("JSON string describing the PPTX presentation. Include slides with elements of type title, text, table, or shape. Use non-overlapping boxes for readable content; omitted boxes use simple auto layout."),
					"filename":     stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":        stringValueSchema("Optional title hint; visible content must be included in presentation.slides."),
					"lifecycle":    enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"presentation"},
			),
			Example: map[string]interface{}{
				"presentation": `{"layout":"wide","slides":[{"elements":[{"type":"title","text":"Quarterly Report","style":{"align":"center"}},{"type":"text","text":"Total revenue: 113.47","x":0.8,"y":1.4,"w":11.6,"h":0.8,"style":{"font_size":24,"bold":true,"color":"C00000"}}]}]}`,
				"filename":     "quarterly-report",
			},
		},
		SkillSensitiveRedaction + "/redact_text": {
			SkillID:     SkillSensitiveRedaction,
			ToolName:    "redact_text",
			Description: "Detect and redact sensitive information from text. Use only after source text or parsed document content is available. Never pass unredacted content to file generation; call this tool first.",
			Schema: objectSchema(
				map[string]interface{}{
					"text":     stringValueSchema("Source text to redact. Required. Do not pass binary file contents."),
					"level":    enumStringSchema("Redaction level. Defaults to medium. Use high for external sharing, model training, logs, contracts, resumes, HR data, or customer data.", []string{"low", "medium", "high"}),
					"strategy": enumStringSchema("Redaction strategy. Defaults to auto. Secrets, tokens, passwords, and private keys are fully hidden even under partial strategy.", []string{"auto", "partial", "full", "label"}),
					"preserve_rules": objectSchema(
						map[string]interface{}{
							"keep_last_digits":  numberSchema("How many trailing digits to keep for partial masking. Must be 0-8. Defaults to 4."),
							"keep_email_domain": booleanSchema("Whether to keep email domains during partial masking. Defaults to true."),
							"keep_city":         booleanSchema("Whether to keep city-level address context during partial masking. Defaults to false."),
							"keep_url_domain":   booleanSchema("Whether to keep URL domain/path while redacting sensitive query parameters. Defaults to true."),
						},
						nil,
					),
					"entity_types": map[string]interface{}{
						"description": "Optional entity type filter. Omit to scan all supported types.",
						"oneOf": []interface{}{
							arraySchema("Entity types to scan.", enumStringSchema("Entity type.", []string{"phone", "email", "id_card", "bank_card", "address", "name", "customer_name", "company", "order_id", "contract_id", "secret", "token", "password", "private_key", "ip", "url_parameter"})),
							stringValueSchema("Comma-separated entity types or JSON array string."),
						},
					},
					"locale":             enumStringSchema("Locale hint. Defaults to auto.", []string{"auto", "zh-CN", "en-US"}),
					"include_field_list": booleanSchema("Whether to return redacted field summaries. Defaults to true. Field summaries never contain complete original sensitive values."),
				},
				[]string{"text"},
			),
			Example: map[string]interface{}{
				"text":     "Name: Zhang San, phone: 13812345678, token=abcdef1234567890",
				"level":    "high",
				"strategy": "auto",
			},
		},
		SkillChartGenerator + "/generate_chart": {
			SkillID:     SkillChartGenerator,
			ToolName:    "generate_chart",
			Description: "Generate a downloadable SVG chart artifact from structured data after prompt-professionalizer has been loaded and chart type, title, data mapping, and rendering style have been provided or confirmed. Supports radar, bar, line, pie, doughnut, scatter, and score_distribution. For generic chart requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"chart_type":      enumStringSchema("Chart type.", []string{"radar", "bar", "line", "pie", "doughnut", "scatter", "score_distribution"}),
					"title":           stringValueSchema("Optional chart title."),
					"output_filename": stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"data":            chartDataSchema(),
					"options": objectSchema(
						map[string]interface{}{
							"width":       numberSchema("Optional SVG width. Defaults to 900."),
							"height":      numberSchema("Optional SVG height. Defaults to 700 for radar and 620 for bar/line."),
							"style":       enumStringSchema("Rendering style.", []string{"simple", "business", "teaching", "comparison"}),
							"show_values": booleanSchema("Whether to show point values. Defaults to true."),
							"show_labels": booleanSchema("Whether to show scatter point labels. Defaults to true."),
							"legend":      booleanSchema("Whether to show legend. Defaults to true."),
							"grid":        booleanSchema("Whether to show grid lines. Defaults to true for bar/line."),
						},
						nil,
					),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"chart_type", "data"},
			),
			Example: map[string]interface{}{
				"chart_type":      "radar",
				"title":           "Score Comparison",
				"output_filename": "score-radar",
				"data": map[string]interface{}{
					"dimensions": []string{"Chinese", "Math", "English", "Physics", "Chemistry", "Biology"},
					"max_value":  100,
					"series": []map[string]interface{}{
						{"name": "Class Average", "values": []int{78, 82, 80, 75, 73, 76}},
						{"name": "Student", "values": []int{88, 92, 84, 81, 77, 86}},
					},
				},
			},
		},
		SkillIntentRouter + "/route_intent": {
			SkillID:     SkillIntentRouter,
			ToolName:    "route_intent",
			Description: "Validate and normalize a structured intent routing result. The model must classify the user's real intent first, then pass a stable task type, confidence, recommended action, evidence, missing information, routing hints, and normalized request.",
			Schema: objectSchema(
				map[string]interface{}{
					"user_input":              stringValueSchema("The current user message being classified."),
					"context_summary":         stringValueSchema("Optional concise summary of relevant conversation context."),
					"uploaded_files":          intentUploadedFilesSchema(),
					"intent_id":               stringValueSchema("Stable dotted lowercase identifier such as file_generation.docx or database_query.filter_records."),
					"task_type":               enumStringSchema("Standard task type.", intentTaskTypes()),
					"subtype":                 stringValueSchema("Optional normalized subtype such as docx, bar, filter_records, or unknown."),
					"confidence":              boundedNumberSchema("Confidence from 0 to 1.", 0, 1),
					"recommended_action":      enumStringSchema("Recommended next action.", intentRecommendedActions()),
					"recommended_skill_id":    enumStringSchema("Optional target skill ID when recommended_action is call_skill.", []string{"file-generator", "chart-generator", "work-report-generator", "schedule-planner", "calculator", "internal-knowledge", "agent-knowledge", "internal-database", "agent-database", "agent-workflow"}),
					"recommended_tool_name":   stringValueSchema("Optional target tool name when recommended_action is call_tool."),
					"recommended_workflow_id": stringValueSchema("Optional workflow or workflow binding identifier when known."),
					"recommended_database_id": stringValueSchema("Optional database identifier when known."),
					"recommended_dataset_ids": intentStringArraySchema("Optional knowledge base or dataset IDs when known."),
					"routing_hints":           intentRoutingHintsSchema(),
					"missing_info":            intentMissingInfoSchema(),
					"evidence":                intentStringArraySchema("Evidence strings grounded in the user input or supplied context."),
					"normalized_request":      stringValueSchema("Concise restatement of what the user is actually asking."),
					"alternate_intents":       intentAlternateIntentsSchema(),
				},
				[]string{"user_input", "intent_id", "task_type", "confidence", "recommended_action", "evidence", "normalized_request"},
			),
			Example: map[string]interface{}{
				"user_input":            "Export the current report as a Word document.",
				"intent_id":             "file_generation.docx",
				"task_type":             "file_generation",
				"subtype":               "docx",
				"confidence":            0.94,
				"recommended_action":    "call_skill",
				"recommended_skill_id":  "file-generator",
				"recommended_tool_name": "generate_docx",
				"routing_hints": map[string]interface{}{
					"requires_file_generation": true,
				},
				"missing_info":       []map[string]interface{}{},
				"evidence":           []string{"User explicitly asked to export as a Word document."},
				"normalized_request": "Generate a DOCX file from the current report.",
			},
		},
		SkillArchitectureDiagram + "/generate_architecture_diagram": {
			SkillID:     SkillArchitectureDiagram,
			ToolName:    "generate_architecture_diagram",
			Description: "Generate downloadable SVG and HTML technical diagram artifacts after prompt-professionalizer has been loaded and diagram type, title, scope, and rendering style have been provided or confirmed. Supports system_architecture, agent_architecture, data_flow, flowchart, comparison_matrix, sequence, state, and er. For generic diagram requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"diagram_type":    enumStringSchema("Diagram type.", []string{"system_architecture", "agent_architecture", "data_flow", "flowchart", "comparison_matrix", "sequence", "state", "er"}),
					"title":           stringValueSchema("Optional diagram title."),
					"description":     stringValueSchema("Optional short subtitle or source summary."),
					"output_filename": stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"data":            architectureDiagramDataSchema(),
					"options": objectSchema(
						map[string]interface{}{
							"formats":     arraySchema("Output formats. Defaults to svg and html.", enumStringSchema("Output format.", []string{"svg", "html"})),
							"width":       numberSchema("Optional SVG width. Defaults to 1200 and must be between 480 and 2400."),
							"height":      numberSchema("Optional SVG height. Defaults to 760 and must be between 320 and 1800."),
							"style":       enumStringSchema("Rendering style. Use technical for engineering docs, business for reports, presentation for slide-ready diagrams, paper for warm report visuals, and simple when unspecified.", []string{"simple", "business", "technical", "presentation", "paper"}),
							"direction":   enumStringSchema("Layout direction.", []string{"left_to_right", "top_to_bottom"}),
							"show_legend": booleanSchema("Whether to show legend when supported. Defaults to true."),
							"show_labels": booleanSchema("Whether to show edge labels. Defaults to true."),
						},
						nil,
					),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"diagram_type", "data"},
			),
			Example: map[string]interface{}{
				"diagram_type":    "agent_architecture",
				"title":           "RAG Agent Architecture",
				"output_filename": "rag-agent-architecture",
				"data": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"id": "user", "label": "User", "type": "actor", "layer": "input"},
						{"id": "agent", "label": "Agent Orchestrator", "type": "agent", "layer": "agent"},
						{"id": "retriever", "label": "Retriever", "type": "tool", "layer": "tools"},
						{"id": "vector", "label": "Vector Store", "type": "memory", "layer": "memory"},
						{"id": "llm", "label": "LLM", "type": "model", "layer": "model"},
					},
					"edges": []map[string]interface{}{
						{"from": "user", "to": "agent", "label": "query"},
						{"from": "agent", "to": "retriever", "label": "retrieve"},
						{"from": "retriever", "to": "vector", "label": "search"},
						{"from": "agent", "to": "llm", "label": "prompt + context"},
					},
				},
				"options": map[string]interface{}{"style": "technical", "formats": []string{"svg", "html"}},
			},
		},
		SkillImageGenerator + "/generate_image": {
			SkillID:     SkillImageGenerator,
			ToolName:    "generate_image",
			Description: "Generate downloadable image files from a text prompt after prompt-professionalizer has been loaded. Supports style, aspect ratio, count, negative prompt, and optional current-user reference image URL guidance. Reference images are passed as signed URLs in the prompt, not as structured image inputs. For generic image requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"prompt":          stringValueSchema("Required image description. Include subject, scene, composition, intended use, and constraints."),
					"style":           imageStyleSchema(),
					"aspect_ratio":    imageAspectRatioSchema(),
					"count":           numberSchema("Number of candidate images. Must be an integer from 1 to 4."),
					"negative_prompt": stringValueSchema("Optional elements, styles, or risks to avoid."),
					"reference_image": imageFileObjectSchema("Optional current-user reference image file object or file ID. The tool places a signed URL in the prompt for loose visual guidance; it is not a structured image input."),
					"filename":        stringValueSchema("Optional base filename. Do not include path separators or an extension."),
					"lifecycle":       enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
					"provider":        stringValueSchema("Optional explicit image model provider. Usually omit this and use the default image generation model."),
					"model":           stringValueSchema("Optional explicit image generation model. Usually omit this and use the default image generation model."),
				},
				[]string{"prompt"},
			),
			Example: map[string]interface{}{"prompt": "A clean product concept image of a smart desk lamp on a white studio background", "style": "product", "aspect_ratio": "1:1", "count": 1},
		},
		SkillImageGenerator + "/edit_image": {
			SkillID:     SkillImageGenerator,
			ToolName:    "edit_image",
			Description: "Create prompt-plus-reference-URL variants or edit-style regenerated images from a current-user reference image and instruction after prompt-professionalizer has been loaded. This is not precise in-place editing and does not pass structured image input to the provider. For ambiguous edits, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"image":            imageFileObjectSchema("Required current-user reference image file object or file ID. The tool places a signed URL in the prompt for loose visual guidance; it is not a structured image input."),
					"edit_instruction": stringValueSchema("Required edit or variant instruction. State what to change, preserve, and avoid."),
					"edit_type":        enumStringSchema("Edit type.", []string{"auto", "variant", "background", "color", "add_element", "remove_element", "style_transfer"}),
					"style":            imageStyleSchema(),
					"aspect_ratio":     imageAspectRatioSchema(),
					"count":            numberSchema("Number of candidate images. Must be an integer from 1 to 4."),
					"negative_prompt":  stringValueSchema("Optional elements, styles, or risks to avoid."),
					"filename":         stringValueSchema("Optional base filename. Do not include path separators or an extension."),
					"lifecycle":        enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
					"provider":         stringValueSchema("Optional explicit image model provider. Usually omit this and use the default image generation model."),
					"model":            stringValueSchema("Optional explicit image generation model. Usually omit this and use the default image generation model."),
				},
				[]string{"image", "edit_instruction"},
			),
			Example: map[string]interface{}{"image": map[string]interface{}{"upload_file_id": "file-id"}, "edit_instruction": "Change the background to a bright office scene and keep the main product shape", "edit_type": "background", "count": 1},
		},
		SkillWorkReport + "/generate_file": {
			SkillID:     SkillWorkReport,
			ToolName:    "generate_file",
			Description: "Generate a downloadable weekly, monthly, or work report artifact from prepared report content.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Final weekly, monthly, or work report content to write into the generated file."),
					"format":    enumStringSchema("Output format.", []string{"txt", "md", "docx", "pdf"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated PDF files."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{"content": "# Weekly Work Report\n\n## Summary\n\n...", "format": "md", "filename": "weekly-work-report"},
		},
		SkillContractFieldExtractor + "/generate_file": {
			SkillID:     SkillContractFieldExtractor,
			ToolName:    "generate_file",
			Description: "Generate a downloadable JSON, CSV, Markdown, or text file from completed contract field extraction results. Use only after contract text and configured fields have been processed and missing fields are marked explicitly.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Final contract extraction result content to write into the generated file. Preserve missing, uncertain, conflict, confidence, evidence, and source_location fields."),
					"format":    enumStringSchema("Output format.", []string{"json", "csv", "md", "txt"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated file formats that support titles."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{
				"content":  `{"contract_summary":{"field_count":2,"extracted_count":1,"missing_count":1},"fields":[{"field_key":"contract_amount","field_label":"Contract Amount","value":"CNY 120,000","normalized_value":"120000","value_type":"money","extraction_status":"extracted","confidence":0.92,"evidence":"The total contract price is CNY 120,000.","source_location":"Section 3","notes":""},{"field_key":"renewal_clause","field_label":"Renewal Clause","value":"Not found","normalized_value":"","value_type":"clause","extraction_status":"missing","confidence":0,"evidence":"","source_location":"","notes":"No explicit renewal clause was found in the contract text."}]}`,
				"format":   "json",
				"filename": "contract-field-extraction",
				"title":    "Contract Field Extraction",
			},
		},
		SkillInternalKnowledge + "/list_accessible_knowledge_bases": {
			SkillID:     SkillInternalKnowledge,
			ToolName:    "list_accessible_knowledge_bases",
			Description: "List knowledge bases accessible to the current AIChat user. Inspect status and fallback_used before selecting dataset IDs.",
			Schema: objectSchema(
				map[string]interface{}{
					"query": stringValueSchema("Optional search text for narrowing candidate knowledge bases."),
					"limit": numberSchema("Maximum number of knowledge bases to list. Defaults to 20 and is capped at 100."),
				},
				nil,
			),
			Example: map[string]interface{}{"query": "expense policy", "limit": 10},
		},
		SkillInternalKnowledge + "/retrieve_knowledge": {
			SkillID:     SkillInternalKnowledge,
			ToolName:    "retrieve_knowledge",
			Description: "Retrieve relevant context from selected accessible knowledge base IDs returned by list_accessible_knowledge_bases.",
			Schema: objectSchema(
				map[string]interface{}{
					"query":          stringValueSchema("The user question or refined search query."),
					"dataset_ids":    stringArrayOrCSVSchema("Knowledge base IDs selected from list_accessible_knowledge_bases. Pass a JSON array of IDs when possible."),
					"top_k":          numberSchema("Maximum number of retrieved chunks. Defaults to 5 and is capped at 20."),
					"retrieval_mode": enumStringSchema("Optional retrieval mode. Omit for default hybrid mode; use graph only for relationship/entity questions.", []string{"hybrid", "vector", "graph"}),
				},
				[]string{"query", "dataset_ids"},
			),
			Example: map[string]interface{}{"query": "What is the reimbursement policy?", "dataset_ids": []string{"dataset-id"}},
		},
		SkillAgentKnowledge + "/retrieve_agent_knowledge": {
			SkillID:     SkillAgentKnowledge,
			ToolName:    "retrieve_agent_knowledge",
			Description: "Retrieve relevant context from knowledge bases bound to the current Agent. Do not pass dataset IDs.",
			Schema: objectSchema(
				map[string]interface{}{
					"query":          stringValueSchema("The user question or refined search query."),
					"top_k":          numberSchema("Maximum number of retrieved chunks. Defaults to 5 and is capped at 20."),
					"retrieval_mode": enumStringSchema("Optional retrieval mode. Omit for default hybrid mode; use graph only for relationship/entity questions.", []string{"hybrid", "vector", "graph"}),
				},
				[]string{"query"},
			),
			Example: map[string]interface{}{"query": "Summarize the configured product FAQ."},
		},
		SkillInternalDatabase + "/list_accessible_databases":             databaseListContract(SkillInternalDatabase),
		SkillInternalDatabase + "/list_database_tables":                  databaseTablesContract(SkillInternalDatabase),
		SkillInternalDatabase + "/describe_database_table":               databaseDescribeTableContract(SkillInternalDatabase),
		SkillInternalDatabase + "/query_table_records":                   databaseQueryRecordsContract(SkillInternalDatabase),
		SkillInternalDatabase + "/insert_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "insert_table_records", "Insert records into a database table."),
		SkillInternalDatabase + "/update_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "update_table_records", "Update records in a database table. Each record must include id."),
		SkillInternalDatabase + "/delete_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "delete_table_records", "Delete records from a database table. Each record must include id."),
		SkillAgentDatabase + "/list_accessible_databases":                databaseListContract(SkillAgentDatabase),
		SkillAgentDatabase + "/list_database_tables":                     databaseTablesContract(SkillAgentDatabase),
		SkillAgentDatabase + "/describe_database_table":                  databaseDescribeTableContract(SkillAgentDatabase),
		SkillAgentDatabase + "/query_table_records":                      databaseQueryRecordsContract(SkillAgentDatabase),
		SkillAgentDatabase + "/insert_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "insert_table_records", "Insert records into an Agent-bound database table."),
		SkillAgentDatabase + "/update_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "update_table_records", "Update records in an Agent-bound database table. Each record must include id."),
		SkillAgentDatabase + "/delete_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "delete_table_records", "Delete records from an Agent-bound database table. Each record must include id."),
		SkillAgentWorkflow + "/list_agent_workflows":                     workflowListContract(),
		SkillAgentWorkflow + "/run_agent_workflow":                       workflowRunContract(),
		SkillAgentWorkflow + "/get_workflow_run_status":                  workflowRunStatusContract(),
		SkillAgentManagement + "/list_agents":                            agentManagementListAgentsContract(),
		SkillAgentManagement + "/get_agent":                              agentManagementAgentIDContract("get_agent", "Read basic details for one resolved Agent asset visible to the current AIChat user."),
		SkillAgentManagement + "/create_agent":                           agentManagementCreateAgentContract(),
		SkillAgentManagement + "/update_agent_identity":                  agentManagementUpdateIdentityContract(),
		SkillAgentManagement + "/delete_agent":                           agentManagementAgentIDContract("delete_agent", "Delete one resolved Agent after explicit governance approval."),
		SkillAgentManagement + "/delete_agents":                          agentManagementDeleteAgentsContract(),
		SkillAgentManagement + "/get_agent_config":                       agentManagementAgentIDContract("get_agent_config", "Read the current draft runtime configuration for one resolved AGENT asset."),
		SkillAgentManagement + "/update_agent_config":                    agentManagementUpdateConfigContract(),
		SkillAgentManagement + "/replace_agent_memory_slots":             agentManagementReplaceMemorySlotsContract(),
		SkillAgentManagement + "/list_agent_skill_candidates":            agentManagementBindingCandidateContract("list_agent_skill_candidates", "List user-selectable, Agent-bindable skills for one resolved AGENT asset."),
		SkillAgentManagement + "/list_agent_knowledge_candidates":        agentManagementBindingCandidateContract("list_agent_knowledge_candidates", "List knowledge bases that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_database_candidates":         agentManagementBindingCandidateContract("list_agent_database_candidates", "List databases that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_database_tables":             agentManagementBindingCandidateContract("list_agent_database_tables", "List database tables that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_workflow_binding_candidates": agentManagementBindingCandidateContract("list_agent_workflow_binding_candidates", "List workflows that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_available_models":                  agentManagementListAvailableModelsContract(),
		SkillTime + "/current_time": {
			SkillID:     SkillTime,
			ToolName:    "current_time",
			Description: "Get the current system time with optional timezone and format.",
			Schema: objectSchema(
				map[string]interface{}{
					"format":   stringValueSchema("Optional strftime-style output format. Defaults to %Y-%m-%d %H:%M:%S."),
					"timezone": stringValueSchema("Optional IANA timezone such as Asia/Shanghai. Defaults to UTC."),
				},
				nil,
			),
			Example: map[string]interface{}{"timezone": "Asia/Shanghai", "format": "%Y-%m-%d %H:%M:%S"},
		},
		SkillTime + "/date_calculate": {
			SkillID:     SkillTime,
			ToolName:    "date_calculate",
			Description: "Add or subtract date intervals, or calculate the day interval between two dates.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation":   enumStringSchema("Operation to perform. diff requires target_date.", []string{"add", "subtract", "diff"}),
					"base_date":   stringValueSchema("Base date in YYYY-MM-DD format. Use today or omit to use the current date."),
					"amount":      numberSchema("Interval amount for add or subtract. Defaults to 1."),
					"unit":        enumStringSchema("Interval unit for add or subtract.", []string{"day", "week", "month", "year"}),
					"target_date": stringValueSchema("Target date in YYYY-MM-DD format. Required when operation is diff."),
					"timezone":    stringValueSchema("IANA timezone used when base_date is omitted. Defaults to UTC."),
				},
				[]string{"operation"},
			),
			Example: map[string]interface{}{"operation": "add", "base_date": "today", "amount": 3, "unit": "day", "timezone": "Asia/Shanghai"},
		},
	}
}

func ExpectedSkillToolArguments(skillID string, toolName string) map[string]interface{} {
	contract, ok := SkillToolArgumentContractFor(skillID, toolName)
	if !ok {
		return nil
	}
	return map[string]interface{}{
		"skill_id":    contract.SkillID,
		"tool_name":   contract.ToolName,
		"description": contract.Description,
		"schema":      contract.Schema,
		"example":     contract.Example,
	}
}

func validateSkillToolArgumentsAgainstContract(skillID string, toolName string, arguments map[string]interface{}) error {
	contract, ok := SkillToolArgumentContractFor(skillID, toolName)
	if !ok {
		return nil
	}
	required := schemaRequiredFields(contract.Schema)
	if len(required) == 0 {
		return nil
	}
	missing := make([]string, 0, len(required))
	for _, field := range required {
		if !argumentValuePresent(arguments[field]) {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("skill tool %s/%s missing required argument(s): %s", normalizeSkillID(skillID), strings.TrimSpace(toolName), strings.Join(missing, ", "))
}

func schemaRequiredFields(schema map[string]interface{}) []string {
	if len(schema) == 0 {
		return nil
	}
	values, ok := schema["required"]
	if !ok || values == nil {
		return nil
	}
	switch typed := values.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := strings.TrimSpace(value); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func argumentValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
}
