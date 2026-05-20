package dto

import (
	"github.com/google/uuid"
)

type CompletionMessageRequest struct {
	Inputs        map[string]interface{} `json:"inputs" binding:"required"`
	Query         string                 `json:"query"`
	Files         []interface{}          `json:"files,omitempty"`
	ModelConfig   map[string]interface{} `json:"model_config" binding:"required"`
	ResponseMode  string                 `json:"response_mode" binding:"omitempty,oneof=blocking streaming"`
	RetrieverFrom string                 `json:"retriever_from"`
}

type ChatMessageRequest struct {
	Inputs          map[string]interface{} `json:"inputs" binding:"required"`
	Query           string                 `json:"query" binding:"required"`
	Files           []interface{}          `json:"files,omitempty"`
	ModelConfig     map[string]interface{} `json:"model_config" binding:"required"`
	ConversationID  *uuid.UUID             `json:"conversation_id,omitempty"`
	ParentMessageID *uuid.UUID             `json:"parent_message_id,omitempty"`
	ResponseMode    string                 `json:"response_mode" binding:"omitempty,oneof=blocking streaming"`
	RetrieverFrom   string                 `json:"retriever_from"`
}

type MessageStopRequest struct {
	TaskID string `uri:"task_id" binding:"required"`
}

type MessageStopResponse struct {
	Result string `json:"result"`
}

type CompletionResponse struct {
	Event          string                 `json:"event,omitempty"`
	TaskID         string                 `json:"task_id,omitempty"`
	ID             string                 `json:"id,omitempty"`
	MessageID      string                 `json:"message_id,omitempty"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Answer         string                 `json:"answer,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      int64                  `json:"created_at,omitempty"`
}

type GenerateArgs struct {
	Inputs           map[string]interface{} `json:"inputs"`
	Query            string                 `json:"query"`
	Files            []interface{}          `json:"files"`
	ModelConfig      map[string]interface{} `json:"model_config"`
	ConversationID   *uuid.UUID             `json:"conversation_id,omitempty"`
	ParentMessageID  *uuid.UUID             `json:"parent_message_id,omitempty"`
	ResponseMode     string                 `json:"response_mode"`
	RetrieverFrom    string                 `json:"retriever_from"`
	AutoGenerateName bool                   `json:"auto_generate_name"`
}

func (r *CompletionMessageRequest) ConvertToGenerateArgs() *GenerateArgs {
	responseMode := "blocking"
	if r.ResponseMode != "" {
		responseMode = r.ResponseMode
	}

	retrieverFrom := "dev"
	if r.RetrieverFrom != "" {
		retrieverFrom = r.RetrieverFrom
	}

	return &GenerateArgs{
		Inputs:           r.Inputs,
		Query:            r.Query,
		Files:            r.Files,
		ModelConfig:      r.ModelConfig,
		ResponseMode:     responseMode,
		RetrieverFrom:    retrieverFrom,
		AutoGenerateName: false,
	}
}

func (r *ChatMessageRequest) ConvertToGenerateArgs() *GenerateArgs {
	responseMode := "blocking"
	if r.ResponseMode != "" {
		responseMode = r.ResponseMode
	}

	retrieverFrom := "dev"
	if r.RetrieverFrom != "" {
		retrieverFrom = r.RetrieverFrom
	}

	return &GenerateArgs{
		Inputs:           r.Inputs,
		Query:            r.Query,
		Files:            r.Files,
		ModelConfig:      r.ModelConfig,
		ConversationID:   r.ConversationID,
		ParentMessageID:  r.ParentMessageID,
		ResponseMode:     responseMode,
		RetrieverFrom:    retrieverFrom,
		AutoGenerateName: false,
	}
}
