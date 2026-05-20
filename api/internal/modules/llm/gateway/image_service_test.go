package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

type LocalOrganization struct {
	ID        string         `gorm:"column:id;primaryKey"`
	Name      string         `gorm:"column:name"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (LocalOrganization) TableName() string { return "organizations" }

type LocalWorkspace struct {
	ID             string         `gorm:"column:id;primaryKey"`
	OrganizationID *string        `gorm:"column:organization_id;index"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (LocalWorkspace) TableName() string { return "workspaces" }

type LocalMember struct {
	ID             string         `gorm:"column:id;primaryKey"`
	OrganizationID string         `gorm:"column:organization_id;index"`
	AccountID      string         `gorm:"column:account_id"`
	Role           string         `gorm:"column:role"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (LocalMember) TableName() string { return "members" }

type LocalOfficialModelSnapshot struct {
	SourceKey          string     `gorm:"column:source_key;primaryKey"`
	EffectiveModels    []string   `gorm:"column:effective_models;serializer:json"`
	LatestModels       []string   `gorm:"column:latest_models;serializer:json"`
	PreviousModels     []string   `gorm:"column:previous_models;serializer:json"`
	LatestEventVersion int64      `gorm:"column:latest_event_version"`
	LatestSyncedAt     *time.Time `gorm:"column:latest_synced_at"`
	EffectiveUpdatedAt *time.Time `gorm:"column:effective_updated_at"`
	LastCheckStatus    string     `gorm:"column:last_check_status"`
	LastRejectReason   string     `gorm:"column:last_reject_reason"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (LocalOfficialModelSnapshot) TableName() string { return "llm_official_model_snapshots" }

type LocalLLMProvider struct {
	ID               string                 `gorm:"column:id;primaryKey"`
	Object           string                 `gorm:"column:object"`
	Provider         string                 `gorm:"column:provider;index"`
	ProviderName     string                 `gorm:"column:provider_name"`
	LogoURL          string                 `gorm:"column:logo_url"`
	Website          string                 `gorm:"column:website"`
	DocumentationURL string                 `gorm:"column:documentation_url"`
	PricingURL       string                 `gorm:"column:pricing_url"`
	CountryCode      string                 `gorm:"column:country_code"`
	Tagline          string                 `gorm:"column:tagline"`
	Description      string                 `gorm:"column:description"`
	Metadata         map[string]interface{} `gorm:"column:metadata;serializer:json"`
	CreatedAt        time.Time              `gorm:"column:created_at"`
	UpdatedAt        time.Time              `gorm:"column:updated_at"`
	FoundedYear      int                    `gorm:"column:founded_year"`
	APIBaseURL       string                 `gorm:"column:api_base_url"`
	ProviderType     string                 `gorm:"column:provider_type"`
	IsActive         bool                   `gorm:"column:is_active"`
	SortOrder        int                    `gorm:"column:sort_order"`
	DeletedAt        gorm.DeletedAt         `gorm:"column:deleted_at;index"`
}

func (LocalLLMProvider) TableName() string { return "llm_providers" }

type LocalLLMCredential struct {
	ID               string                 `gorm:"column:id;primaryKey"`
	OrganizationID   string                 `gorm:"column:organization_id;index"`
	Name             string                 `gorm:"column:name"`
	ChannelProvider  string                 `gorm:"column:provider"`
	APIKeyCiphertext string                 `gorm:"column:api_key_ciphertext"`
	APIKeyHash       string                 `gorm:"column:api_key_hash"`
	APIBaseURL       string                 `gorm:"column:api_base_url"`
	IsActive         bool                   `gorm:"column:is_active"`
	LastUsedAt       *time.Time             `gorm:"column:last_used_at"`
	ExpiresAt        *time.Time             `gorm:"column:expires_at"`
	Metadata         map[string]interface{} `gorm:"column:metadata;serializer:json"`
	CreatedAt        time.Time              `gorm:"column:created_at"`
	UpdatedAt        time.Time              `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt         `gorm:"column:deleted_at;index"`
}

func (LocalLLMCredential) TableName() string { return "llm_credentials" }

type LocalLLMRoute struct {
	ID               string                 `gorm:"column:id;primaryKey"`
	OrganizationID   string                 `gorm:"column:organization_id;index"`
	Type             string                 `gorm:"column:type;index"`
	CredentialID     *string                `gorm:"column:user_credential_id;index"`
	Name             string                 `gorm:"column:name"`
	Models           []string               `gorm:"column:models;serializer:json"`
	ChannelProvider  string                 `gorm:"column:provider"`
	APIBaseURL       string                 `gorm:"column:api_base_url"`
	NativeProtocols  map[string]interface{} `gorm:"column:native_protocols;serializer:json"`
	ModelMaps        map[string]interface{} `gorm:"column:model_maps;serializer:json"`
	ParamOverride    map[string]interface{} `gorm:"column:param_override;serializer:json"`
	HeaderOverride   map[string]interface{} `gorm:"column:header_override;serializer:json"`
	ValidationReport map[string]interface{} `gorm:"column:validation_report;serializer:json"`
	Tags             []string               `gorm:"column:tags;serializer:json"`
	Description      string                 `gorm:"column:description"`
	Priority         int                    `gorm:"column:priority"`
	Weight           int                    `gorm:"column:weight"`
	IsEnabled        bool                   `gorm:"column:is_enabled"`
	IsOfficial       bool                   `gorm:"column:is_official"`
	AutoBan          bool                   `gorm:"column:auto_ban"`
	SyncMode         string                 `gorm:"column:sync_mode"`
	LastSyncedAt     *time.Time             `gorm:"column:last_synced_at"`
	Balance          float64                `gorm:"column:balance;type:numeric"`
	Currency         string                 `gorm:"column:currency"`
	CreatedAt        time.Time              `gorm:"column:created_at"`
	UpdatedAt        time.Time              `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt         `gorm:"column:deleted_at;index"`
}

func (LocalLLMRoute) TableName() string { return "llm_routes" }

type captureImageAdapter struct {
	lastImageReq *adapter.ImageRequest
}

func (a *captureImageAdapter) Name() string { return "capture-image" }
func (a *captureImageAdapter) ChatCompletion(ctx context.Context, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}
func (a *captureImageAdapter) ChatCompletionStream(ctx context.Context, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}
func (a *captureImageAdapter) CreateEmbeddings(ctx context.Context, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (a *captureImageAdapter) CreateImage(ctx context.Context, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	copied := *req
	a.lastImageReq = &copied
	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data: []adapter.ImageItem{
			{URL: "https://example.com/generated.png"},
		},
	}, nil
}
func (a *captureImageAdapter) CreateResponse(ctx context.Context, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}
func (a *captureImageAdapter) Rerank(ctx context.Context, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}
func (a *captureImageAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	return nil, nil
}
func (a *captureImageAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, nil
}
func (a *captureImageAdapter) ValidateConfig(config *adapter.AdapterConfig) error { return nil }
func (a *captureImageAdapter) GetProviderInfo() *adapter.ProviderInfo             { return nil }

type captureAdapterFactory struct {
	adapter adapter.LLMProviderAdapter
}

func (f *captureAdapterFactory) CreateAdapter(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
	return f.adapter, nil
}

type noopBillingProvider struct{}

func (noopBillingProvider) PreDeduct(ctx context.Context, bc *BillingContext) error { return nil }
func (noopBillingProvider) Settle(ctx context.Context, bc *BillingContext) error    { return nil }
func (noopBillingProvider) CalculateCreditsFromTokens(promptTokens, completionTokens int, modelID uuid.UUID) (int64, int64, int64, error) {
	return 0, 0, 0, nil
}
func (noopBillingProvider) CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error) {
	return 1, nil
}
func (noopBillingProvider) CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error) {
	return true, nil
}
func (noopBillingProvider) CheckPrivateChannelBalance(ctx context.Context, organizationID uuid.UUID, channelID uuid.UUID, estimatedCredits int64) (bool, error) {
	return true, nil
}

