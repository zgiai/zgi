package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/gorm"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
	ErrCredentialExists   = errors.New("credential already exists")
	ErrCredentialInactive = errors.New("credential is inactive")
	ErrCredentialExpired  = errors.New("credential has expired")
	ErrInvalidAPIKey      = errors.New("invalid API key")
)

type tenantCredentialService struct {
	repo   repository.TenantCredentialRepository
	crypto shared.CryptoService
	db     *gorm.DB
}

// NewTenantCredentialService creates a new tenant credential service
func NewTenantCredentialService(repo repository.TenantCredentialRepository, crypto shared.CryptoService, dbs ...*gorm.DB) TenantCredentialService {
	var db *gorm.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	return &tenantCredentialService{repo: repo, crypto: crypto, db: db}
}

func (s *tenantCredentialService) Create(ctx context.Context, organizationID uuid.UUID, req *dto.CreateTenantCredentialRequest) (*model.TenantCredential, error) {
	normalizedProvider, err := channelprovider.Normalize(req.ChannelProvider)
	if err != nil {
		return nil, err
	}

	hash := hashAPIKey(req.APIKey)

	exists, err := s.repo.ExistsByHash(ctx, organizationID, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to check credential existence: %w", err)
	}
	if exists {
		return nil, ErrCredentialExists
	}

	ciphertext, err := s.crypto.Encrypt(req.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt API key: %w", err)
	}

	credential := &model.TenantCredential{
		OrganizationID:   organizationID,
		Name:             req.Name,
		ChannelProvider:  normalizedProvider,
		APIKeyCiphertext: ciphertext,
		APIKeyHash:       hash,
		APIBaseURL:       req.APIBaseURL,
		IsActive:         true,
	}

	if err := s.repo.Create(ctx, credential); err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	return credential, nil
}

// GetOrCreateByAPIKey gets an existing credential by API key hash, or creates a new one if not exists.
// Returns (credential, created, error) where created is true if a new credential was created.
func (s *tenantCredentialService) GetOrCreateByAPIKey(ctx context.Context, organizationID uuid.UUID, req *dto.CreateTenantCredentialRequest) (*model.TenantCredential, bool, error) {
	normalizedProvider, err := channelprovider.Normalize(req.ChannelProvider)
	if err != nil {
		return nil, false, err
	}

	hash := hashAPIKey(req.APIKey)

	existing, err := s.repo.GetBySignature(ctx, organizationID, hash, normalizedProvider, req.APIBaseURL)
	if err == nil && existing != nil {
		return existing, false, nil
	}

	ciphertext, err := s.crypto.Encrypt(req.APIKey)
	if err != nil {
		return nil, false, fmt.Errorf("failed to encrypt API key: %w", err)
	}

	credential := &model.TenantCredential{
		OrganizationID:   organizationID,
		Name:             req.Name,
		ChannelProvider:  normalizedProvider,
		APIKeyCiphertext: ciphertext,
		APIKeyHash:       hash,
		APIBaseURL:       req.APIBaseURL,
		IsActive:         true,
	}

	if err := s.repo.Create(ctx, credential); err != nil {
		return nil, false, fmt.Errorf("failed to create credential: %w", err)
	}

	return credential, true, nil
}

func (s *tenantCredentialService) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.TenantCredential, error) {
	credential, err := s.repo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrCredentialNotFound
	}
	return credential, nil
}

func (s *tenantCredentialService) List(ctx context.Context, organizationID uuid.UUID, req *dto.ListCredentialRequest) ([]*model.TenantCredential, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	return s.repo.List(ctx, organizationID, req.Provider, req.IsActive, offset, req.PageSize)
}

func (s *tenantCredentialService) Update(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateTenantCredentialRequest) (*model.TenantCredential, error) {
	credential, err := s.repo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrCredentialNotFound
	}

	if req.Name != nil {
		credential.Name = *req.Name
	}
	if req.ChannelProvider != nil {
		normalizedProvider, err := channelprovider.Normalize(*req.ChannelProvider)
		if err != nil {
			return nil, err
		}
		credential.ChannelProvider = normalizedProvider
	}
	if req.APIBaseURL != nil {
		credential.APIBaseURL = *req.APIBaseURL
	}
	if req.IsActive != nil {
		credential.IsActive = *req.IsActive
	}
	if req.APIKey != nil {
		ciphertext, err := s.crypto.Encrypt(*req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		credential.APIKeyCiphertext = ciphertext
		credential.APIKeyHash = hashAPIKey(*req.APIKey)
	}

	if err := s.repo.Update(ctx, credential); err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}

	return credential, nil
}

func (s *tenantCredentialService) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	if s.db == nil {
		return s.repo.Delete(ctx, organizationID, id)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("organization_id = ? AND user_credential_id = ?", organizationID, id).
			Delete(&channelmodel.LLMRoute{}).Error; err != nil {
			return fmt.Errorf("failed to delete dependent routes: %w", err)
		}

		if err := tx.
			Where("id = ? AND organization_id = ?", id, organizationID).
			Delete(&model.TenantCredential{}).Error; err != nil {
			return fmt.Errorf("failed to delete credential: %w", err)
		}

		return nil
	})
}

func (s *tenantCredentialService) GetDecryptedAPIKey(ctx context.Context, organizationID, id uuid.UUID) (string, error) {
	credential, err := s.repo.GetByID(ctx, organizationID, id)
	if err != nil {
		return "", ErrCredentialNotFound
	}

	if !credential.IsUsable() {
		if !credential.IsActive {
			return "", ErrCredentialInactive
		}
		return "", ErrCredentialExpired
	}

	apiKey, err := s.crypto.Decrypt(credential.APIKeyCiphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	go func() {
		_ = s.repo.UpdateLastUsed(context.Background(), id)
	}()

	return apiKey, nil
}

func (s *tenantCredentialService) TestCredential(ctx context.Context, organizationID, id uuid.UUID, modelName string, apiBaseURL string) (*dto.TestCredentialResult, error) {
	credential, err := s.repo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrCredentialNotFound
	}

	apiKey, err := s.crypto.Decrypt(credential.APIKeyCiphertext)
	if err != nil {
		return &dto.TestCredentialResult{
			Success: false,
			Message: "failed to decrypt API key",
		}, nil
	}

	baseURL := credential.APIBaseURL
	if apiBaseURL != "" {
		baseURL = apiBaseURL
	}

	return testCredentialWithChannelProvider(ctx, credential.ChannelProvider, baseURL, apiKey, modelName)
}

// ============================================================================
// Helper functions
// ============================================================================

func hashAPIKey(apiKey string) string {
	h := sha256.New()
	h.Write([]byte(apiKey))
	return hex.EncodeToString(h.Sum(nil))
}

func testCredentialWithChannelProvider(ctx context.Context, channelProvider, baseURL, apiKey, modelName string) (*dto.TestCredentialResult, error) {
	result, err := channelprovider.TestModel(ctx, channelProvider, baseURL, apiKey, modelName)
	if err != nil {
		return nil, err
	}

	return &dto.TestCredentialResult{
		Success:        result.Success,
		Message:        result.Message,
		ResponseTimeMs: result.ResponseTimeMs,
		Model:          result.Model,
		Response:       result.Response,
	}, nil
}
