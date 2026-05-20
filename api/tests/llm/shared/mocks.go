package shared

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/credential/model"
)

// ErrNotFound is returned when a resource is not found
var ErrNotFound = errors.New("not found")

// MockTenantCredentialRepository is a mock implementation for tenant credentials
type MockTenantCredentialRepository struct {
	mu    sync.RWMutex
	store map[uuid.UUID]*model.TenantCredential
}

// NewMockTenantCredentialRepository creates a new mock repository
func NewMockTenantCredentialRepository() *MockTenantCredentialRepository {
	return &MockTenantCredentialRepository{
		store: make(map[uuid.UUID]*model.TenantCredential),
	}
}

func (m *MockTenantCredentialRepository) Create(ctx context.Context, cred *model.TenantCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cred.ID == uuid.Nil {
		cred.ID = uuid.New()
	}
	m.store[cred.ID] = cred
	return nil
}

func (m *MockTenantCredentialRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*model.TenantCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cred, ok := m.store[id]; ok && cred.OrganizationID == tenantID {
		return cred, nil
	}
	return nil, ErrNotFound
}

func (m *MockTenantCredentialRepository) List(ctx context.Context, tenantID uuid.UUID, provider string, isActive *bool, offset, limit int) ([]*model.TenantCredential, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []*model.TenantCredential
	for _, cred := range m.store {
		if cred.OrganizationID != tenantID {
			continue
		}
		if provider != "" && cred.ChannelProvider != provider {
			continue
		}
		if isActive != nil && cred.IsActive != *isActive {
			continue
		}
		results = append(results, cred)
	}
	total := int64(len(results))
	if offset < len(results) {
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	} else {
		results = nil
	}
	return results, total, nil
}

func (m *MockTenantCredentialRepository) Update(ctx context.Context, cred *model.TenantCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.store[cred.ID]; !ok {
		return ErrNotFound
	}
	m.store[cred.ID] = cred
	return nil
}

func (m *MockTenantCredentialRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cred, ok := m.store[id]; ok && cred.OrganizationID == tenantID {
		delete(m.store, id)
		return nil
	}
	return ErrNotFound
}

func (m *MockTenantCredentialRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *MockTenantCredentialRepository) ExistsByHash(ctx context.Context, tenantID uuid.UUID, hash string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cred := range m.store {
		if cred.OrganizationID == tenantID && cred.APIKeyHash == hash {
			return true, nil
		}
	}
	return false, nil
}

func (m *MockTenantCredentialRepository) GetByHash(ctx context.Context, tenantID uuid.UUID, hash string) (*model.TenantCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cred := range m.store {
		if cred.OrganizationID == tenantID && cred.APIKeyHash == hash {
			return cred, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MockTenantCredentialRepository) GetBySignature(ctx context.Context, tenantID uuid.UUID, hash, channelProvider, apiBaseURL string) (*model.TenantCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cred := range m.store {
		if cred.OrganizationID != tenantID {
			continue
		}
		if cred.APIKeyHash != hash {
			continue
		}
		if cred.ChannelProvider != channelProvider {
			continue
		}
		if cred.APIBaseURL != apiBaseURL {
			continue
		}
		return cred, nil
	}
	return nil, ErrNotFound
}

// MockCryptoService is a mock implementation for crypto service
type MockCryptoService struct{}

// NewMockCryptoService creates a new mock crypto service
func NewMockCryptoService() *MockCryptoService {
	return &MockCryptoService{}
}

func (m *MockCryptoService) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (m *MockCryptoService) Decrypt(ciphertext string) (string, error) {
	if len(ciphertext) > 10 && ciphertext[:10] == "encrypted:" {
		return ciphertext[10:], nil
	}
	return ciphertext, nil
}

func (m *MockCryptoService) Hash(input string) string {
	return "hash:" + input
}