type captureBillingProvider struct {
	checkBalanceResult bool
	preDeductCalls     int
	settleCalls        int
	lastPreDeduct      *BillingContext
	lastSettle         *BillingContext
}

func (c *captureBillingProvider) PreDeduct(ctx context.Context, bc *BillingContext) error {
	c.preDeductCalls++
	copied := *bc
	c.lastPreDeduct = &copied
	return nil
}

func (c *captureBillingProvider) Settle(ctx context.Context, bc *BillingContext) error {
	c.settleCalls++
	copied := *bc
	c.lastSettle = &copied
	return nil
}

func (c *captureBillingProvider) CalculateCreditsFromTokens(promptTokens, completionTokens int, modelID uuid.UUID) (int64, int64, int64, error) {
	return 0, 0, 0, nil
}

func (c *captureBillingProvider) CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error) {
	return 0, nil
}

func (c *captureBillingProvider) CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error) {
	return c.checkBalanceResult, nil
}

func (c *captureBillingProvider) CheckPrivateChannelBalance(ctx context.Context, organizationID uuid.UUID, channelID uuid.UUID, estimatedCredits int64) (bool, error) {
	return c.checkBalanceResult, nil
}

type sequencePricingEngine struct {
	imageQuotes []PricingQuote
	imageCalls  int
}

