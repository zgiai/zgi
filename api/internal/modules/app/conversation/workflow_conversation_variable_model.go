package conversation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// WorkflowConversationVariable represents the workflow_conversation_variables table
type WorkflowConversationVariable struct {
	ID             uuid.UUID `gorm:"type:uuid;not null;primaryKey" json:"id"`
	ConversationID uuid.UUID `gorm:"type:uuid;not null;primaryKey;index:workflow_conversation_variables_conversation_id_idx" json:"conversation_id"`
	AppID          uuid.UUID `gorm:"type:uuid;not null;index:workflow_conversation_variables_app_id_idx" json:"app_id"`
	Data           string    `gorm:"type:text;not null" json:"data"` // JSON string containing variable data
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index:workflow_conversation_variables_created_at_idx" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Conversation *AgentConversation `gorm:"foreignKey:ConversationID;references:ID" json:"conversation,omitempty"`
}

// TableName specifies the table name for WorkflowConversationVariable
func (WorkflowConversationVariable) TableName() string {
	return "workflow_conversation_variables"
}

// VariableData represents the structure of variable data stored in JSON format
type VariableData struct {
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	ValueType   string      `json:"value_type"`
	Description string      `json:"description"`
}

// GetDataAsVariableData parses the JSON data string into VariableData
func (wcv *WorkflowConversationVariable) GetDataAsVariableData() (*VariableData, error) {
	var data VariableData
	if wcv.Data == "" {
		return &VariableData{}, nil
	}
	
	err := json.Unmarshal([]byte(wcv.Data), &data)
	if err != nil {
		return nil, err
	}
	
	return &data, nil
}

// SetDataFromVariableData converts VariableData to JSON string for storage
func (wcv *WorkflowConversationVariable) SetDataFromVariableData(data *VariableData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	
	wcv.Data = string(jsonData)
	wcv.UpdatedAt = time.Now()
	return nil
}

// GetDataAsMap parses the JSON data string into a map
func (wcv *WorkflowConversationVariable) GetDataAsMap() (map[string]interface{}, error) {
	var data map[string]interface{}
	if wcv.Data == "" {
		return make(map[string]interface{}), nil
	}
	
	err := json.Unmarshal([]byte(wcv.Data), &data)
	if err != nil {
		return nil, err
	}
	
	return data, nil
}

// SetDataFromMap converts a map to JSON string for storage
func (wcv *WorkflowConversationVariable) SetDataFromMap(data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	
	wcv.Data = string(jsonData)
	wcv.UpdatedAt = time.Now()
	return nil
}

// CreateFromVariable creates a WorkflowConversationVariable from variable data
func CreateWorkflowConversationVariable(conversationID, appID uuid.UUID, name string, value interface{}, valueType, description string) *WorkflowConversationVariable {
	variableData := &VariableData{
		Name:        name,
		Value:       value,
		ValueType:   valueType,
		Description: description,
	}
	
	wcv := &WorkflowConversationVariable{
		ID:             uuid.New(),
		ConversationID: conversationID,
		AppID:          appID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	wcv.SetDataFromVariableData(variableData)
	return wcv
}

// UpdateVariable updates the variable data
func (wcv *WorkflowConversationVariable) UpdateVariable(name string, value interface{}, valueType, description string) error {
	variableData := &VariableData{
		Name:        name,
		Value:       value,
		ValueType:   valueType,
		Description: description,
	}
	
	return wcv.SetDataFromVariableData(variableData)
}