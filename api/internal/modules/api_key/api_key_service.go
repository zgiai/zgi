package APIKey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// APIKeyService handles API key operations
type APIKeyService struct {
	db *gorm.DB
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(db *gorm.DB) *APIKeyService {
	return &APIKeyService{db: db}
}

// APIKey represents an API key record
// APIKey service uses the model defined in api_key_model.go

// GenerateAPIKey generates a new API key
func (s *APIKeyService) GenerateAPIKey(ctx context.Context, agentID, tenantID uuid.UUID, name string) (string, error) {
	// Generate random key
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	key := "zgi_" + hex.EncodeToString(bytes)
	keyPrefix := key[:12] // Store first 12 characters for identification

	// Hash the key before storing
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	apiKey := &APIKey{
		AgentID:   agentID,
		TenantID:  tenantID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      name,
		Status:    "active",
	}

	logger.DebugContext(ctx, "creating api key", "agent_id", agentID.String(), "tenant_id", tenantID.String(), "name", name)
	if err := s.db.WithContext(ctx).Create(apiKey).Error; err != nil {
		logger.ErrorContext(ctx, "failed to create api key", "agent_id", agentID.String(), "tenant_id", tenantID.String(), err)
		return "", fmt.Errorf("failed to create API key: %w", err)
	}
	logger.InfoContext(ctx, "api key created", "api_key_id", apiKey.ID.String(), "agent_id", agentID.String(), "tenant_id", tenantID.String())

	return key, nil
}

// ValidateAPIKey validates an API key and returns associated agent/tenant info
func (s *APIKeyService) ValidateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	var apiKey APIKey
	err := s.db.WithContext(ctx).
		Where("key_hash = ? AND status = 'active'", key).
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

// ListAPIKeys returns all API keys for a specific agent
func (s *APIKeyService) ListAPIKeys(ctx context.Context, agentID, tenantID uuid.UUID) ([]*APIKey, error) {
	var apiKeys []*APIKey
	err := s.db.WithContext(ctx).
		Where("agent_id = ? AND tenant_id = ?", agentID, tenantID).
		Order("created_at DESC").
		Find(&apiKeys).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return apiKeys, nil
}

// GetAPIKey returns a specific API key by ID
func (s *APIKeyService) GetAPIKey(ctx context.Context, keyID, agentID, tenantID uuid.UUID) (*APIKey, error) {
	var apiKey APIKey
	err := s.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", keyID, agentID, tenantID).
		First(&apiKey).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return &apiKey, nil
}

// UpdateAPIKey updates an existing API key
func (s *APIKeyService) UpdateAPIKey(ctx context.Context, keyID, agentID, tenantID uuid.UUID, updates map[string]interface{}) error {
	// Only allow updating specific fields
	allowedFields := map[string]bool{
		"name":       true,
		"status":     true,
		"expires_at": true,
	}

	filteredUpdates := make(map[string]interface{})
	for key, value := range updates {
		if allowedFields[key] {
			filteredUpdates[key] = value
		}
	}

	if len(filteredUpdates) == 0 {
		return fmt.Errorf("no valid fields to update")
	}

	filteredUpdates["updated_at"] = time.Now()

	result := s.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", keyID, agentID, tenantID).
		Updates(filteredUpdates)

	if result.Error != nil {
		return fmt.Errorf("failed to update API key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("API key not found or no changes made")
	}

	return nil
}

// DeleteAPIKey deletes an API key
func (s *APIKeyService) DeleteAPIKey(ctx context.Context, keyID, agentID, tenantID uuid.UUID) error {
	result := s.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND tenant_id = ?", keyID, agentID, tenantID).
		Delete(&APIKey{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete API key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("API key not found")
	}

	return nil
}

// RevokeAPIKey revokes an API key by setting its status to 'revoked'
func (s *APIKeyService) RevokeAPIKey(ctx context.Context, keyID, agentID, tenantID uuid.UUID) error {
	return s.UpdateAPIKey(ctx, keyID, agentID, tenantID, map[string]interface{}{
		"status": "revoked",
	})
}

// ValidateAPIKeyByHash validates an API key using its hash
func (s *APIKeyService) ValidateAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var apiKey APIKey
	err := s.db.WithContext(ctx).
		Where("key_hash = ? AND status = 'active'", keyHash).
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

// CreateAPIKey creates a new API key and returns both the key and the database record
func (s *APIKeyService) CreateAPIKey(ctx context.Context, agentID, tenantID uuid.UUID, name string, expiresAt *time.Time) (*APIKey, string, error) {
	// Generate random key
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", err)
	}

	key := "zgi_" + hex.EncodeToString(bytes)
	keyPrefix := key[:12] // Store first 12 characters for identification

	// Hash the key before storing
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	apiKey := &APIKey{
		ID:        uuid.New(),
		AgentID:   agentID,
		TenantID:  tenantID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      name,
		Status:    "active",
		ExpiresAt: expiresAt,
	}

	logger.DebugContext(ctx, "creating api key", "api_key_id", apiKey.ID.String(), "agent_id", agentID.String(), "tenant_id", tenantID.String(), "name", name)

	// Use transaction to ensure data is committed
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(apiKey).Error; err != nil {
		logger.ErrorContext(ctx, "failed to create api key", "api_key_id", apiKey.ID.String(), "agent_id", agentID.String(), "tenant_id", tenantID.String(), err)
		tx.Rollback()
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		logger.ErrorContext(ctx, "failed to commit api key", "api_key_id", apiKey.ID.String(), "agent_id", agentID.String(), "tenant_id", tenantID.String(), err)
		return nil, "", fmt.Errorf("failed to commit API key: %w", err)
	}

	logger.InfoContext(ctx, "api key created", "api_key_id", apiKey.ID.String(), "agent_id", agentID.String(), "tenant_id", tenantID.String())

	return apiKey, key, nil
}

// HashAPIKey creates a hash of the API key for validation
func (s *APIKeyService) HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
