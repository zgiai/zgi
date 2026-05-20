package conversation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/pkg/logger"
)

// WorkflowConversationVariableService defines the interface for workflow conversation variable business logic
type WorkflowConversationVariableService interface {
	CreateVariable(ctx context.Context, req *CreateVariableRequest) (*WorkflowConversationVariable, error)
	GetVariable(ctx context.Context, id uuid.UUID) (*WorkflowConversationVariable, error)
	GetVariablesByConversation(ctx context.Context, conversationID uuid.UUID) ([]*WorkflowConversationVariable, error)
	GetVariableByName(ctx context.Context, conversationID uuid.UUID, name string) (*WorkflowConversationVariable, error)
	UpdateVariable(ctx context.Context, variable *WorkflowConversationVariable) error
	DeleteVariable(ctx context.Context, id uuid.UUID) error
	DeleteVariablesByConversation(ctx context.Context, conversationID uuid.UUID) error
	UpsertVariable(ctx context.Context, req *UpsertVariableRequest) (*WorkflowConversationVariable, error)
	GetVariablesByApp(ctx context.Context, appID uuid.UUID, limit, offset int) ([]*WorkflowConversationVariable, int64, error)
	LoadConversationVariables(ctx context.Context, conversationID uuid.UUID) (map[string]interface{}, error)
	SaveConversationVariables(ctx context.Context, conversationID, appID uuid.UUID, variables map[string]interface{}) error
}

// CreateVariableRequest represents the request to create a variable
type CreateVariableRequest struct {
	ConversationID uuid.UUID   `json:"conversation_id"`
	AppID          uuid.UUID   `json:"app_id"`
	Name           string      `json:"name"`
	Value          interface{} `json:"value"`
	ValueType      string      `json:"value_type"`
	Description    string      `json:"description"`
}

// UpsertVariableRequest represents the request to upsert a variable
type UpsertVariableRequest struct {
	ConversationID uuid.UUID   `json:"conversation_id"`
	AppID          uuid.UUID   `json:"app_id"`
	Name           string      `json:"name"`
	Value          interface{} `json:"value"`
	ValueType      string      `json:"value_type"`
	Description    string      `json:"description"`
}

// workflowConversationVariableService implements WorkflowConversationVariableService
type workflowConversationVariableService struct {
	variableRepo WorkflowConversationVariableRepository
}

// NewWorkflowConversationVariableService creates a new WorkflowConversationVariableService
func NewWorkflowConversationVariableService(
	variableRepo WorkflowConversationVariableRepository,
) WorkflowConversationVariableService {
	return &workflowConversationVariableService{
		variableRepo: variableRepo,
	}
}

// CreateVariable creates a new workflow conversation variable
func (s *workflowConversationVariableService) CreateVariable(ctx context.Context, req *CreateVariableRequest) (*WorkflowConversationVariable, error) {
	variable := CreateWorkflowConversationVariable(
		req.ConversationID,
		req.AppID,
		req.Name,
		req.Value,
		req.ValueType,
		req.Description,
	)

	err := s.variableRepo.Create(ctx, variable)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow conversation variable: %w", err)
	}

	return variable, nil
}

// GetVariable retrieves a workflow conversation variable by ID
func (s *workflowConversationVariableService) GetVariable(ctx context.Context, id uuid.UUID) (*WorkflowConversationVariable, error) {
	return s.variableRepo.GetByID(ctx, id)
}

// GetVariablesByConversation retrieves all variables for a conversation
func (s *workflowConversationVariableService) GetVariablesByConversation(ctx context.Context, conversationID uuid.UUID) ([]*WorkflowConversationVariable, error) {
	return s.variableRepo.GetByConversationID(ctx, conversationID)
}

// GetVariableByName retrieves a specific variable by conversation ID and name
func (s *workflowConversationVariableService) GetVariableByName(ctx context.Context, conversationID uuid.UUID, name string) (*WorkflowConversationVariable, error) {
	return s.variableRepo.GetByConversationAndName(ctx, conversationID, name)
}

// UpdateVariable updates a workflow conversation variable
func (s *workflowConversationVariableService) UpdateVariable(ctx context.Context, variable *WorkflowConversationVariable) error {
	return s.variableRepo.Update(ctx, variable)
}

// DeleteVariable deletes a workflow conversation variable
func (s *workflowConversationVariableService) DeleteVariable(ctx context.Context, id uuid.UUID) error {
	return s.variableRepo.Delete(ctx, id)
}

// DeleteVariablesByConversation deletes all variables for a conversation
func (s *workflowConversationVariableService) DeleteVariablesByConversation(ctx context.Context, conversationID uuid.UUID) error {
	return s.variableRepo.DeleteByConversationID(ctx, conversationID)
}

// UpsertVariable creates or updates a variable for a conversation
func (s *workflowConversationVariableService) UpsertVariable(ctx context.Context, req *UpsertVariableRequest) (*WorkflowConversationVariable, error) {
	err := s.variableRepo.UpsertVariable(
		ctx,
		req.ConversationID,
		req.AppID,
		req.Name,
		req.Value,
		req.ValueType,
		req.Description,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert workflow conversation variable: %w", err)
	}

	// Return the updated/created variable
	return s.GetVariableByName(ctx, req.ConversationID, req.Name)
}

// GetVariablesByApp retrieves variables by app ID with pagination
func (s *workflowConversationVariableService) GetVariablesByApp(ctx context.Context, appID uuid.UUID, limit, offset int) ([]*WorkflowConversationVariable, int64, error) {
	return s.variableRepo.GetVariablesByApp(ctx, appID, limit, offset)
}

// LoadConversationVariables loads all variables for a conversation as a map
func (s *workflowConversationVariableService) LoadConversationVariables(ctx context.Context, conversationID uuid.UUID) (map[string]interface{}, error) {
	variables, err := s.variableRepo.GetByConversationID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to load conversation variables: %w", err)
	}

	result := make(map[string]interface{})
	for _, variable := range variables {
		data, err := variable.GetDataAsVariableData()
		if err != nil {
			// Log error but continue with other variables
			logger.WarnContext(ctx, "failed to parse workflow conversation variable data", "variable_id", variable.ID.String(), err)
			continue
		}
		result[data.Name] = data.Value
	}

	return result, nil
}

// SaveConversationVariables saves multiple variables for a conversation
func (s *workflowConversationVariableService) SaveConversationVariables(ctx context.Context, conversationID, appID uuid.UUID, variables map[string]interface{}) error {
	for name, value := range variables {
		// Determine value type
		valueType := determineValueType(value)

		req := &UpsertVariableRequest{
			ConversationID: conversationID,
			AppID:          appID,
			Name:           name,
			Value:          value,
			ValueType:      valueType,
			Description:    fmt.Sprintf("Variable %s for conversation %s", name, conversationID),
		}

		_, err := s.UpsertVariable(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to save variable %s: %w", name, err)
		}
	}

	return nil
}

// determineValueType determines the type of a value
func determineValueType(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case int, int8, int16, int32, int64:
		return "integer"
	case float32, float64:
		return "float"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "string" // Default to string
	}
}
