package prompt

import (
	"bytes"
	"text/template"
	"time"
)

// TemplateID uses module/purpose hierarchy to clearly identify the source and purpose of prompts.
type TemplateID string

const (
	// Datasource module specific prompts.
	DatasourceTableAnalysis       TemplateID = "datasource/table_analysis"
	DatasourceFileConversion      TemplateID = "datasource/file_conversion"
	DatasourceDefaultUserIngestZh TemplateID = "datasource/default_user_ingest_zh-CN"
	DatasourceDefaultUserIngestEn TemplateID = "datasource/default_user_ingest"

	// Dataset module specific prompts.
	DatasetQuestionGeneration TemplateID = "dataset/question_generation"

	// Common prompts.
	CommonConversationTitle TemplateID = "common/conversation_title"

	// AIChat prompts.
	AIChatSystem TemplateID = "aichat/chat_system"

	// GraphFlow prompts.
	GraphFlowQueryEntityExtraction TemplateID = "graphflow/query_entity_extraction"
	GraphFlowGlobalEntitySummary   TemplateID = "graphflow/global_entity_summary"
	GraphFlowGraphExtraction       TemplateID = "graphflow/graph_extraction"
	GraphFlowOpenIEExtraction      TemplateID = "graphflow/openie_extraction"

	// Workflow prompts.
	WorkflowSQLGeneratorSystem             TemplateID = "workflow/sql_generator_system"
	WorkflowParameterExtractorSystem       TemplateID = "workflow/parameter_extractor_system"
	WorkflowParameterExtractorChatUser     TemplateID = "workflow/parameter_extractor_chat_user"
	WorkflowParameterExtractorCompletion   TemplateID = "workflow/parameter_extractor_completion"
	WorkflowParameterExtractorExampleUser1 TemplateID = "workflow/parameter_extractor_example_user_1"
	WorkflowParameterExtractorExampleAsst1 TemplateID = "workflow/parameter_extractor_example_assistant_1"
	WorkflowParameterExtractorExampleUser2 TemplateID = "workflow/parameter_extractor_example_user_2"
	WorkflowParameterExtractorExampleAsst2 TemplateID = "workflow/parameter_extractor_example_assistant_2"
	WorkflowLLMChatSystemDefault           TemplateID = "workflow/llm_chat_system_default"
	WorkflowLLMCompletionDefault           TemplateID = "workflow/llm_completion_default"
	WorkflowSuggestedQuestionsSystem       TemplateID = "workflow/suggested_questions_system"
	WorkflowSuggestedQuestionsUser         TemplateID = "workflow/suggested_questions_user"

	// Automation prompts.
	AutomationTaskDraftSystem TemplateID = "automation/task_draft_system"
	AutomationTaskDraftUser   TemplateID = "automation/task_draft_user"

	// Workflow module specific prompts
	WorkflowDiagnosisSystem TemplateID = "workflow/diagnosis_system"
	WorkflowDiagnosisUserZh TemplateID = "workflow/diagnosis_user_zh-CN"
	WorkflowDiagnosisUserEn TemplateID = "workflow/diagnosis_user"
)

// TemplateMetadata contains metadata information for prompts to help maintainers understand context.
type TemplateMetadata struct {
	Module      string
	Purpose     string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Template contains prompt content and metadata.
type Template struct {
	ID         TemplateID
	Metadata   TemplateMetadata
	rawContent string
	goTemplate *template.Template
}

// Render renders the template using Go text/template semantics.
func (t *Template) Render(data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := t.goTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RawContent returns the original template content.
func (t *Template) RawContent() string {
	return t.rawContent
}
