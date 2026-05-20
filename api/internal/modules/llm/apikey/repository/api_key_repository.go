package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

type apiKeyRepositoryImpl struct {
	db *gorm.DB
}

const (
	apiKeyCachePrefix = "llm:apikey:"
	apiKeyCacheTTL    = 5 * time.Minute
)

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(db *gorm.DB) APIKeyRepository {
	return &apiKeyRepositoryImpl{db: db}
}

// Create creates a new API key
func (r *apiKeyRepositoryImpl) Create(ctx context.Context, apiKey *model.TenantAPIKey) error {
	return r.db.WithContext(ctx).Create(apiKey).Error
}

// GetByID gets an API key by ID
func (r *apiKeyRepositoryImpl) GetByID(ctx context.Context, id, organizationID string) (*model.TenantAPIKey, error) {
	var apiKey model.TenantAPIKey
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetByIDInOrganizations gets an external API key by ID within allowed organizations.
func (r *apiKeyRepositoryImpl) GetByIDInOrganizations(ctx context.Context, id string, organizationIDs []string) (*model.TenantAPIKey, error) {
	if len(organizationIDs) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	var apiKey model.TenantAPIKey
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id IN ? AND is_internal = ?", id, organizationIDs, false).
		First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetByKey gets an API key by key string (deprecated, use GetByKeyHash)
func (r *apiKeyRepositoryImpl) GetByKey(ctx context.Context, key string) (*model.TenantAPIKey, error) {
	// Hash the key to query database (encryption produces different results each time)
	keyHash := util.HashAPIKey(key)
	return r.GetByKeyHash(ctx, keyHash)
}

// GetByKeyHash gets an API key by key hash
func (r *apiKeyRepositoryImpl) GetByKeyHash(ctx context.Context, keyHash string) (*model.TenantAPIKey, error) {
	// Try cache first
	cacheKey := apiKeyCacheKey(keyHash)
	if redis.GetClient() != nil {
		if cached, err := redis.GetString(ctx, cacheKey); err == nil && cached != "" {
			var apiKey model.TenantAPIKey
			if err := json.Unmarshal([]byte(cached), &apiKey); err == nil {
				return &apiKey, nil
			}
		}
	}

	// Cache miss, query database
	var apiKey model.TenantAPIKey
	err := r.db.WithContext(ctx).
		Where("key_hash = ?", keyHash).
		First(&apiKey).Error
	if err != nil {
		return nil, err
	}

	// Store in cache (5 minutes TTL)
	if redis.GetClient() != nil {
		if data, err := json.Marshal(&apiKey); err == nil {
			_ = redis.SetEx(ctx, cacheKey, string(data), apiKeyCacheTTL)
		}
	}

	return &apiKey, nil
}

// List lists API keys with filters and pagination
func (r *apiKeyRepositoryImpl) List(ctx context.Context, organizationID string, filters map[string]interface{}, page, limit int) ([]*model.TenantAPIKey, int64, error) {
	var apiKeys []*model.TenantAPIKey
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TenantAPIKey{}).
		Where("organization_id = ?", organizationID)

	// Apply filters
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}

	if search, ok := filters["search"].(string); ok && search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	if isInternal, ok := filters["is_internal"].(bool); ok {
		query = query.Where("is_internal = ?", isInternal)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&apiKeys).Error; err != nil {
		return nil, 0, err
	}

	return apiKeys, total, nil
}

// Update updates an API key
func (r *apiKeyRepositoryImpl) Update(ctx context.Context, apiKey *model.TenantAPIKey) error {
	if err := r.db.WithContext(ctx).Save(apiKey).Error; err != nil {
		return err
	}
	r.invalidateAPIKeyCache(ctx, apiKey.KeyHash)
	return nil
}

// Delete soft deletes an API key
func (r *apiKeyRepositoryImpl) Delete(ctx context.Context, id, organizationID string) error {
	var apiKey model.TenantAPIKey
	if err := r.db.WithContext(ctx).
		Select("id", "key_hash").
		Where("id = ? AND organization_id = ? AND is_internal = ?", id, organizationID, false).
		First(&apiKey).Error; err != nil {
		return err
	}

	if err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND is_internal = ?", id, organizationID, false).
		Delete(&model.TenantAPIKey{}).Error; err != nil {
		return err
	}

	r.invalidateAPIKeyCache(ctx, apiKey.KeyHash)
	return nil
}

// UpdateAccessedAt updates the last accessed time
func (r *apiKeyRepositoryImpl) UpdateAccessedAt(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.TenantAPIKey{}).
		Where("id = ?", id).
		Update("accessed_at", now).Error
}

// UpdateQuota updates the quota usage
func (r *apiKeyRepositoryImpl) UpdateQuota(ctx context.Context, id string, usedDelta, remainDelta int64) error {
	var keyHash string
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var apiKey model.TenantAPIKey
		if err := tx.Where("id = ?", id).First(&apiKey).Error; err != nil {
			return err
		}
		keyHash = apiKey.KeyHash

		// Update quota
		apiKey.UsedQuota += usedDelta
		apiKey.RemainQuota += remainDelta

		// Validate quota
		if apiKey.UsedQuota < 0 {
			return fmt.Errorf("used quota cannot be negative")
		}
		if apiKey.RemainQuota < 0 {
			return fmt.Errorf("remain quota cannot be negative")
		}

		return tx.Save(&apiKey).Error
	}); err != nil {
		return err
	}

	r.invalidateAPIKeyCache(ctx, keyHash)
	return nil
}

// CountByTenant counts API keys for a tenant
func (r *apiKeyRepositoryImpl) CountByTenant(ctx context.Context, organizationID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.TenantAPIKey{}).
		Where("organization_id = ?", organizationID).
		Count(&count).Error
	return count, err
}

func apiKeyCacheKey(keyHash string) string {
	return apiKeyCachePrefix + keyHash
}

func (r *apiKeyRepositoryImpl) invalidateAPIKeyCache(ctx context.Context, keyHash string) {
	if keyHash == "" {
		return
	}

	client := redis.GetClient()
	if client == nil {
		return
	}

	_ = client.Del(ctx, apiKeyCacheKey(keyHash)).Err()
}
