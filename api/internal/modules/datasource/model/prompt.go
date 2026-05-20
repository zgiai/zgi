package model

import (
	"time"
)

// TablePrompt represents a prompt associated with a table
type TablePrompt struct {
	ID        string    `json:"id"`
	TableID   string    `json:"table_id"`
	Prompt    string    `json:"prompt"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies table name
func (TablePrompt) TableName() string {
	return "data_source_table_prompts"
}

// CreateTablePromptRequest is the request for creating a new table prompt
type CreateTablePromptRequest struct {
	TableID string `json:"table_id"`
	Prompt  string `json:"prompt"`
}

// UpdateTablePromptRequest is the request for updating a table prompt
type UpdateTablePromptRequest struct {
	Prompt    string `json:"prompt"`
	UpdatedBy string `json:"updated_by"`
}