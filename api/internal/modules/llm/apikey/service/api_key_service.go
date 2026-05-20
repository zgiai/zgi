package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/zgiai/ginext/internal/modules/llm/apikey/dto"
	"github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	"github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/util"
	"gorm.io/gorm"
)

type apiKeyServiceImpl struct {
	db                  *gorm.DB
	apiKeyRepo          repository.APIKeyRepository
	tenantService       interfaces.WorkspaceManagementService
	organizationService interfaces.OrganizationService
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(db *gorm.DB, apiKeyRepo repository.APIKeyRepository, tenantService interfaces.WorkspaceManagementService, organizationService interfaces.OrganizationService) APIKeyService {
	return &apiKeyServiceImpl{
		db:                  db,
		apiKeyRepo:          apiKeyRepo,
		tenantService:       tenantService,
		organizationService: organizationService,
	}
}

// generateAPIKey generates a random API key
func generateAPIKey() (string, error) {
	// Generate 32 bytes of random data
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Encode to base64 and add prefix
	key := "sk-" + base64.URLEncoding.EncodeToString(b)

	// Ensure the key is exactly 48 characters
	if len(key) > 48 {
		key = key[:48]
	}

	return key, nil
}

// modelToResponse converts model to response DTO
func (s *apiKeyServiceImpl) modelToResponse(ctx context.Context, apiKey *model.TenantAPIKey, includeFullKey bool, plainKey ...string) *dto.APIKeyResponse {
	// Parse ModelLimits from JSON string to string array
	var modelLimits []string
	if apiKey.ModelLimits != nil && *apiKey.ModelLimits != "" {
		if err := json.Unmarshal([]byte(*apiKey.ModelLimits), &modelLimits); err != nil {
			modelLimits = []string{}
		}
	}

	// Determine key value and masked key
	var keyValue string
	var keyMasked string

	if includeFullKey && len(plainKey) > 0 {
		keyValue = plainKey[0]
		keyMasked = util.ObfuscateAPIKey(plainKey[0])
	} else {
		decryptedKey, err := util.DecryptAPIKey(apiKey.Key)
		if err != nil {
			keyValue = ""
			keyMasked = "****"
		} else {
			keyValue = decryptedKey
			keyMasked = util.ObfuscateAPIKey(decryptedKey)
		}
	}

	// Fetch organization name
	organizationName := ""
	if org, err := s.organizationService.GetOrganizationByID(ctx, apiKey.OrganizationID); err == nil && org != nil {
		organizationName = org.Name
	}

	return &dto.APIKeyResponse{
		ID:                 apiKey.ID,
		OrganizationID:     apiKey.OrganizationID,
		OrganizationName:   organizationName,
		Key:                keyValue,
		KeyMasked:          keyMasked,
		Name:               apiKey.Name,
		Status:             apiKey.Status,
		CreatedAt:          apiKey.CreatedAt,
		UpdatedAt:          apiKey.UpdatedAt,
		AccessedAt:         apiKey.AccessedAt,
		ExpiresAt:          apiKey.ExpiresAt,
		UsedQuota:          apiKey.UsedQuota,
		RemainQuota:        apiKey.RemainQuota,
		QuotaLimit:         apiKey.QuotaLimit,
		ModelLimitsEnabled: apiKey.ModelLimitsEnabled,
		ModelLimits:        modelLimits,
		AllowIPs:           apiKey.AllowIPs,
	}
}

// CreateAPIKey creates new API keys (supports batch creation)
func (s *apiKeyServiceImpl) CreateAPIKey(ctx context.Context, req *dto.CreateAPIKeyRequest) (*dto.CreateAPIKeyResponse, error) {
	if req.OrganizationID == nil || *req.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}
	finalOrganizationID := *req.OrganizationID

	// Validate quota settings
	var quotaLimit *int64
	var remainQuota int64

	quotaType := req.QuotaType
	if quotaType == "" {
		quotaType = dto.QuotaTypeUnlimited
	}

	switch quotaType {
	case dto.QuotaTypeUnlimited:
		quotaLimit = nil
		remainQuota = 0
	case dto.QuotaTypeCustom:
		if req.QuotaAmount == nil || *req.QuotaAmount <= 0 {
			return nil, fmt.Errorf("quota_amount is required and must be greater than 0 when quota_type is custom")
		}
		quotaLimit = req.QuotaAmount
		remainQuota = *req.QuotaAmount
	default:
		return nil, fmt.Errorf("invalid quota_type: %s", req.QuotaType)
	}

	// Handle model limits
	var modelLimits *string
	modelLimitsEnabled := false

	allowAllModels := true
	if req.AllowAllModels != nil {
		allowAllModels = *req.AllowAllModels
	}

	if !allowAllModels && len(req.ModelNames) > 0 {
		if err := s.validateTenantModels(ctx, finalOrganizationID, req.ModelNames); err != nil {
			return nil, fmt.Errorf("model validation failed: %w", err)
		}

		modelJSON, err := json.Marshal(req.ModelNames)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal model names: %w", err)
		}
		modelLimitsStr := string(modelJSON)
		modelLimits = &modelLimitsStr
		modelLimitsEnabled = true
	}

	// Default count to 1
	count := req.Count
	if count <= 0 {
		count = 1
	}

	createdKeys := make([]dto.APIKeyResponse, 0, count)

	for i := 0; i < count; i++ {
		key, err := generateAPIKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate API key: %w", err)
		}

		keyName := req.Name
		if count > 1 {
			keyName = fmt.Sprintf("%s-%d", req.Name, i+1)
		}

		encryptedKey, err := util.EncryptAPIKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key %d: %w", i+1, err)
		}

		keyHash := util.HashAPIKey(key)

		apiKey := &model.TenantAPIKey{
			OrganizationID:     finalOrganizationID,
			Key:                encryptedKey,
			KeyHash:            keyHash,
			Name:               keyName,
			Status:             "active",
			ExpiresAt:          req.ExpiresAt,
			UsedQuota:          0,
			RemainQuota:        remainQuota,
			QuotaLimit:         quotaLimit,
			ModelLimitsEnabled: modelLimitsEnabled,
			ModelLimits:        modelLimits,
			AllowIPs:           req.AllowIPs,
		}

		if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
			return nil, fmt.Errorf("failed to create API key %d: %w", i+1, err)
		}

		resp := s.modelToResponse(ctx, apiKey, true, key)
		createdKeys = append(createdKeys, *resp)
	}

	message := fmt.Sprintf("Successfully created %d API key(s). Please save them securely as they won't be shown again.", count)

	return &dto.CreateAPIKeyResponse{
		Keys:    createdKeys,
		Count:   count,
		Message: message,
	}, nil
}

