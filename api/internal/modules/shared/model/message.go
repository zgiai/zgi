package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type MessageStatus string

const (
	MessageStatusNormal    MessageStatus = "normal"
	MessageStatusStopped   MessageStatus = "stopped"
	MessageStatusError     MessageStatus = "error"
	MessageStatusCompleted MessageStatus = "completed"
)

const (
	MessageFromUser      MessageFrom = "user"
	MessageFromHuman     MessageFrom = "human"
	MessageFromAssistant MessageFrom = "assistant"
)

type MessageFrom string

// JSONArray Custom JSON array type
type JSONArray []interface{}

// Value Implements driver.Valuer interface
func (j JSONArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// JSONMap Custom JSON object type
type JSONMap map[string]interface{}

// Value Implements driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	// Try to unmarshal as map first
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err == nil {
		*j = JSONMap(result)
		return nil
	}

	// If it's not a map, try to unmarshal as array and convert to map
	var arrayResult []interface{}
	if err := json.Unmarshal(bytes, &arrayResult); err == nil {
		// Convert array to map with "keywords" key
		*j = JSONMap{"keywords": arrayResult}
		return nil
	}

	// If both fail, return the original error
	return json.Unmarshal(bytes, j)
}

type Message struct {
	ID                      string        `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID                   string        `gorm:"type:uuid;not null;index:message_app_idx" json:"app_id"`
	ModelProvider           string        `gorm:"type:varchar(255);not null" json:"model_provider"`
	ModelID                 string        `gorm:"type:varchar(255);not null" json:"model_id"`
	OverrideModelConfigs    *string       `gorm:"type:text" json:"override_model_configs"`
	ConversationID          string        `gorm:"type:uuid;not null;index:message_conversation_idx" json:"conversation_id"`
	Inputs                  JSONMap       `gorm:"type:jsonb" json:"inputs"`
	Query                   string        `gorm:"type:text;not null" json:"query"`
	Message                 JSONArray     `gorm:"type:jsonb;not null" json:"message"`
	MessageTokens           int           `gorm:"type:integer;not null;default:0" json:"message_tokens"`
	MessageUnitPrice        *float64      `gorm:"type:numeric(10,7)" json:"message_unit_price"`
	MessagePriceUnit        *string       `gorm:"type:varchar(255);default:'tokens'" json:"message_price_unit"`
	Answer                  string        `gorm:"type:text;not null" json:"answer"`
	AnswerTokens            int           `gorm:"type:integer;not null;default:0" json:"answer_tokens"`
	AnswerUnitPrice         *float64      `gorm:"type:numeric(10,7)" json:"answer_unit_price"`
	AnswerPriceUnit         *string       `gorm:"type:varchar(255);default:'tokens'" json:"answer_price_unit"`
	ProviderResponseLatency float64       `gorm:"type:numeric(10,4);not null;default:0" json:"provider_response_latency"`
	TotalPrice              *float64      `gorm:"type:numeric(10,7)" json:"total_price"`
	Currency                *string       `gorm:"type:varchar(255)" json:"currency"`
	FromSource              string        `gorm:"type:varchar(255);not null" json:"from_source"`
	FromEndUserID           *string       `gorm:"type:uuid" json:"from_end_user_id"`
	FromAccountID           *string       `gorm:"type:uuid" json:"from_account_id"`
	CreatedAt               time.Time     `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedAt               time.Time     `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`
	AgentBased              bool          `gorm:"not null;default:false" json:"agent_based"`
	WorkflowRunID           *string       `gorm:"type:uuid" json:"workflow_run_id"`
	Status                  MessageStatus `gorm:"type:varchar(255);not null;default:'normal'" json:"status"`
	Error                   *string       `gorm:"type:text" json:"error"`
	MessageMetadata         JSONMap       `gorm:"type:jsonb" json:"message_metadata"`
	InvokeFrom              *string       `gorm:"type:varchar(255)" json:"invoke_from"`
	ParentMessageID         *string       `gorm:"type:uuid" json:"parent_message_id"`

	// Relationships
	Conversation  *Conversation       `gorm:"foreignKey:ConversationID" json:"conversation,omitempty"`
	Feedbacks     []MessageFeedback   `gorm:"foreignKey:MessageID" json:"feedbacks,omitempty"`
	Annotations   []MessageAnnotation `gorm:"foreignKey:MessageID" json:"annotations,omitempty"`
	AgentThoughts []AgentThought      `gorm:"foreignKey:MessageID" json:"agent_thoughts,omitempty"`
	MessageFiles  []MessageFile       `gorm:"foreignKey:MessageID" json:"message_files,omitempty"`
}

