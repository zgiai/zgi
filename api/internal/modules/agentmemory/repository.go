package agentmemory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) WithTransaction(ctx context.Context, fn func(store) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) ResolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	var row struct {
		WorkspaceID uuid.UUID `gorm:"column:tenant_id"`
	}
	err := r.db.WithContext(ctx).
		Table("agents").
		Select("tenant_id").
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(&row).Error
	if err != nil {
		return uuid.Nil, err
	}
	return row.WorkspaceID, nil
}

func (r *Repository) LockAgent(ctx context.Context, agentID uuid.UUID) error {
	var row struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	return r.db.WithContext(ctx).
		Table("agents").
		Select("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(&row).Error
}

func (r *Repository) ListSlots(ctx context.Context, workspaceID, agentID uuid.UUID, enabledOnly bool) ([]*AgentMemorySlot, error) {
	var slots []*AgentMemorySlot
	query := r.db.WithContext(ctx).Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("sort_order ASC, created_at ASC").Find(&slots).Error
	return slots, err
}

func (r *Repository) CreateSlot(ctx context.Context, slot *AgentMemorySlot) error {
	return r.db.WithContext(ctx).Create(slot).Error
}

func (r *Repository) UpdateSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID, values map[string]interface{}) (*AgentMemorySlot, error) {
	if err := r.db.WithContext(ctx).
		Model(&AgentMemorySlot{}).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		Updates(values).Error; err != nil {
		return nil, err
	}
	return r.GetSlotScoped(ctx, workspaceID, agentID, slotID)
}

func (r *Repository) DeleteSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		Delete(&AgentMemorySlot{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) GetSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) (*AgentMemorySlot, error) {
	var slot AgentMemorySlot
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		First(&slot).Error
	if err != nil {
		return nil, err
	}
	return &slot, nil
}

func (r *Repository) ListValuesForAgent(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemoryValue, error) {
	var values []*AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID).
		Find(&values).Error
	return values, err
}

func (r *Repository) ListValuesForUser(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]*AgentMemoryValue, error) {
	var values []*AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, userScope, userID).
		Find(&values).Error
	return values, err
}

func (r *Repository) GetValueScoped(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error) {
	var value AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND slot_key = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, slotKey, userScope, userID).
		First(&value).Error
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) UpsertValue(ctx context.Context, value *AgentMemoryValue) error {
	now := time.Now()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "workspace_id"},
			{Name: "agent_id"},
			{Name: "slot_key"},
			{Name: "user_scope"},
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"content", "updated_at"}),
	}).Create(value).Error
}

func (r *Repository) CreateEvent(ctx context.Context, event *AgentMemoryEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}