// validateTenantModels validates that all model names exist in llm_tenant_model_configs for the given tenant
// Uses the unified table with UUID model_id and JOINs to llm_models to match by name
func (s *apiKeyServiceImpl) validateTenantModels(ctx context.Context, organizationID string, modelNames []string) error {
	if len(modelNames) == 0 {
		return nil
	}

	var count int64
	err := s.db.WithContext(ctx).
		Table("llm_model_configs").
		Joins("JOIN llm_models ON llm_models.id = llm_model_configs.model_id").
		Where("llm_model_configs.organization_id = ? AND llm_models.name IN ? AND llm_model_configs.is_enabled = ? AND llm_model_configs.deleted_at IS NULL AND llm_models.deleted_at IS NULL", organizationID, modelNames, true).
		Count(&count).Error

	if err != nil {
		return fmt.Errorf("failed to query tenant model configs: %w", err)
	}

	if int(count) != len(modelNames) {
		return fmt.Errorf("some models are not available for this tenant or are not enabled")
	}

	return nil
}

// GetAPIKey gets an API key by ID
func (s *apiKeyServiceImpl) GetAPIKey(ctx context.Context, id string, organizationIDs []string) (*dto.APIKeyResponse, error) {
	apiKey, err := s.apiKeyRepo.GetByIDInOrganizations(ctx, id, organizationIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return s.modelToResponse(ctx, apiKey, false), nil
}

// ListAPIKeys lists API keys with filters and pagination
func (s *apiKeyServiceImpl) ListAPIKeys(ctx context.Context, req *dto.ListAPIKeyRequest) (*dto.ListAPIKeyResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	var apiKeys []*model.TenantAPIKey
	var total int64

	query := s.db.WithContext(ctx).Model(&model.TenantAPIKey{}).
		Where("is_internal = ?", false)

	// Filter by multiple tenant IDs (for group-level queries)
	if len(req.OrganizationIDs) > 0 {
		query = query.Where("organization_id IN ?", req.OrganizationIDs)
	} else if req.OrganizationID != nil && *req.OrganizationID != "" {
		query = query.Where("organization_id = ?", *req.OrganizationID)
	}

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	if req.Search != "" {
		query = query.Where("name ILIKE ?", "%"+req.Search+"%")
	}
	if req.IsInternal != nil {
		query = query.Where("is_internal = ?", *req.IsInternal)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count API keys: %w", err)
	}

	offset := (req.Page - 1) * req.Limit
	err := query.Order("created_at DESC").
		Limit(req.Limit).
		Offset(offset).
		Find(&apiKeys).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	items := make([]dto.APIKeyResponse, 0, len(apiKeys))
	for _, apiKey := range apiKeys {
		items = append(items, *s.modelToResponse(ctx, apiKey, false))
	}

	totalPages := int(math.Ceil(float64(total) / float64(req.Limit)))

	return &dto.ListAPIKeyResponse{
		Items:      items,
		Total:      total,
		Page:       req.Page,
		Limit:      req.Limit,
		TotalPages: totalPages,
	}, nil
}

// UpdateAPIKey updates an API key
func (s *apiKeyServiceImpl) UpdateAPIKey(ctx context.Context, id string, organizationIDs []string, req *dto.UpdateAPIKeyRequest) (*dto.APIKeyResponse, error) {
	apiKey, err := s.apiKeyRepo.GetByIDInOrganizations(ctx, id, organizationIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	if req.Name != nil {
		apiKey.Name = *req.Name
	}

	if req.Status != nil {
		apiKey.Status = *req.Status
	}

	if req.ExpiresAt != nil {
		apiKey.ExpiresAt = req.ExpiresAt
	}

	if req.QuotaLimit != nil {
		apiKey.QuotaLimit = req.QuotaLimit
		remaining := *req.QuotaLimit - apiKey.UsedQuota
		if remaining < 0 {
			remaining = 0
		}
		apiKey.RemainQuota = remaining
	}

	if req.RemainQuota != nil {
		apiKey.RemainQuota = *req.RemainQuota
	}

	if req.ModelLimitsEnabled != nil {
		apiKey.ModelLimitsEnabled = *req.ModelLimitsEnabled
	}

	if req.ModelLimits != nil {
		if len(req.ModelLimits) > 0 {
			if err := s.validateTenantModels(ctx, apiKey.OrganizationID, req.ModelLimits); err != nil {
				return nil, fmt.Errorf("model validation failed: %w", err)
			}
			modelJSON, err := json.Marshal(req.ModelLimits)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal model names: %w", err)
			}
			modelLimitsStr := string(modelJSON)
			apiKey.ModelLimits = &modelLimitsStr
		} else {
			apiKey.ModelLimits = nil
		}
	}

	if req.AllowIPs != nil {
		apiKey.AllowIPs = *req.AllowIPs
	}

	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to update API key: %w", err)
	}

	return s.modelToResponse(ctx, apiKey, false), nil
}