// TableName specifies table name
func (Message) TableName() string {
	return "messages"
}

// IsNormal Check if message is normal
func (m *Message) IsNormal() bool {
	return m.Status == MessageStatusNormal
}

// IsError Check if message has an error
func (m *Message) IsError() bool {
	return m.Status == MessageStatusError
}

// GetMessageMetadataDict Get message metadata dictionary
func (m *Message) GetMessageMetadataDict() map[string]interface{} {
	if m.MessageMetadata == nil {
		return make(map[string]interface{})
	}
	return map[string]interface{}(m.MessageMetadata)
}

// MessageFeedback Message feedback model
type MessageFeedback struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID         string    `gorm:"type:uuid;not null" json:"app_id"`
	MessageID     string    `gorm:"type:uuid;not null;index" json:"message_id"`
	Rating        *string   `gorm:"type:varchar(255)" json:"rating"`
	Content       *string   `gorm:"type:text" json:"content"`
	FromSource    string    `gorm:"type:varchar(255);not null" json:"from_source"`
	FromEndUserID *string   `gorm:"type:uuid" json:"from_end_user_id"`
	FromAccountID *string   `gorm:"type:uuid" json:"from_account_id"`
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`

	// Relationships
	Message *Message `gorm:"foreignKey:MessageID" json:"message,omitempty"`
}

// TableName specifies table name
func (MessageFeedback) TableName() string {
	return "message_feedbacks"
}

// MessageAnnotation Message annotation model
type MessageAnnotation struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID     string    `gorm:"type:uuid;not null" json:"app_id"`
	MessageID string    `gorm:"type:uuid;not null;index" json:"message_id"`
	Question  *string   `gorm:"type:text" json:"question"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	AccountID string    `gorm:"type:uuid;not null" json:"account_id"`
	HitCount  int       `gorm:"type:integer;not null;default:0" json:"hit_count"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`

	// Relationships
	Message *Message `gorm:"foreignKey:MessageID" json:"message,omitempty"`
}

// TableName specifies table name
func (MessageAnnotation) TableName() string {
	return "message_annotations"
}

// AgentThought Agent thought model
type AgentThought struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	MessageID   string    `gorm:"type:uuid;not null;index" json:"message_id"`
	ChainID     *string   `gorm:"type:uuid" json:"chain_id"`
	Position    int       `gorm:"type:integer;not null" json:"position"`
	Thought     *string   `gorm:"type:text" json:"thought"`
	Tool        *string   `gorm:"type:text" json:"tool"`
	ToolInput   *string   `gorm:"type:text" json:"tool_input"`
	Observation *string   `gorm:"type:text" json:"observation"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`

	// Relationships
	Message *Message `gorm:"foreignKey:MessageID" json:"message,omitempty"`
}

// TableName specifies table name
func (AgentThought) TableName() string {
	return "agent_thoughts"
}

// MessageFile Message file model
type MessageFile struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	MessageID      string    `gorm:"type:uuid;not null;index" json:"message_id"`
	Type           string    `gorm:"type:varchar(255);not null" json:"type"`
	TransferMethod string    `gorm:"type:varchar(255);not null" json:"transfer_method"`
	URL            *string   `gorm:"type:varchar(255)" json:"url"`
	BelongsTo      *string   `gorm:"type:varchar(255)" json:"belongs_to"`
	UploadFileID   *string   `gorm:"type:uuid" json:"upload_file_id"`
	CreatedBy      *string   `gorm:"type:uuid" json:"created_by"`
	CreatedByRole  string    `gorm:"type:varchar(255);not null" json:"created_by_role"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`

	// Relationships
	Message *Message `gorm:"foreignKey:MessageID" json:"message,omitempty"`
}

// TableName specifies table name
func (MessageFile) TableName() string {
	return "message_files"
}
