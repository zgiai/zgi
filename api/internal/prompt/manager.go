package prompt

import (
	"embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed templates/*/*.tpl
var templateFiles embed.FS

var templates = make(map[TemplateID]*Template)

// RegisterTemplate registers a prompt template, requiring complete metadata.
func RegisterTemplate(id TemplateID, metadata TemplateMetadata, fileName string) error {
	content, err := templateFiles.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("failed to read template file %s: %w", fileName, err)
	}

	parsed, err := template.New(string(id)).Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", fileName, err)
	}

	templates[id] = &Template{
		ID:         id,
		Metadata:   metadata,
		rawContent: string(content),
		goTemplate: parsed,
	}

	return nil
}

// GetTemplate gets a prompt template.
func GetTemplate(id TemplateID) (*Template, error) {
	tmpl, exists := templates[id]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", string(id))
	}
	return tmpl, nil
}

// init initializes all built-in templates.
func init() {
	err := RegisterTemplate(
		DatasourceTableAnalysis,
		TemplateMetadata{
			Module:      "datasource",
			Purpose:     "Analyze file content to infer table structure",
			Description: "Used for analyzing file content and inferring table structure for the AnalyzeFileForTable method",
			CreatedAt:   time.Date(2025, time.October, 15, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/datasource/table_analysis.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register table analysis template: %v", err))
	}

	err = RegisterTemplate(
		DatasourceFileConversion,
		TemplateMetadata{
			Module:      "datasource",
			Purpose:     "Convert file content to table records",
			Description: "Used for converting file content to table records for the IngestFileToTable and BatchIngestFileToTable methods",
			CreatedAt:   time.Date(2025, time.October, 15, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/datasource/file_conversion.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register file conversion template: %v", err))
	}

	err = RegisterTemplate(
		DatasetQuestionGeneration,
		TemplateMetadata{
			Module:      "dataset",
			Purpose:     "Generate questions based on text content",
			Description: "Used for generating questions based on document segment content for the GenerateQuestionsForSegment method",
			CreatedAt:   time.Date(2025, time.October, 15, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/dataset/question_generation.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register question generation template: %v", err))
	}

	err = RegisterTemplate(
		CommonConversationTitle,
		TemplateMetadata{
			Module:      "common",
			Purpose:     "Generate concise conversation titles",
			Description: "Used by chat-like products to generate sidebar titles from conversation messages",
			CreatedAt:   time.Date(2026, time.May, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/common/conversation_title.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register common conversation title template: %v", err))
	}

	err = RegisterTemplate(
		AIChatSystem,
		TemplateMetadata{
			Module:      "aichat",
			Purpose:     "Default AIChat system prompt",
			Description: "Used as the internal system prompt for AIChat conversations",
			CreatedAt:   time.Date(2026, time.May, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/aichat/chat_system.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register aichat system template: %v", err))
	}

	err = RegisterTemplate(
		DatasourceDefaultUserIngestZh,
		TemplateMetadata{
			Module:      "datasource",
			Purpose:     "Default user ingest prompt for table analysis in Chinese",
			Description: "Used as the default user ingest prompt when analyzing file content to infer table structure in Chinese",
			CreatedAt:   time.Date(2025, time.October, 28, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/datasource/default_user_ingest_zh-CN.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register default user prompt template (zh-CN): %v", err))
	}

	err = RegisterTemplate(
		DatasourceDefaultUserIngestEn,
		TemplateMetadata{
			Module:      "datasource",
			Purpose:     "Default user ingest prompt for table analysis in English",
			Description: "Used as the default user ingest prompt when analyzing file content to infer table structure in English",
			CreatedAt:   time.Date(2025, time.October, 28, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/datasource/default_user_ingest.tpl",
	)
	if err != nil {
		panic(fmt.Errorf("failed to register default user prompt template (en-US): %w", err))
	}

	// Register the workflow diagnosis system prompt
	err = RegisterTemplate(
		WorkflowDiagnosisSystem,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Workflow error diagnosis system prompt",
			Description: "Used to guide the LLM to perform error root cause analysis",
			CreatedAt:   time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/diagnosis_system.tpl",
	)
	if err != nil {
		panic(fmt.Errorf("failed to register diagnosis system prompt: %w", err))
	}

	// Register the workflow diagnosis user prompt (Chinese)
	err = RegisterTemplate(
		WorkflowDiagnosisUserZh,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Workflow error diagnosis user prompt (Chinese)",
			Description: "Used to format execution context for error diagnosis in Chinese",
			CreatedAt:   time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/diagnosis_user_zh-CN.tpl",
	)
	if err != nil {
		panic(fmt.Errorf("failed to register diagnosis user prompt (zh-CN): %w", err))
	}

	// Register the workflow diagnosis user prompt (English)
	err = RegisterTemplate(
		WorkflowDiagnosisUserEn,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Workflow error diagnosis user prompt (English)",
			Description: "Used to format execution context for error diagnosis in English",
			CreatedAt:   time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/diagnosis_user.tpl",
	)
	if err != nil {
		panic(fmt.Errorf("failed to register diagnosis user prompt (en-US): %w", err))
	}

	err = RegisterTemplate(
		GraphFlowQueryEntityExtraction,
		TemplateMetadata{
			Module:      "graphflow",
			Purpose:     "Extract and expand query entities for graph traversal",
			Description: "Used when GraphFlow expands search queries into robust entity seed nodes",
			CreatedAt:   time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/graphflow/query_entity_extraction.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register graphflow query entity extraction template: %v", err))
	}

	err = RegisterTemplate(
		GraphFlowGlobalEntitySummary,
		TemplateMetadata{
			Module:      "graphflow",
			Purpose:     "Summarize document-level core entities",
			Description: "Used before segment extraction to establish the document-level entity context",
			CreatedAt:   time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/graphflow/global_entity_summary.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register graphflow global entity summary template: %v", err))
	}

	err = RegisterTemplate(
		GraphFlowGraphExtraction,
		TemplateMetadata{
			Module:      "graphflow",
			Purpose:     "Extract document segment entities and relationships",
			Description: "Used by the main GraphFlow ingestion pipeline to extract graph entities and relations from each segment",
			CreatedAt:   time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/graphflow/graph_extraction.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register graphflow graph extraction template: %v", err))
	}

	err = RegisterTemplate(
		GraphFlowOpenIEExtraction,
		TemplateMetadata{
			Module:      "graphflow",
			Purpose:     "Schema-guided OpenIE extraction",
			Description: "Used by the GraphFlow OpenIE strategy to extract entities and triples with optional ontology constraints",
			CreatedAt:   time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/graphflow/openie_extraction.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register graphflow OpenIE extraction template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowSQLGeneratorSystem,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Default SQL generator system prompt",
			Description: "Used by the workflow SQL generator node when no node-level system prompt is provided",
			CreatedAt:   time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/sql_generator_system.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow SQL generator system prompt template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorSystem,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor system prompt",
			Description: "Used by the workflow parameter extractor node in prompt engineering chat mode",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_system.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor system prompt template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorChatUser,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor chat user prompt",
			Description: "Used by the workflow parameter extractor node to format the chat user prompt from structure, instruction, and text",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_chat_user.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor chat user prompt template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorCompletion,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor completion prompt",
			Description: "Used as the canonical completion-mode prompt template for the workflow parameter extractor node",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_completion.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor completion prompt template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorExampleUser1,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor few-shot example user 1",
			Description: "Used as the first few-shot user example for the workflow parameter extractor node",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_example_user_1.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor example user 1 template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorExampleAsst1,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor few-shot example assistant 1",
			Description: "Used as the first few-shot assistant example for the workflow parameter extractor node",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_example_assistant_1.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor example assistant 1 template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorExampleUser2,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor few-shot example user 2",
			Description: "Used as the second few-shot user example for the workflow parameter extractor node",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_example_user_2.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor example user 2 template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowParameterExtractorExampleAsst2,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Parameter extractor few-shot example assistant 2",
			Description: "Used as the second few-shot assistant example for the workflow parameter extractor node",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/parameter_extractor_example_assistant_2.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow parameter extractor example assistant 2 template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowLLMChatSystemDefault,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Default workflow LLM chat system prompt",
			Description: "Used by the workflow LLM node to populate the default chat system message",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/llm_chat_system_default.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow LLM chat system default template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowLLMCompletionDefault,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Default workflow LLM completion prompt",
			Description: "Used by the workflow LLM node to populate the default completion prompt while preserving workflow placeholders",
			CreatedAt:   time.Date(2026, time.April, 13, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/llm_completion_default.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow LLM completion default template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowSuggestedQuestionsSystem,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Suggested questions system prompt",
			Description: "Used by the workflow suggested question generator to constrain output format and safety rules",
			CreatedAt:   time.Date(2026, time.May, 16, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/suggested_questions_system.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow suggested questions system prompt template: %v", err))
	}

	err = RegisterTemplate(
		WorkflowSuggestedQuestionsUser,
		TemplateMetadata{
			Module:      "workflow",
			Purpose:     "Suggested questions user prompt",
			Description: "Used by the workflow suggested question generator to render locale, count, and compact workflow context",
			CreatedAt:   time.Date(2026, time.May, 16, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/workflow/suggested_questions_user.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register workflow suggested questions user prompt template: %v", err))
	}

	err = RegisterTemplate(
		AutomationTaskDraftSystem,
		TemplateMetadata{
			Module:      "automation",
			Purpose:     "Generate scheduled task draft system prompt",
			Description: "Used by the scheduled task natural-language draft generator to constrain output format and supported actions",
			CreatedAt:   time.Date(2026, time.May, 17, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/automation/task_draft_system.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register automation task draft system prompt template: %v", err))
	}

	err = RegisterTemplate(
		AutomationTaskDraftUser,
		TemplateMetadata{
			Module:      "automation",
			Purpose:     "Generate scheduled task draft user prompt",
			Description: "Used to render the user request, locale, timezone, and current time for scheduled task draft generation",
			CreatedAt:   time.Date(2026, time.May, 17, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Now(),
		},
		"templates/automation/task_draft_user.tpl",
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register automation task draft user prompt template: %v", err))
	}
}
