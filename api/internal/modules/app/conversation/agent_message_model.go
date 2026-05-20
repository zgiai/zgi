package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	AgentMessageStatusNormal          = "normal"
	AgentMessageStatusRunning         = "running"
	AgentMessageStatusPendingApproval = "pending_approval"
	AgentMessageStatusPendingQuestion = "pending_question"
	AgentMessageStatusCompleted       = "completed"
	AgentMessageStatusError           = "error"
	AgentMessageStatusStopped         = "stopped"
	AgentMessageStatusExpired         = "expired"
)

// AgentMessage represents the agents_messages table
type AgentMessage struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AgentID                 uuid.UUID  `gorm:"type:uuid;not null;index:message_agents_id_idx" json:"agent_id"`
	ModelProvider           *string    `gorm:"type:varchar(255)" json:"model_provider"`
	ModelVersionID          *string    `gorm:"type:varchar(255)" json:"model_version_id"`
	OverrideModelConfigs    *string    `gorm:"type:text" json:"override_model_configs"`
	ConversationID          uuid.UUID  `gorm:"type:uuid;not null;index:agents_message_conversation_id_idx" json:"conversation_id"`
	Inputs                  string     `gorm:"type:json;not null" json:"inputs"` // JSON string
	Query                   string     `gorm:"type:text;not null" json:"query"`
	Message                 string     `gorm:"type:json;not null" json:"message"` // JSON string for structured messages
	MessageTokens           int        `gorm:"type:integer;not null;default:0" json:"message_tokens"`
	MessageUnitPrice        *float64   `gorm:"type:numeric(10,4)" json:"message_unit_price"`
	MessagePriceUnit        *float64   `gorm:"type:numeric(10,7);default:0.001" json:"message_price_unit"`
	Answer                  string     `gorm:"type:text;not null" json:"answer"`
	AnswerTokens            int        `gorm:"type:integer;not null;default:0" json:"answer_tokens"`
	AnswerUnitPrice         *float64   `gorm:"type:numeric(10,4)" json:"answer_unit_price"`
	AnswerPriceUnit         *float64   `gorm:"type:numeric(10,7);default:0.001" json:"answer_price_unit"`
	ParentMessageID         *uuid.UUID `gorm:"type:uuid" json:"parent_message_id"`
	ProviderResponseLatency float64    `gorm:"type:double precision;not null;default:0" json:"provider_response_latency"`
	TotalPrice              *float64   `gorm:"type:numeric(10,7)" json:"total_price"`
	Currency                string     `gorm:"type:varchar(255);not null" json:"currency"`
	Status                  string     `gorm:"type:varchar(255);not null;default:'normal'" json:"status"`
	Error                   *string    `gorm:"type:text" json:"error"`
	MessageMetadata         *string    `gorm:"type:text" json:"message_metadata"` // JSON string
	InvokeFrom              *string    `gorm:"type:varchar(255)" json:"invoke_from"`
	FromSource              string     `gorm:"type:varchar(255);not null;index:agents_message_end_user_idx,index:agents_message_account_idx" json:"from_source"`
	FromEndUserID           *uuid.UUID `gorm:"type:uuid;index:agents_message_end_user_idx" json:"from_end_user_id"`
	FromAccountID           *uuid.UUID `gorm:"type:uuid;index:agents_message_account_idx" json:"from_account_id"`
	CreatedBy               *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0);index:agents_message_created_at_idx" json:"created_at"`
	UpdatedBy               *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	UpdatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`
	DeletedBy               *uuid.UUID `gorm:"type:uuid" json:"deleted_by"`
	DeletedAt               *time.Time `json:"deleted_at"`
	AgentBased              bool       `gorm:"not null;default:false" json:"agent_based"`
	WorkflowRunID           *uuid.UUID `gorm:"type:uuid;index:agents_message_workflow_run_id_idx" json:"workflow_run_id"`
	WebAppID                *string    `gorm:"type:uuid" json:"web_app_id"` // Track which web application was used

	// Relationships
	Conversation *AgentConversation `gorm:"foreignKey:ConversationID;references:ID" json:"conversation,omitempty"`
}

// TableName specifies the table name for AgentMessage
func (AgentMessage) TableName() string {
	return "agents_messages"
}

// IsDeleted checks if the message is soft deleted
func (am *AgentMessage) IsDeleted() bool {
	return am.DeletedAt != nil
}

// IsNormal checks if the message status is normal
func (am *AgentMessage) IsNormal() bool {
	return am.Status == AgentMessageStatusNormal
}

// IsError checks if the message status is error
func (am *AgentMessage) IsError() bool {
	return am.Status == AgentMessageStatusError
}

// GetInputsAsMap parses the JSON inputs string into a map
func (am *AgentMessage) GetInputsAsMap() (map[string]interface{}, error) {
	var inputs map[string]interface{}
	if am.Inputs == "" {
		return make(map[string]interface{}), nil
	}

	err := json.Unmarshal([]byte(am.Inputs), &inputs)
	if err != nil {
		return nil, err
	}
	return inputs, nil
}

// SetInputsFromMap converts a map to JSON string for storage
func (am *AgentMessage) SetInputsFromMap(inputs map[string]interface{}) error {
	jsonBytes, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	am.Inputs = string(jsonBytes)
	return nil
}

// GetMessageAsArray parses the JSON message string into an array
func (am *AgentMessage) GetMessageAsArray() ([]interface{}, error) {
	var messages []interface{}
	if am.Message == "" {
		return make([]interface{}, 0), nil
	}

	err := json.Unmarshal([]byte(am.Message), &messages)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// SetMessageFromArray converts an array to JSON string for storage
func (am *AgentMessage) SetMessageFromArray(messages []interface{}) error {
	jsonBytes, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	am.Message = string(jsonBytes)
	return nil
}

// GetMessageMetadataAsMap parses the JSON metadata string into a map
func (am *AgentMessage) GetMessageMetadataAsMap() (map[string]interface{}, error) {
	var metadata map[string]interface{}
	if am.MessageMetadata == nil || *am.MessageMetadata == "" {
		return make(map[string]interface{}), nil
	}

	err := json.Unmarshal([]byte(*am.MessageMetadata), &metadata)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

// CalculateTotalPrice calculates the total price based on input and output tokens
func (am *AgentMessage) CalculateTotalPrice() {
	var total float64 = 0

	if am.MessageUnitPrice != nil && am.MessagePriceUnit != nil {
		total += float64(am.MessageTokens) * (*am.MessageUnitPrice) * (*am.MessagePriceUnit)
	}

	if am.AnswerUnitPrice != nil && am.AnswerPriceUnit != nil {
		total += float64(am.AnswerTokens) * (*am.AnswerUnitPrice) * (*am.AnswerPriceUnit)
	}

	am.TotalPrice = &total
}
