package knowledgeretrieval

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

// PlanningStrategy defines the strategy for dataset selection during retrieval
type PlanningStrategy string

const (
	PlanningStrategyRouter      PlanningStrategy = "router"
	PlanningStrategyREACTRouter PlanningStrategy = "react_router"
)

// DocumentHit represents a retrieved document with metadata
type DocumentHit struct {
	Provider    string          `json:"provider"`
	Score       float64         `json:"score"`
	PageContent string          `json:"page_content"`
	Vector      []float64       `json:"vector,omitempty"` // Vector embedding data
	Metadata    map[string]any  `json:"metadata"`
	Children    []ChildDocument `json:"children,omitempty"` // Child document list
}

// ChildDocument represents child chunks within a document hit
type ChildDocument struct {
	PageContent string         `json:"page_content"`
	Vector      []float64      `json:"vector,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

// SingleRetrieveParams groups inputs for single retrieve.
type SingleRetrieveParams struct {
	TenantID           string
	OrganizationID     string
	BillingSubjectType string
	UserID             string
	AppID              string
	UserFrom           string
	Query              string
	ModelConfig        llm.ModelConfig
	Planning           PlanningStrategy
	DatasetIDs         []string
	AvailableDatasets  []*datasetmodel.Dataset
	Tools              []DatasetTool
	MetadataDocIDs     map[string][]string
	MetadataCond       any
}

type DatasetTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// MultipleRetrieveParams groups inputs for multiple retrieve.
type MultipleRetrieveParams struct {
	TenantID           string
	OrganizationID     string
	BillingSubjectType string
	UserID             string
	AppID              string
	UserFrom           string
	Query              string
	AvailableDatasets  []*datasetmodel.Dataset
	DatasetIDs         []string
	TopK               int
	ScoreThreshold     float64
	RerankingMode      rerankingMode // Using correct type
	RerankingModel     map[string]any
	Weights            map[string]any
	RerankingEnable    bool
	MetadataDocIDs     map[string][]string
	MetadataCond       any
}

// Predefined prompt templates for metadata filtering.
// Using variables improves readability and avoids duplication across the package.
const (
	// Chat mode prompts
	metadataFilterSystemPrompt     = "### Job Description\nYou are a text metadata extract engine that extract text's metadata based on user input and set the metadata value\n### Task\nYour task is to ONLY extract the metadatas that exist in the input text from the provided metadata list and Use the following operators [\"contains\", \"not contains\", \"start with\", \"end with\", \"is\", \"is not\", \"empty\", \"not empty\", \"=\", \"≠\", \"\\u003e\", \"\\u003c\", \"≥\", \"≤\", \"before\", \"after\"] to express logical relationships, then return result in JSON format with the key \"metadata_fields\" and value \"metadata_field_value\" and comparison operator \"comparison_operator\".\n### Format\nThe input text is in the variable input_text. Metadata are specified as a list in the variable metadata_fields.\n### Constraint\nDO NOT include anything other than the JSON array in your response."
	metadataFilterUserPrompt1      = "{ \"input_text\": \"I want to know which company’s email address test@example.com is?\",\n\"metadata_fields\": [\"filename\", \"email\", \"phone\", \"address\"]\n}"
	metadataFilterAssistantPrompt1 = "```json\n{\"metadata_map\": [\n    {\"metadata_field_name\": \"email\", \"metadata_field_value\": \"test@example.com\", \"comparison_operator\": \"=\"}\n]\n}\n```"
	metadataFilterUserPrompt2      = "{\"input_text\": \"What are the movies with a score of more than 9 in 2024?\",\n\"metadata_fields\": [\"name\", \"year\", \"rating\", \"country\"]}"
	metadataFilterAssistantPrompt2 = "```json\n{\"metadata_map\": [\n    {\"metadata_field_name\": \"year\", \"metadata_field_value\": \"2024\", \"comparison_operator\": \"=\"},\n    {\"metadata_field_name\": \"rating\", \"metadata_field_value\": \"9\", \"comparison_operator\": \"\\u003e\"}\n]}\n```"
	// metadataFilterUserPrompt3JSONFormat expects JSON string for metadata_fields
	metadataFilterUserPrompt3JSONFormat = "{\"input_text\": \"%s\", \"metadata_fields\": %s}"
	// metadataFilterUserPrompt3SliceFormat is used where a Go slice is formatted directly
	metadataFilterUserPrompt3SliceFormat = "{{\"input_text\": \"%s\", \"metadata_fields\": %v}}"

	// Completion mode prompt template with two placeholders: input_text and metadata_fields (JSON)
	metadataFilterCompletionPromptTemplate = "### Job Description\n" +
		"You are a text metadata extract engine that extract text's metadata based on user input and set the metadata value\n" +
		"### Task\n" +
		"# Your task is to ONLY extract the metadatas that exist in the input text from the provided metadata list and Use the following operators [\"=\", \"!=\", \"\\u003e\", \"\\u003c\", \"\\u003e=\", \"\\u003c=\"] to express logical relationships, then return result in JSON format with the key \"metadata_fields\" and value \"metadata_field_value\" and comparison operator \"comparison_operator\".\n" +
		"### Format\n" +
		"The input text is in the variable input_text. Metadata are specified as a list in the variable metadata_fields.\n" +
		"### Constraint\n" +
		"DO NOT include anything other than the JSON array in your response.\n" +
		"### Example\n" +
		"Here is the chat example between human and assistant, inside <example></example> XML tags.\n" +
		"<example>\n" +
		"User:{{\\\"input_text\\\": [\\\"I want to know which company’s email address test@example.com is?\\\"], \\\"metadata_fields\\\": [\\\"filename\\\", \\\"email\\\", \\\"phone\\\", \\\"address\\\"]}}\n" +
		"Assistant:{{\\\"metadata_map\\\": [{{\\\"metadata_field_name\\\": \\\"email\\\", \\\"metadata_field_value\\\": \\\"test@example.com\\\", \\\"comparison_operator\\\": \\\"=\\\"}}]}}\n" +
		"User:{{\\\"input_text\\\": \\\"What are the movies with a score of more than 9 in 2024?\\\", \\\"metadata_fields\\\": [\\\"name\\\", \\\"year\\\", \\\"rating\\\", \\\"country\\\"]}}\n" +
		"Assistant:{{\\\"metadata_map\\\": [{{\\\"metadata_field_name\\\": \\\"year\\\", \\\"metadata_field_value\\\": \\\"2024\\\", \\\"comparison_operator\\\": \\\"=\\\"}, {{\\\"metadata_field_name\\\": \\\"rating\\\", \\\"metadata_field_value\\\": \\\"9\\\", \\\"comparison_operator\\\": \\\"\\u003e\\\"}}]}}\n" +
		"</example>\n" +
		"### User Input\n" +
		"{\\\"input_text\\\" : \"%s\", \\\"metadata_fields\\\" : %s}\n" +
		"### Assistant Output\n"
)
