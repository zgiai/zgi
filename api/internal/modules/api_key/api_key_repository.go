package APIKey

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIKeyRepository interface defines the contract for API key data operations
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *APIKey) error
	GetByID(ctx context.Context, id, agentID, tenantID uuid.UUID) (*APIKey, error)
	GetByKeyHash(ctx context.Context, keyHash string) (*APIKey, error)
	List(ctx context.Context, agentID, tenantID uuid.UUID) ([]*APIKey, error)
	Update(ctx context.Context, id, agentID, tenantID uuid.UUID, updates map[string]interface{}) error
	Delete(ctx context.Context, id, agentID, tenantID uuid.UUID) error
	CountByAgent(ctx context.Context, agentID, tenantID uuid.UUID) (int64, error)
	CheckNameExists(ctx context.Context, name string, agentID, tenantID uuid.UUID, excludeID *uuid.UUID) (bool, error)
}

// apiKeyRepository implements APIKeyRepository interface
type apiKeyRepository struct {
	db *gorm.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(db *gorm.DB) APIKeyRepository {
	return &apiKeyRepository{db: db}
}

// Create creates a new API key record
func (r *apiKeyRepository) Create(ctx context.Context, apiKey *APIKey) error {
	if err := r.db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}
	return nil
}

// GetByID retrieves an API key by its ID, agent ID, and tenant ID
func (r *apiKeyRepository) GetByID(ctx context.Context, id, agentID, tenantID uuid.UUID) (*APIKey, error) {
	var apiKey APIKey
	err := r.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", id, agentID, tenantID).
		First(&apiKey).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return &apiKey, nil
}

// GetByKeyHash retrieves an API key by its hash
func (r *apiKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var apiKey APIKey
	err := r.db.WithContext(ctx).
		Where("key_hash = ? AND status = ?", keyHash, APIKeyStatusActive).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		First(&apiKey).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid or expired API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	return &apiKey, nil
}

// List retrieves all API keys for a specific agent and tenant
func (r *apiKeyRepository) List(ctx context.Context, agentID, tenantID uuid.UUID) ([]*APIKey, error) {
	var apiKeys []*APIKey
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND tenant_id = ? AND status != ?", agentID, tenantID, APIKeyStatusRevoked).
		Order("created_at DESC").
		Find(&apiKeys).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return apiKeys, nil
}

// Update updates an API key with the provided fields
func (r *apiKeyRepository) Update(ctx context.Context, id, agentID, tenantID uuid.UUID, updates map[string]interface{}) error {
	// Add updated_at timestamp
	updates["updated_at"] = time.Now()

	result := r.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", id, agentID, tenantID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update API key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("API key not found or no changes made")
	}

	return nil
}

// Delete deletes an API key
func (r *apiKeyRepository) Delete(ctx context.Context, id, agentID, tenantID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", id, agentID, tenantID).
		Delete(&APIKey{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete API key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("API key not found")
	}

	return nil
}

// CountByAgent counts the number of API keys for a specific agent and tenant
func (r *apiKeyRepository) CountByAgent(ctx context.Context, agentID, tenantID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("agent_id = ? AND tenant_id = ? AND status != ?", agentID, tenantID, APIKeyStatusRevoked).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count API keys: %w", err)
	}

	return count, nil
}

// CheckNameExists checks if an API key name already exists for the given agent and tenant
func (r *apiKeyRepository) CheckNameExists(ctx context.Context, name string, agentID, tenantID uuid.UUID, excludeID *uuid.UUID) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("name = ? AND agent_id = ? AND tenant_id = ? AND status != ?", name, agentID, tenantID, APIKeyStatusRevoked)

	// Exclude the current API key when updating
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	err := query.Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check name existence: %w", err)
	}

	return count > 0, nil
}
