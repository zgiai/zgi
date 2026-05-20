package dto

import (
	"encoding/json"
)

type ModelConfigRequest struct {
	// Model configuration
	Provider string                 `json:"provider" binding:"required"`
	ModelID  string                 `json:"model_id" binding:"required"`
	Configs  map[string]interface{} `json:"configs"`

	// Prompt configuration
	PromptType             string                 `json:"prompt_type"`
	ChatPromptConfig       map[string]interface{} `json:"chat_prompt_config"`
	CompletionPromptConfig map[string]interface{} `json:"completion_prompt_config"`

	// Basic settings
	OpeningStatement              string   `json:"opening_statement"`
	SuggestedQuestions            []string `json:"suggested_questions"`
	SuggestedQuestionsAfterAnswer *string  `json:"suggested_questions_after_answer"`

	// User input form
	UserInputForm []map[string]interface{} `json:"user_input_form"`

	// Dataset configuration
	DatasetConfigs       map[string]interface{} `json:"dataset_configs"`
	DatasetQueryVariable string                 `json:"dataset_query_variable"`
	RetrieverResource    map[string]interface{} `json:"retriever_resource"`

	// Agent configuration
	AgentMode map[string]interface{} `json:"agent_mode"`

	// External data tools
	ExternalDataTools []map[string]interface{} `json:"external_data_tools"`

	// File upload
	FileUpload map[string]interface{} `json:"file_upload"`

	// Text to speech
	TextToSpeech map[string]interface{} `json:"text_to_speech"`

	// Speech to text
	SpeechToText map[string]interface{} `json:"speech_to_text"`

	// Sensitive word avoidance
	SensitiveWordAvoidance map[string]interface{} `json:"sensitive_word_avoidance"`

	// More like this
	MoreLikeThis map[string]interface{} `json:"more_like_this"`
}

type AgentTool struct {
	ProviderID     string                 `json:"provider_id"`
	ProviderType   string                 `json:"provider_type"`
	ToolName       string                 `json:"tool_name"`
	ToolParameters map[string]interface{} `json:"tool_parameters"`
}

type ModelConfigResponse struct {
	Result string `json:"result"`
}

type ModelConfigValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type AppModelConfigDTO struct {
	ID                            string `json:"id"`
	AppID                         string `json:"app_id"`
	Provider                      string `json:"provider"`
	ModelID                       string `json:"model_id"`
	Configs                       string `json:"configs"`
	OpeningStatement              string `json:"opening_statement"`
	SuggestedQuestions            string `json:"suggested_questions"`
	SuggestedQuestionsAfterAnswer string `json:"suggested_questions_after_answer"`
	MoreLikeThis                  string `json:"more_like_this"`
	Model                         string `json:"model"`
	UserInputForm                 string `json:"user_input_form"`
	PrePrompt                     string `json:"pre_prompt"`
	AgentMode                     string `json:"agent_mode"`
	SpeechToText                  string `json:"speech_to_text"`
	SensitiveWordAvoidance        string `json:"sensitive_word_avoidance"`
	RetrieverResource             string `json:"retriever_resource"`
	DatasetQueryVariable          string `json:"dataset_query_variable"`
	PromptType                    string `json:"prompt_type"`
	ChatPromptConfig              string `json:"chat_prompt_config"`
	CompletionPromptConfig        string `json:"completion_prompt_config"`
	DatasetConfigs                string `json:"dataset_configs"`
	ExternalDataTools             string `json:"external_data_tools"`
	FileUpload                    string `json:"file_upload"`
	TextToSpeech                  string `json:"text_to_speech"`
	CreatedBy                     string `json:"created_by"`
	UpdatedBy                     string `json:"updated_by"`
	CreatedAt                     string `json:"created_at"`
	UpdatedAt                     string `json:"updated_at"`
}

func ToJSON(data interface{}) string {
	if data == nil {
		return ""
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	return string(bytes)
}

func FromJSON(jsonStr string) map[string]interface{} {
	if jsonStr == "" {
		return nil
	}

	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return nil
	}

	return result
}
