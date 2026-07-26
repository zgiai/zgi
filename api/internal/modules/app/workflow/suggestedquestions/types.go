package suggestedquestions

// Question is a generated suggested question candidate.
type Question struct {
	Text   string `json:"text"`
	Reason string `json:"reason,omitempty"`
}

// VariableSummary is the minimal start-node variable metadata needed to
// propose useful first questions without exposing values.
type VariableSummary struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptSummary is a compact representation of an LLM prompt node.
type PromptSummary struct {
	NodeTitle string `json:"node_title,omitempty"`
	Role      string `json:"role,omitempty"`
	Text      string `json:"text,omitempty"`
	Model     string `json:"model,omitempty"`
}

// NoteSummary is a compact representation of note nodes visible on the canvas.
type NoteSummary struct {
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
}

// CapabilitySummary records a workflow capability inferred from node types.
type CapabilitySummary struct {
	Type       string `json:"type,omitempty"`
	Title      string `json:"title,omitempty"`
	Dependency string `json:"dependency,omitempty"`
}

// QueryRole describes how a conversational workflow uses the implicit user
// query. It is intentionally semantic rather than tied to a particular node
// implementation so the generation prompt remains stable as nodes evolve.
type QueryRole string

const (
	QueryRoleUnused           QueryRole = "unused"
	QueryRoleContentInput     QueryRole = "content_input"
	QueryRoleRouteSelector    QueryRole = "route_selector"
	QueryRoleExtractionSource QueryRole = "extraction_source"
	QueryRoleMixed            QueryRole = "mixed"
	QueryRoleUnknown          QueryRole = "unknown"
)

const (
	WarningConversationQueryUnused         = "conversation_query_not_used"
	WarningConversationContextInsufficient = "conversation_generation_context_insufficient"
)

// ConversationRouteSummary describes a reachable user-facing route without
// exposing node IDs or other implementation details.
type ConversationRouteSummary struct {
	Intent       string   `json:"intent,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ConversationSummary contains graph-derived semantics for conversational
// workflows. The implicit query is represented by its role and never as a
// start-node field the user needs to fill in.
type ConversationSummary struct {
	QueryRole       QueryRole                  `json:"query_role"`
	Routes          []ConversationRouteSummary `json:"routes,omitempty"`
	RequiredContext []VariableSummary          `json:"required_context,omitempty"`
}

// WorkflowContext is the product context used for question generation.
type WorkflowContext struct {
	Locale            string               `json:"locale,omitempty"`
	AgentName         string               `json:"agent_name,omitempty"`
	AgentDescription  string               `json:"agent_description,omitempty"`
	WorkflowType      string               `json:"workflow_type,omitempty"`
	OpeningStatement  string               `json:"opening_statement,omitempty"`
	ExistingQuestions []string             `json:"existing_questions,omitempty"`
	StartVariables    []VariableSummary    `json:"start_variables,omitempty"`
	LLMPrompts        []PromptSummary      `json:"llm_prompts,omitempty"`
	Notes             []NoteSummary        `json:"notes,omitempty"`
	Capabilities      []CapabilitySummary  `json:"capabilities,omitempty"`
	Conversation      *ConversationSummary `json:"conversation,omitempty"`

	// AnalysisWarnings are stable product warning codes. They are returned to
	// the editor but are not included in the model prompt.
	AnalysisWarnings []string `json:"-"`
	// SkipGeneration prevents a model call when graph semantics prove that no
	// user query can affect the workflow result.
	SkipGeneration bool `json:"-"`
}

// BuildContextInput contains raw workflow data from either the saved draft or
// the currently edited client graph.
type BuildContextInput struct {
	Locale            string
	AgentName         string
	AgentDescription  string
	WorkflowType      string
	Graph             map[string]interface{}
	Features          map[string]interface{}
	ExistingQuestions []string
}

// GenerateRequest configures one LLM generation request.
type GenerateRequest struct {
	Context        WorkflowContext
	Count          int
	Provider       string
	Model          string
	AgentID        string
	WorkspaceID    string
	OrganizationID string
	AccountID      string
	AppType        string
}

// GenerateResult is returned by Generator.
type GenerateResult struct {
	Questions []Question
	Warnings  []string
	Provider  string
	Model     string
}
