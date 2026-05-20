package question_answer

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
)

const (
	AnswerTypeText   = "text"
	AnswerTypeChoice = "choice"

	ChoiceModeStatic  = "static"
	ChoiceModeDynamic = "dynamic"

	DynamicChoiceHandle = "dynamicOption"

	ExtractionFieldTypeString  = "string"
	ExtractionFieldTypeNumber  = "number"
	ExtractionFieldTypeBoolean = "boolean"
)

type Node struct {
	base.NodeStruct
	NodeData  NodeData
	llmClient llmclient.LLMClient
}

type NodeData struct {
	base.NodeData
	Question              string            `json:"question"`
	AnswerType            string            `json:"answer_type"`
	CompletionInstruction string            `json:"completion_instruction"`
	ExtractFromAnswer     bool              `json:"extract_from_answer"`
	ExtractionInstruction string            `json:"extraction_instruction"`
	ExtractionFields      []ExtractionField `json:"extraction_fields"`
	MaxAnswerCount        int               `json:"max_answer_count"`
	Model                 ModelConfig       `json:"model"`
	ModelConfig           ModelConfig       `json:"model_config"`
	Choices               []Choice          `json:"choices"`
	DynamicChoices        Selector          `json:"dynamic_choices"`
	ChoiceMode            string            `json:"choice_mode"`
}

type ModelConfig struct {
	Provider         string         `json:"provider"`
	Name             string         `json:"name"`
	Model            string         `json:"model"`
	CompletionParams map[string]any `json:"completion_params"`
}

type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type ExtractionField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type Selector struct {
	Selector []string `json:"selector"`
}

type AnswerRound struct {
	Round    int    `json:"round"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type extractionDecision struct {
	Fields           map[string]any `json:"fields"`
	MissingFields    []string       `json:"missing_fields"`
	FollowUpQuestion string         `json:"follow_up_question"`
	Reason           string         `json:"reason"`
}