func (s *sequencePricingEngine) QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error) {
	return PricingQuote{}, nil
}

func (s *sequencePricingEngine) QuoteImage(ctx context.Context, model PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error) {
	s.imageCalls++
	if len(s.imageQuotes) == 0 {
		return PricingQuote{}, nil
	}
	idx := s.imageCalls - 1
	if idx >= len(s.imageQuotes) {
		idx = len(s.imageQuotes) - 1
	}
	return s.imageQuotes[idx], nil
}

func newImageServiceTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&LocalOrganization{},
		&LocalWorkspace{},
		&LocalMember{},
		&LocalOfficialModelSnapshot{},
		&LocalLLMProvider{},
		&LocalLLMCredential{},
		&LocalLLMRoute{},
	)
	require.NoError(t, err)

	return db
}

func seedPrivateImageRoute(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	return seedPrivateImageRouteWithModel(
		t,
		db,
		"qwen",
		"alibaba",
		"qwen-image-2.0",
		"https://dashscope.aliyuncs.com/api/v1",
	)
}

func seedPrivateImageRouteWithModel(t *testing.T, db *gorm.DB, routeProvider, credentialProvider, routeModel, apiBaseURL string) uuid.UUID {
	t.Helper()

	orgID := uuid.New()
	ownerID := uuid.New()
	credentialID := uuid.New().String()
	now := time.Now()

	require.NoError(t, db.Create(&LocalOrganization{
		ID:        orgID.String(),
		Name:      "Test Org",
		CreatedAt: now,
		UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&LocalMember{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		AccountID:      ownerID.String(),
		Role:           "owner",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&LocalLLMProvider{
		ID:           uuid.New().String(),
		Provider:     routeProvider,
		ProviderName: routeProvider,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error)
	if credentialProvider != routeProvider {
		require.NoError(t, db.Create(&LocalLLMProvider{
			ID:           uuid.New().String(),
			Provider:     credentialProvider,
			ProviderName: credentialProvider,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}).Error)
	}
	if routeProvider == "siliconflow" {
		require.NoError(t, db.Create(&LocalLLMProvider{
			ID:           uuid.New().String(),
			Provider:     "deepseek",
			ProviderName: "deepseek",
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}).Error)
	}
	require.NoError(t, db.Create(&LocalLLMCredential{
		ID:               credentialID,
		OrganizationID:   orgID.String(),
		Name:             "Qwen Image Credential",
		ChannelProvider:  credentialProvider,
		APIKeyCiphertext: "ciphertext",
		APIBaseURL:       apiBaseURL,
		IsActive:         true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}).Error)

	require.NoError(t, db.Create(&LocalLLMRoute{
		ID:              uuid.New().String(),
		OrganizationID:  orgID.String(),
		Type:            "PRIVATE",
		CredentialID:    &credentialID,
		Name:            "Qwen Image Route",
		Models:          []string{routeModel},
		ChannelProvider: routeProvider,
		APIBaseURL:      apiBaseURL,
		IsEnabled:       true,
		Priority:        100,
		Weight:          1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	return orgID
}

func TestCreateImage_PreservesFullModelNameForRoutingAndUpstream(t *testing.T) {
	db := newImageServiceTestDB(t, "file:image-service-test-full-model?mode=memory&cache=shared")
	orgID := seedPrivateImageRouteWithModel(
		t,
		db,
		"siliconflow",
		"siliconflow",
		"Qwen/Qwen-Image",
		"https://api.siliconflow.cn/v1",
	)

	captureAdapter := &captureImageAdapter{}
	billingProvider := noopBillingProvider{}
	svc := &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: &captureAdapterFactory{adapter: captureAdapter},
		channelRouter:  NewChannelRouter(db, stubCryptoService{}, nil),
		billing:        billingProvider,
		localBilling:   billingProvider,
		healthTracker:  NewChannelHealthTracker(nil),
	}

	n := 1
	req := &adapter.ImageRequest{
		Model:  "Qwen/Qwen-Image",
		Prompt: "draw a flower",
		Size:   "1024x1024",
		N:      &n,
	}

	resp, err := svc.CreateImage(context.Background(), &apikeymodel.TenantAPIKey{
		ID:             "test-api-key",
		OrganizationID: orgID.String(),
		Status:         "active",
	}, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, captureAdapter.lastImageReq)
	assert.Equal(t, "Qwen/Qwen-Image", captureAdapter.lastImageReq.Model)
}

func TestCreateImageWithAppContext_OrganizationSubjectOverridesWorkspace(t *testing.T) {
	db := newImageServiceTestDB(t, "file:image-service-test-org-subject?mode=memory&cache=shared")
	orgID := seedPrivateImageRoute(t, db)

	captureAdapter := &captureImageAdapter{}
	billingProvider := &captureBillingProvider{checkBalanceResult: true}
	svc := &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: &captureAdapterFactory{adapter: captureAdapter},
		channelRouter:  NewChannelRouter(db, stubCryptoService{}, nil),
		billing:        billingProvider,
		localBilling:   billingProvider,
		healthTracker:  NewChannelHealthTracker(nil),
	}

	appID := uuid.New()
	accountID := uuid.New()
	appType := "agent"
	workspaceID := uuid.New().String()
	subjectType := quotaSubjectTypeOrganization
	n := 1

	_, err := svc.CreateImageWithAppContext(context.Background(), &apikeymodel.TenantAPIKey{
		ID:             "test-api-key",
		OrganizationID: orgID.String(),
		Status:         "active",
	}, &AppContext{
		AppID:              &appID,
		AppType:            &appType,
		AccountID:          &accountID,
		WorkspaceID:        &workspaceID,
		BillingSubjectType: &subjectType,
	}, &adapter.ImageRequest{
		Model:  "qwen-image-2.0",
		Prompt: "draw a flower",
		Size:   "1024x1024",
		N:      &n,
	})
	require.NoError(t, err)
	require.NotNil(t, billingProvider.lastPreDeduct)
	assert.Equal(t, quotaSubjectTypeOrganization, billingProvider.lastPreDeduct.QuotaSubjectType)
	assert.Equal(t, orgID.String(), billingProvider.lastPreDeduct.QuotaSubjectID)
	assert.Equal(t, orgID.String(), billingProvider.lastPreDeduct.OrganizationID)
}

func TestCreateImage_RequotesActualPriceBeforeSettle(t *testing.T) {
	db := newImageServiceTestDB(t, "file:image-service-test-actual-quote?mode=memory&cache=shared")
	orgID := seedPrivateImageRoute(t, db)

	captureAdapter := &captureImageAdapter{}
	billingProvider := &captureBillingProvider{checkBalanceResult: true}
	engine := &sequencePricingEngine{
		imageQuotes: []PricingQuote{
			{
				OutputUSD:     decimal.RequireFromString("0.10"),
				TotalUSD:      decimal.RequireFromString("0.10"),
				OutputCredits: 100,
				TotalCredits:  100,
				Source:        PricingSourceUSDPrice,
			},
			{
				OutputUSD:     decimal.RequireFromString("0.16"),
				TotalUSD:      decimal.RequireFromString("0.16"),
				OutputCredits: 160,
				TotalCredits:  160,
				Source:        PricingSourceUSDPrice,
			},
		},
	}

	svc := &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: &captureAdapterFactory{adapter: captureAdapter},
		channelRouter:  NewChannelRouter(db, stubCryptoService{}, nil),
		billing:        billingProvider,
		localBilling:   billingProvider,
		healthTracker:  NewChannelHealthTracker(nil),
		pricingEngine:  engine,
	}

	n := 1
	_, err := svc.CreateImage(context.Background(), &apikeymodel.TenantAPIKey{
		ID:             "test-api-key",
		OrganizationID: orgID.String(),
		Status:         "active",
	}, &adapter.ImageRequest{
		Model:  "qwen-image-2.0",
		Prompt: "draw a flower",
		Size:   "1024x1024",
		N:      &n,
	})
	require.NoError(t, err)
	require.NotNil(t, billingProvider.lastSettle)
	assert.Equal(t, 2, engine.imageCalls)
	assert.Equal(t, int64(160), billingProvider.lastSettle.ActualCredits)
	assert.True(t, billingProvider.lastSettle.OutputCost.Equal(decimal.NewFromInt(160)))
	assert.True(t, billingProvider.lastSettle.TotalCost.Equal(decimal.NewFromInt(160)))
	assert.True(t, billingProvider.lastSettle.OutputUSD.Equal(decimal.RequireFromString("0.16")))
	assert.True(t, billingProvider.lastSettle.TotalUSD.Equal(decimal.RequireFromString("0.16")))
}
