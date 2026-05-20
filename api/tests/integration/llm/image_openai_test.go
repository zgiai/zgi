package llm_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// MockOpenAIAdapter implements adapter.LLMProviderAdapter for OpenAI testing
type MockOpenAIAdapter struct {
	LastRequest *adapter.ImageRequest
}

func (m *MockOpenAIAdapter) Name() string { return "openai" }
func (m *MockOpenAIAdapter) ChatCompletion(ctx context.Context, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) ChatCompletionStream(ctx context.Context, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) CreateEmbeddings(ctx context.Context, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) CreateImage(ctx context.Context, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	m.LastRequest = req
	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data: []adapter.ImageItem{
			{
				URL:           "https://openai-fake-url.com/image.png",
				RevisedPrompt: "A cute cat in hd quality and vivid style",
			},
		},
	}, nil
}
func (m *MockOpenAIAdapter) CreateResponse(ctx context.Context, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) Rerank(ctx context.Context, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, nil
}
func (m *MockOpenAIAdapter) ValidateConfig(config *adapter.AdapterConfig) error { return nil }
func (m *MockOpenAIAdapter) GetProviderInfo() *adapter.ProviderInfo             { return nil }

func TestOpenAIImageGenerationFlow(t *testing.T) {
	// 1. Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = db.AutoMigrate(
		&LocalOrganization{},
		&LocalWorkspace{},
		&LocalMember{},
		&LocalLLMProvider{},
		&LocalOfficialModelSnapshot{},
		&LocalTenantAPIKey{},
		&LocalLLMRoute{},
		&LocalLLMCredential{},
		&LocalLLMModel{},
		&LocalBillingAttempt{},
		&LocalBillingAttemptEntry{},
		&LocalGroupAICreditAccount{},
		&LocalTransaction{},
		&LocalChannelWallet{},
		&LocalChannelWalletTransaction{},
		&LocalWorkspaceQuota{},
	)
	require.NoError(t, err)

	// 2. Seed data
	orgID := uuid.New()
	apiKeyID := "sk-test-apikey-openai"

	// Create Organization
	db.Create(&LocalOrganization{
		ID:        orgID.String(),
		Name:      "Test Org OpenAI",
		CreatedAt: time.Now(),
	})

	// Create Provider
	db.Create(&LocalLLMProvider{
		ID:           uuid.New().String(),
		Provider:     "openai",
		ProviderName: "OpenAI",
		IsActive:     true,
	})

	// Create API Key
	db.Create(&LocalTenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: orgID.String(),
		Key:            "test-key-hash-openai",
		Status:         "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		RemainQuota:    1000,
	})

	// Create Model (dall-e-3) with image prices
	modelID := uuid.New()
	imagePrices := []map[string]interface{}{
		{
			"priority": 1,
			"conditions": map[string]interface{}{
				"quality": "hd",
				"size":    "1024x1024",
			},
			"price": map[string]interface{}{
				"credits": 8, // 8 credits for hd 1024x1024
			},
		},
		{
			"priority": 0,
			"conditions": map[string]interface{}{
				"quality": "standard",
				"size":    "1024x1024",
			},
			"price": map[string]interface{}{
				"credits": 4, // 4 credits for standard 1024x1024
			},
		},
	}
	imagePricesJSON, _ := json.Marshal(imagePrices)
	db.Create(&LocalLLMModel{
		ID:          modelID,
		Model:       "dall-e-3",
		ModelName:   "DALL-E 3",
		Provider:    "openai",
		ImagePrices: imagePricesJSON,
		IsActive:    true,
		CreatedAt:   time.Now(),
	})

	// Create Credential
	db.Create(&LocalLLMCredential{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		Name:           "OpenAI Cred",
		Provider:       "openai",
		APIKey:         "fake-openai-key",
		IsActive:       true,
		CreatedAt:      time.Now(),
	})

	// Create Channel (Route)
	modelsJSON, _ := json.Marshal([]string{"dall-e-3"})
	db.Create(&LocalLLMRoute{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		Name:           "OpenAI Route",
		Provider:       "openai",
		Type:           "PRIVATE",
		Models:         modelsJSON,
		IsEnabled:      true,
		Priority:       100,
		Balance:        100.0,
		CreatedAt:      time.Now(),
	})

	// 3. Mock openai adapter
	mockAdapter := &MockOpenAIAdapter{}
	adapter.GlobalFactory.Register("openai", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return mockAdapter, nil
	})

	// 4. Initialize LLMGatewayService
	apiKeyRepo := apikeyrepo.NewAPIKeyRepository(db)

	svc, err := gateway.NewLLMGatewayService(
		db,
		apiKeyRepo,
		adapter.GlobalFactory,
	)
	require.NoError(t, err)

	// 5. Call CreateImage
	n := 1
	req := &adapter.ImageRequest{
		Model:   "dall-e-3",
		Prompt:  "A futuristic city",
		Size:    "1024x1024",
		Quality: "hd",
		Style:   "vivid",
		N:       &n,
	}

	// Construct TenantAPIKey
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: orgID.String(),
		Key:            "test-key-hash-openai",
		Status:         "active",
	}

	resp, err := svc.CreateImage(context.Background(), apiKey, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Data)
	assert.Equal(t, "https://openai-fake-url.com/image.png", resp.Data[0].URL)

	// 6. Verify Request Parameters passed to Adapter
	require.NotNil(t, mockAdapter.LastRequest)
	assert.Equal(t, "dall-e-3", mockAdapter.LastRequest.Model)
	assert.Equal(t, "hd", mockAdapter.LastRequest.Quality)
	assert.Equal(t, "vivid", mockAdapter.LastRequest.Style)
	assert.Equal(t, "1024x1024", mockAdapter.LastRequest.Size)

	// 7. Verify Billing (Should match HD price = 8 credits)
	var walletTx LocalChannelWalletTransaction
	err = db.Order("created_at DESC").First(&walletTx).Error
	require.NoError(t, err)

	assert.Equal(t, int64(-8), walletTx.Amount)
	assert.Equal(t, "prededuct", walletTx.Type)
}
