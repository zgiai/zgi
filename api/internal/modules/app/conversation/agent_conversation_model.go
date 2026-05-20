package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentConversation represents the agents_conversations table
type AgentConversation struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AgentID                 uuid.UUID  `gorm:"type:uuid;not null;index:conversation_from_user_idx" json:"agent_id"`
	AgentConfigID           *uuid.UUID `gorm:"type:uuid" json:"agent_config_id"`
	ModelProvider           *string    `gorm:"type:varchar(255)" json:"model_provider"`
	OverrideModelConfigs    *string    `gorm:"type:text" json:"override_model_configs"`
	ModelVersionID          *string    `gorm:"type:varchar(255)" json:"model_version_id"`
	Mode                    string     `gorm:"type:varchar(255);not null" json:"mode"`
	Name                    string     `gorm:"type:varchar(255);not null" json:"name"`
	Summary                 *string    `gorm:"type:text" json:"summary"`
	Inputs                  string     `gorm:"type:json;not null" json:"inputs"` // JSON string
	Introduction            *string    `gorm:"type:text" json:"introduction"`
	SystemInstruction       *string    `gorm:"type:text" json:"system_instruction"`
	SystemInstructionTokens int        `gorm:"type:integer;not null;default:0" json:"system_instruction_tokens"`
	Status                  string     `gorm:"type:varchar(255);not null" json:"status"`
	InvokeFrom              *string    `gorm:"type:varchar(255)" json:"invoke_from"`
	WebAppID                *string    `gorm:"type:uuid" json:"web_app_id"` // Track which web application was used
	FromSource              string     `gorm:"type:varchar(255);not null;index:conversation_from_user_idx" json:"from_source"`
	FromEndUserID           *uuid.UUID `gorm:"type:uuid;index:conversation_from_user_idx" json:"from_end_user_id"`
	FromAccountID           *uuid.UUID `gorm:"type:uuid" json:"from_account_id"`
	ReadAt                  *time.Time `json:"read_at"`
	ReadAccountID           *uuid.UUID `gorm:"type:uuid" json:"read_account_id"`
	DialogueCount           int        `gorm:"type:integer;not null;default:0" json:"dialogue_count"`
	CreatedBy               *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedBy               *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	UpdatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`
	DeletedBy               *uuid.UUID `gorm:"type:uuid" json:"deleted_by"`
	DeletedAt               *time.Time `json:"deleted_at"`

	// Relationships
	Messages []AgentMessage `gorm:"foreignKey:ConversationID;references:ID" json:"messages,omitempty"`
}

// TableName specifies the table name for AgentConversation
func (AgentConversation) TableName() string {
	return "agents_conversations"
}

// IsDeleted checks if the conversation is soft deleted
func (ac *AgentConversation) IsDeleted() bool {
	return ac.DeletedAt != nil
}

// GetInputsAsMap parses the JSON inputs string into a map
func (ac *AgentConversation) GetInputsAsMap() (map[string]interface{}, error) {
	var inputs map[string]interface{}
	if ac.Inputs == "" {
		return make(map[string]interface{}), nil
	}

	err := json.Unmarshal([]byte(ac.Inputs), &inputs)
	if err != nil {
		return nil, err
	}
	return inputs, nil
}

// SetInputsFromMap converts a map to JSON string for storage
func (ac *AgentConversation) SetInputsFromMap(inputs map[string]interface{}) error {
	jsonBytes, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	ac.Inputs = string(jsonBytes)
	return nil
}

// IncrementDialogueCount increments the dialogue count
func (ac *AgentConversation) IncrementDialogueCount() {
	ac.DialogueCount++
	ac.UpdatedAt = time.Now()
}

// GetSummaryOrName returns summary if available, otherwise returns name
func (ac *AgentConversation) GetSummaryOrName() string {
	if ac.Summary != nil && *ac.Summary != "" {
		return *ac.Summary
	}
	return ac.Name
}