// DeleteAPIKey deletes an API key
func (s *apiKeyServiceImpl) DeleteAPIKey(ctx context.Context, id string, organizationIDs []string) (*dto.DeleteAPIKeyResponse, error) {
	apiKey, err := s.apiKeyRepo.GetByIDInOrganizations(ctx, id, organizationIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	if err := s.apiKeyRepo.Delete(ctx, id, apiKey.OrganizationID); err != nil {
		return nil, fmt.Errorf("failed to delete API key: %w", err)
	}

	return &dto.DeleteAPIKeyResponse{
		ID:      id,
		Message: "API key deleted successfully",
	}, nil
}

// ValidateAPIKey validates an API key
func (s *apiKeyServiceImpl) ValidateAPIKey(ctx context.Context, key string) (*dto.ValidateAPIKeyResponse, error) {
	keyHash := util.HashAPIKey(key)

	apiKey, err := s.apiKeyRepo.GetByKeyHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &dto.ValidateAPIKeyResponse{
				Valid:   false,
				Message: "Invalid API key",
			}, nil
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	if !apiKey.IsActive() {
		return &dto.ValidateAPIKeyResponse{
			Valid:   false,
			Message: "API key is not active or has expired",
		}, nil
	}

	if !apiKey.HasQuota() {
		return &dto.ValidateAPIKeyResponse{
			Valid:   false,
			Message: "API key has no remaining quota",
		}, nil
	}

	// Update accessed time asynchronously
	go func() {
		_ = s.apiKeyRepo.UpdateAccessedAt(context.Background(), apiKey.ID)
	}()

	// Fetch organization name
	organizationName := ""
	if org, err := s.organizationService.GetOrganizationByID(ctx, apiKey.OrganizationID); err == nil && org != nil {
		organizationName = org.Name
	}

	return &dto.ValidateAPIKeyResponse{
		Valid:            true,
		OrganizationID:   apiKey.OrganizationID,
		OrganizationName: organizationName,
		KeyID:            apiKey.ID,
		KeyName:          apiKey.Name,
		ExpiresAt:        apiKey.ExpiresAt,
		Message:          "API key is valid",
	}, nil
}

// UpdateAccessedAt updates the last accessed time
func (s *apiKeyServiceImpl) UpdateAccessedAt(ctx context.Context, id string) error {
	return s.apiKeyRepo.UpdateAccessedAt(ctx, id)
}
