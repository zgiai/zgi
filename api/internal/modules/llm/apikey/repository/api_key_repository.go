package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
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
		Where("id = ? AND organization_id IN ? AND is_internal = ? AND principal_type IS NULL", id, organizationIDs, false).
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
				// KeyHash is intentionally excluded from JSON so it never appears in
				// cached values or API responses. Restore it from the trusted cache
				// lookup key for the authoritative principal-access recheck.
				apiKey.KeyHash = keyHash
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

// ValidatePrincipalAccess performs the dynamic checks that must not be trusted
// from the API-key cache. Legacy organization keys have no principal and keep
// their existing behavior.
func (r *apiKeyRepositoryImpl) ValidatePrincipalAccess(ctx context.Context, apiKey *model.TenantAPIKey) error {
	if apiKey == nil {
		return gorm.ErrRecordNotFound
	}
	if apiKey.PrincipalType == nil && apiKey.PrincipalID == nil {
		return nil
	}
	if apiKey.PrincipalType == nil || apiKey.PrincipalID == nil || apiKey.WorkspaceID == nil || apiKey.AccessGrantID == nil {
		return fmt.Errorf("personal API key has incomplete principal scope")
	}
	// A personal key may have been served from Redis before a lifecycle update.
	// Re-read it here so disable, revoke, expiry, or deletion is authoritative
	// even when best-effort cache invalidation failed.
	var persisted model.TenantAPIKey
	if err := r.db.WithContext(ctx).
		Where("id = ? AND key_hash = ?", apiKey.ID, apiKey.KeyHash).
		First(&persisted).Error; err != nil {
		return fmt.Errorf("reload personal API key: %w", err)
	}
	if !persisted.IsActive() {
		if persisted.Status == "active" && persisted.ExpiresAt != nil && persisted.ExpiresAt.Before(time.Now()) {
			return apperror.New(llmerrors.AppCodeAPIKeyExpired,
				apperror.WithOperation("apikey.validate_principal_access"))
		}
		if persisted.Status == "inactive" {
			return apperror.New(llmerrors.AppCodeAPIKeyInactive,
				apperror.WithOperation("apikey.validate_principal_access"))
		}
		return errors.New("personal API key is inactive or expired")
	}
	*apiKey = persisted

	var workspace workspacemodel.Workspace
	if err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", *apiKey.WorkspaceID, apiKey.OrganizationID).
		First(&workspace).Error; err != nil {
		return fmt.Errorf("load personal API key workspace: %w", err)
	}
	if !workspace.IsNormal() {
		return errors.New("API key workspace is archived")
	}
	var organization workspacemodel.Organization
	if err := r.db.WithContext(ctx).
		Where("id = ?", apiKey.OrganizationID).
		First(&organization).Error; err != nil {
		return fmt.Errorf("load personal API key organization: %w", err)
	}
	if !organization.IsActive() {
		return errors.New("API key organization is inactive or archived")
	}
	var grant accessmodel.Grant
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND workspace_id = ? AND principal_type = ? AND principal_id = ?", *apiKey.AccessGrantID, apiKey.OrganizationID, *apiKey.WorkspaceID, *apiKey.PrincipalType, *apiKey.PrincipalID).
		First(&grant).Error
	if err != nil {
		return fmt.Errorf("load personal API key grant: %w", err)
	}
	if !grant.IsActive(time.Now()) || grant.AuthorizationVersion != apiKey.AuthorizationVersion {
		return errors.New("developer access grant is inactive or stale")
	}
	var policy accessmodel.Policy
	policyErr := r.db.WithContext(ctx).
		Where("workspace_id = ?", *apiKey.WorkspaceID).
		First(&policy).Error
	if policyErr != nil && !errors.Is(policyErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load personal API key policy: %w", policyErr)
	}
	if policyErr == nil && policy.Mode == accessmodel.AccessModeDisabled {
		return errors.New("developer access is disabled for the workspace")
	}
	if *apiKey.PrincipalType == accessmodel.PrincipalTypeUser {
		var count int64
		if err := r.db.WithContext(ctx).Table("workspace_members").
			Where("workspace_id = ? AND account_id = ?", *apiKey.WorkspaceID, *apiKey.PrincipalID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count personal API key workspace membership: %w", err)
		}
		if count == 0 {
			return errors.New("API key principal is no longer a workspace member")
		}
	}
	// Authorization failures take precedence over quota: removed members must
	// not receive allowance information, even when their grant is exhausted.
	if grant.QuotaLimit != nil && grant.RemainQuota <= 0 {
		return apperror.New(llmerrors.AppCodeDeveloperQuotaExhausted,
			apperror.WithOperation("apikey.validate_principal_access"))
	}
	return nil
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
		Where("id = ? AND organization_id = ? AND is_internal = ? AND principal_type IS NULL", id, organizationID, false).
		First(&apiKey).Error; err != nil {
		return err
	}

	if err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND is_internal = ? AND principal_type IS NULL", id, organizationID, false).
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

// InvalidateKeyCache allows transactional domain services to invalidate a key
// only after their transaction commits.
func (r *apiKeyRepositoryImpl) InvalidateKeyCache(ctx context.Context, keyHash string) {
	r.invalidateAPIKeyCache(ctx, keyHash)
}
