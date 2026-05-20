package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkflowConversationVariableRepository defines the interface for workflow conversation variable data access
type WorkflowConversationVariableRepository interface {
	Create(ctx context.Context, variable *WorkflowConversationVariable) error
	GetByID(ctx context.Context, id uuid.UUID) (*WorkflowConversationVariable, error)
	GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*WorkflowConversationVariable, error)
	GetByConversationAndName(ctx context.Context, conversationID uuid.UUID, name string) (*WorkflowConversationVariable, error)
	Update(ctx context.Context, variable *WorkflowConversationVariable) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByConversationID(ctx context.Context, conversationID uuid.UUID) error
	UpsertVariable(ctx context.Context, conversationID, appID uuid.UUID, name string, value interface{}, valueType, description string) error
	GetVariablesByApp(ctx context.Context, appID uuid.UUID, limit, offset int) ([]*WorkflowConversationVariable, int64, error)
}

// workflowConversationVariableRepository implements WorkflowConversationVariableRepository
type workflowConversationVariableRepository struct {
	db *gorm.DB
}

// NewWorkflowConversationVariableRepository creates a new WorkflowConversationVariableRepository
func NewWorkflowConversationVariableRepository(db *gorm.DB) WorkflowConversationVariableRepository {
	return &workflowConversationVariableRepository{
		db: db,
	}
}

// Create creates a new workflow conversation variable
func (r *workflowConversationVariableRepository) Create(ctx context.Context, variable *WorkflowConversationVariable) error {
	if variable.ID == uuid.Nil {
		variable.ID = uuid.New()
	}
	variable.CreatedAt = time.Now()
	variable.UpdatedAt = time.Now()

	if err := r.db.WithContext(ctx).Create(variable).Error; err != nil {
		return fmt.Errorf("failed to create workflow conversation variable: %w", err)
	}
	return nil
}

// GetByID retrieves a workflow conversation variable by ID
func (r *workflowConversationVariableRepository) GetByID(ctx context.Context, id uuid.UUID) (*WorkflowConversationVariable, error) {
	var variable WorkflowConversationVariable
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&variable).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow conversation variable not found")
		}
		return nil, fmt.Errorf("failed to get workflow conversation variable: %w", err)
	}

	return &variable, nil
}

// GetByConversationID retrieves all variables for a conversation
// Returns empty slice (not error) when no records exist
func (r *workflowConversationVariableRepository) GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*WorkflowConversationVariable, error) {
	var variables []*WorkflowConversationVariable
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Find(&variables).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get workflow conversation variables: %w", err)
	}

	// Return empty slice if no records found (not an error)
	return variables, nil
}

// GetByConversationAndName retrieves a specific variable by conversation ID and name
func (r *workflowConversationVariableRepository) GetByConversationAndName(ctx context.Context, conversationID uuid.UUID, name string) (*WorkflowConversationVariable, error) {
	var variable WorkflowConversationVariable

	// We need to search within the JSON data field for the variable name
	// This is a simplified approach - in production, you might want to use JSON operators
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND data LIKE ?", conversationID, fmt.Sprintf("%%\"name\":\"%s\"%%", name)).
		First(&variable).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow conversation variable not found")
		}
		return nil, fmt.Errorf("failed to get workflow conversation variable: %w", err)
	}

	return &variable, nil
}

// Update updates a workflow conversation variable
func (r *workflowConversationVariableRepository) Update(ctx context.Context, variable *WorkflowConversationVariable) error {
	variable.UpdatedAt = time.Now()

	err := r.db.WithContext(ctx).
		Where("id = ? AND conversation_id = ?", variable.ID, variable.ConversationID).
		Updates(variable).Error

	if err != nil {
		return fmt.Errorf("failed to update workflow conversation variable: %w", err)
	}
	return nil
}

// Delete deletes a workflow conversation variable
func (r *workflowConversationVariableRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&WorkflowConversationVariable{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete workflow conversation variable: %w", err)
	}
	return nil
}

// DeleteByConversationID deletes all variables for a conversation
func (r *workflowConversationVariableRepository) DeleteByConversationID(ctx context.Context, conversationID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Delete(&WorkflowConversationVariable{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete workflow conversation variables: %w", err)
	}
	return nil
}

// UpsertVariable creates or updates a variable for a conversation
func (r *workflowConversationVariableRepository) UpsertVariable(ctx context.Context, conversationID, appID uuid.UUID, name string, value interface{}, valueType, description string) error {
	// First try to find existing variable
	existingVar, err := r.GetByConversationAndName(ctx, conversationID, name)

	if err != nil && err.Error() != "workflow conversation variable not found" {
		return fmt.Errorf("failed to check existing variable: %w", err)
	}

	if existingVar != nil {
		// Update existing variable
		err = existingVar.UpdateVariable(name, value, valueType, description)
		if err != nil {
			return fmt.Errorf("failed to update variable data: %w", err)
		}
		return r.Update(ctx, existingVar)
	} else {
		// Create new variable
		newVar := CreateWorkflowConversationVariable(conversationID, appID, name, value, valueType, description)
		return r.Create(ctx, newVar)
	}
}

// GetVariablesByApp retrieves variables by app ID with pagination
func (r *workflowConversationVariableRepository) GetVariablesByApp(ctx context.Context, appID uuid.UUID, limit, offset int) ([]*WorkflowConversationVariable, int64, error) {
	var variables []*WorkflowConversationVariable
	var total int64

	query := r.db.WithContext(ctx).
		Model(&WorkflowConversationVariable{}).
		Where("app_id = ?", appID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count workflow conversation variables: %w", err)
	}

	// Get variables with pagination
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&variables).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get workflow conversation variables: %w", err)
	}

	return variables, total, nil
}
