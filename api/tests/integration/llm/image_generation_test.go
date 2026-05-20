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

	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/internal/modules/llm/shared"

	"gorm.io/gorm"
)

// MockAdapter implements adapter.LLMProviderAdapter for testing
type MockAdapter struct {
}

// Implement required methods for MockAdapter
func (m *MockAdapter) Name() string { return "dashscope" }
func (m *MockAdapter) ChatCompletion(ctx context.Context, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}
func (m *MockAdapter) ChatCompletionStream(ctx context.Context, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}
func (m *MockAdapter) CreateEmbeddings(ctx context.Context, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *MockAdapter) CreateImage(ctx context.Context, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data: []adapter.ImageItem{
			{
				URL: "https://fake-image-url.com/image.png",
			},
		},
	}, nil
}
func (m *MockAdapter) CreateResponse(ctx context.Context, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *MockAdapter) Rerank(ctx context.Context, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}
func (m *MockAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	return nil, nil
}
func (m *MockAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, nil
}
func (m *MockAdapter) ValidateConfig(config *adapter.AdapterConfig) error { return nil }
func (m *MockAdapter) GetProviderInfo() *adapter.ProviderInfo             { return nil }

// --- Local Models for SQLite Migration (avoiding Postgres-specific types) ---

type LocalOrganization struct {
	ID        string `gorm:"primaryKey"`
	Name      string
	CreatedAt time.Time
}

func (LocalOrganization) TableName() string { return "organizations" }

type LocalWorkspace struct {
	ID             string `gorm:"primaryKey"`
	OrganizationID string
}

func (LocalWorkspace) TableName() string { return "workspaces" }

type LocalMember struct {
	ID             string `gorm:"primaryKey"`
	OrganizationID string
	AccountID      string
	Role           string
}

func (LocalMember) TableName() string { return "members" }

type LocalLLMProvider struct {
	ID           string `gorm:"primaryKey"`
	Provider     string `gorm:"uniqueIndex"`
	ProviderName string
	IsActive     bool `gorm:"default:true"`
	DeletedAt    gorm.DeletedAt
	// Add other fields to satisfy GORM select * if needed, but usually joins select specific fields or * from main table
	// ChannelRouter joins llm_providers but mostly checks is_active.
}

func (LocalLLMProvider) TableName() string { return "llm_providers" }

type LocalOfficialModelSnapshot struct {
	SourceKey          string     `gorm:"column:source_key;primaryKey"`
	EffectiveModels    []byte     `gorm:"column:effective_models;type:text"`
	LatestModels       []byte     `gorm:"column:latest_models;type:text"`
	PreviousModels     []byte     `gorm:"column:previous_models;type:text"`
	LatestEventVersion int64      `gorm:"column:latest_event_version;not null;default:0"`
	LatestSyncedAt     *time.Time `gorm:"column:latest_synced_at"`
	EffectiveUpdatedAt *time.Time `gorm:"column:effective_updated_at"`
	LastCheckStatus    string     `gorm:"column:last_check_status;not null;default:'accepted'"`
	LastRejectReason   string     `gorm:"column:last_reject_reason;type:text"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;not null"`
}

func (LocalOfficialModelSnapshot) TableName() string { return "llm_official_model_snapshots" }

type LocalLLMRoute struct {
	ID               string `gorm:"primaryKey"`
	OrganizationID   string
	Type             string
	CredentialID     *string `gorm:"column:user_credential_id"`
	Name             string
	Models           []byte `gorm:"type:text"` // SQLite supports text for JSON
	APIBaseURL       string
	Provider         string
	ModelMaps        []byte `gorm:"type:text"`
	ParamOverride    []byte `gorm:"type:text"`
	HeaderOverride   []byte `gorm:"type:text"`
	ValidationReport []byte `gorm:"type:text"`
	Tags             []byte `gorm:"type:text"`
	Description      string
	Priority         int
	Weight           int
	IsEnabled        bool
	IsOfficial       bool
	AutoBan          bool
	SyncMode         string
	LastSyncedAt     *time.Time
	Balance          float64
	Currency         string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
}

func (LocalLLMRoute) TableName() string { return "llm_routes" }

type LocalLLMCredential struct {
	ID             string `gorm:"primaryKey"`
	OrganizationID string
	Name           string
	Provider       string
	APIKey         string `gorm:"column:api_key_ciphertext"`
	BaseURL        string `gorm:"column:api_base_url"`
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt
}

func (LocalLLMCredential) TableName() string { return "llm_credentials" }

type LocalLLMModel struct {
	ID uuid.UUID `gorm:"type:text;primaryKey"` // Use UUID type in struct but text in DB (SQLite handles it via driver or text)
	// Actually, creating table with `id text` works. GORM might complain if struct has uuid.UUID but I can use string in local struct.
	// But wait, the REAL struct uses uuid.UUID.
	// If I create table with `id text`, and REAL struct has `id uuid.UUID`, GORM usually handles it.

	// I will use string for ID in local struct to ensure `text` column type.

	Provider      string
	Model         string `gorm:"column:name"`
	ModelName     string `gorm:"column:display_name"`
	Family        string
	FamilyName    string
	ParentID      *string
	FamilyDefault bool
	Status        string
	Tagline       string
	Description   string
	IsFlagship    bool
	IsFeatured    bool
	IsNew         bool
	AccessType    string
	Currency      string
	Type          string
	UseCases      []byte `gorm:"type:text"` // Array in Postgres, Text (JSON) in SQLite? Or just text.
	// GORM `text[]` support in SQLite is tricky.
	// I should probably use `string` and store JSON or comma separated if needed.
	// But `LLMModel` uses `StringArray` (custom type?).
	// `StringArray` usually implements Scanner/Valuer.
	// If I use `[]byte` (blob/text) here, `AutoMigrate` creates a column.
	// When `LLMModel` (real struct) tries to read, it expects `text[]`.
	// SQLite doesn't support arrays.
	// GORM's postgres driver handles `text[]`.
	// `StringArray` likely handles JSON or PG array format.
	// If I create it as `text` in SQLite, and `StringArray` tries to scan it...
	// If `StringArray` supports scanning from string/bytes, it might work.

	SupportsReasoning        bool   `gorm:"column:reasoning"`
	SupportsToolCall         bool   `gorm:"column:function_calling"`
	SupportsStructuredOutput bool   `gorm:"column:structured_output"`
	SupportsTemperature      bool   `gorm:"column:temperature"`
	SupportsTopP             bool   `gorm:"column:top_p"`
	SupportsPresencePenalty  bool   `gorm:"column:presence_penalty"`
	SupportsFrequencyPenalty bool   `gorm:"column:frequency_penalty"`
	SupportsLogitBias        bool   `gorm:"column:logit_bias"`
	SupportsSeed             bool   `gorm:"column:seed"`
	SupportsStop             bool   `gorm:"column:stop"`
	MaxStopSequences         int    `gorm:"column:max_stop_sequences"`
	SupportsVision           bool   `gorm:"column:vision"`
	SupportsAudio            bool   `gorm:"column:audio"`
	SupportsJsonMode         bool   `gorm:"column:json_mode"`
	SupportsStreaming        bool   `gorm:"column:streaming"`
	ChatCompletions          bool   `gorm:"column:chat_completions"`
	Embeddings               bool   `gorm:"column:embeddings"`
	ImageGeneration          bool   `gorm:"column:image_generation"`
	SpeechGeneration         bool   `gorm:"column:speech_generation"`
	Transcription            bool   `gorm:"column:transcription"`
	Translation              bool   `gorm:"column:translation"`
	Moderation               bool   `gorm:"column:moderation"`
	Videos                   bool   `gorm:"column:videos"`
	ImageEdit                bool   `gorm:"column:image_edit"`
	Realtime                 bool   `gorm:"column:realtime"`
	Batch                    bool   `gorm:"column:batch"`
	FineTuning               bool   `gorm:"column:fine_tuning"`
	Assistants               bool   `gorm:"column:assistants"`
	Responses                bool   `gorm:"column:responses"`
	Distillation             bool   `gorm:"column:distillation"`
	SystemPrompt             bool   `gorm:"column:system_prompt"`
	Logprobs                 bool   `gorm:"column:logprobs"`
	WebSearch                bool   `gorm:"column:web_search"`
	FileSearch               bool   `gorm:"column:file_search"`
	CodeInterpreter          bool   `gorm:"column:code_interpreter"`
	ComputerUse              bool   `gorm:"column:computer_use"`
	Mcp                      bool   `gorm:"column:mcp"`
	ParallelToolCalls        bool   `gorm:"column:parallel_tool_calls"`
	ReasoningEffort          bool   `gorm:"column:reasoning_effort"`
	InputModalities          []byte `gorm:"column:input_modalities;type:text"`
	OutputModalities         []byte `gorm:"column:output_modalities;type:text"`
	KnowledgeCutoff          string
	OpenWeights              bool
	ContextWindow            int
	MaxOutputTokens          int
	MaxInputTokens           int
	SupportedParameters      []byte `gorm:"column:supported_parameters;type:text"`
	DefaultParameters        []byte `gorm:"column:default_parameters;type:text"`
	IsModerated              bool
	IsFinetuned              bool
	CostRate                 []byte  `gorm:"type:text"`
	InputPrice               float64 `gorm:"column:input_price"`
	OutputPrice              float64 `gorm:"column:output_price"`
	CachedInputPrice         float64 `gorm:"column:cached_input_price"`
	CostCacheRead            float64
	CostCacheWrite           float64
	CostContextOver200k      []byte `gorm:"column:cost_context_over_200k;type:text"`
	ImagePrices              []byte `gorm:"column:image_prices;type:text"`
	IsActive                 bool
	IsConfigured             bool
	SortOrder                int
	ModelTier                string
	IsRecommended            bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt
}

func (LocalLLMModel) TableName() string { return "llm_models" }

type LocalTenantAPIKey struct {
	ID                 string `gorm:"primaryKey"`
	OrganizationID     string `gorm:"not null;index;column:organization_id"`
	Key                string `gorm:"not null"`
	KeyHash            string `gorm:"uniqueIndex"`
	Name               string `gorm:"not null"`
	Status             string `gorm:"not null;default:'active'"`
	IsInternal         bool   `gorm:"not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt
	UsedQuota          int64 `gorm:"not null;default:0"`
	RemainQuota        int64 `gorm:"not null;default:0"`
	QuotaLimit         *int64
	ModelLimitsEnabled bool    `gorm:"not null;default:false"`
	ModelLimits        *string `gorm:"type:text"`
	AllowIPs           string  `gorm:"type:text;not null;default:''"`
	AccessedAt         *time.Time
	ExpiresAt          *time.Time
}

func (LocalTenantAPIKey) TableName() string { return "llm_organization_api_keys" }

type LocalBillingAttempt struct {
	AttemptID         string     `gorm:"column:attempt_id;primaryKey"`
	RequestID         string     `gorm:"column:request_id;not null;index"`
	OrganizationID    string     `gorm:"column:organization_id;not null;index"`
	Lane              string     `gorm:"column:lane;not null"`
	RouteID           *string    `gorm:"column:route_id"`
	ProviderID        *string    `gorm:"column:provider_id"`
	ModelID           *string    `gorm:"column:model_id"`
	QuotaSubjectType  string     `gorm:"column:quota_subject_type;not null"`
	QuotaSubjectID    string     `gorm:"column:quota_subject_id;not null"`
	Status            string     `gorm:"column:status;not null;index"`
	InvocationResult  *string    `gorm:"column:invocation_result"`
	ErrorCode         *string    `gorm:"column:error_code"`
	ErrorMessage      *string    `gorm:"column:error_message;type:text"`
	ReconcileAttempts int        `gorm:"column:reconcile_attempts;not null;default:0"`
	NextReconcileAt   *time.Time `gorm:"column:next_reconcile_at"`
	LastReconcileAt   *time.Time `gorm:"column:last_reconcile_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
}

func (LocalBillingAttempt) TableName() string { return "billing_attempts" }

type LocalBillingAttemptEntry struct {
	ID             string    `gorm:"column:id;primaryKey"`
	AttemptID      string    `gorm:"column:attempt_id;not null;uniqueIndex:idx_attempt_entry_unique"`
	EntryType      string    `gorm:"column:entry_type;not null;uniqueIndex:idx_attempt_entry_unique"`
	LedgerType     string    `gorm:"column:ledger_type;not null;uniqueIndex:idx_attempt_entry_unique"`
	LedgerRefID    string    `gorm:"column:ledger_ref_id;not null"`
	ReservedAmount int64     `gorm:"column:reserved_amount;not null;default:0"`
	ActualAmount   int64     `gorm:"column:actual_amount;not null;default:0"`
	RefundedAmount int64     `gorm:"column:refunded_amount;not null;default:0"`
	Status         string    `gorm:"column:status;not null;index"`
	ErrorCode      *string   `gorm:"column:error_code"`
	ErrorMessage   *string   `gorm:"column:error_message;type:text"`
	IdempotencyKey *string   `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (LocalBillingAttemptEntry) TableName() string { return "billing_attempt_entries" }

type LocalChannelWallet struct {
	ChannelID      string    `gorm:"column:channel_id;primaryKey"`
	OrganizationID string    `gorm:"column:organization_id;not null;index"`
	Balance        int64     `gorm:"column:balance;not null;default:0"`
	Status         string    `gorm:"column:status;not null;default:'ACTIVE';index"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (LocalChannelWallet) TableName() string { return "channel_wallets" }

type LocalChannelWalletTransaction struct {
	ID            string    `gorm:"column:id;primaryKey"`
	ChannelID     string    `gorm:"column:channel_id;not null;index"`
	AttemptID     *string   `gorm:"column:attempt_id;index"`
	Type          string    `gorm:"column:type;not null"`
	Amount        int64     `gorm:"column:amount;not null"`
	BalanceBefore int64     `gorm:"column:balance_before;not null"`
	BalanceAfter  int64     `gorm:"column:balance_after;not null"`
	Metadata      []byte    `gorm:"column:metadata;type:text"` // JSON in text
	CreatedAt     time.Time `gorm:"column:created_at;not null"`
}

func (LocalChannelWalletTransaction) TableName() string { return "channel_wallet_transactions" }

type LocalGroupAICreditAccount struct {
	ID                  string `gorm:"primaryKey"`
	AccountID           string `gorm:"not null;uniqueIndex:idx_account_group_credit;index:idx_credit_account"`
	GroupID             string `gorm:"not null;uniqueIndex:idx_account_group_credit;index:idx_credit_group"`
	SubscriptionCredits int64  `gorm:"not null;default:0"`
	PurchasedCredits    int64  `gorm:"not null;default:0"`
	TotalEarned         int64  `gorm:"not null;default:0"`
	TotalSpent          int64  `gorm:"not null;default:0"`
	LastResetAt         *time.Time
	NextResetAt         *time.Time
	CreatedAt           time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt           time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (LocalGroupAICreditAccount) TableName() string { return "group_ai_credit_accounts" }

type LocalTransaction struct {
	ID                string  `gorm:"primaryKey"`
	BatchID           string  `gorm:"not null;index:idx_tx_batch"`
	GroupID           string  `gorm:"not null;index:idx_tx_group;index:idx_tx_group_currency"`
	TenantID          *string `gorm:"index:idx_tx_tenant"`
	CurrencyType      string  `gorm:"not null;index:idx_tx_group_currency"`
	Type              string  `gorm:"column:type;index:idx_tx_type"`
	TransactionType   string  `gorm:"not null;index:idx_tx_type"`
	Amount            float64 `gorm:"not null"`
	BalanceBefore     float64 `gorm:"not null"`
	BalanceAfter      float64 `gorm:"not null"`
	Currency          *string
	ReferenceType     *string `gorm:"index:idx_tx_reference"`
	ReferenceID       *string `gorm:"index:idx_tx_reference"`
	Description       *string
	TransactionDetail []byte    `gorm:"type:text"` // JSON
	CreatedAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_tx_created_at"`
}

func (LocalTransaction) TableName() string { return "transactions" }

type LocalWorkspaceQuota struct {
	WorkspaceID    string    `gorm:"column:workspace_id;primaryKey"`
	OrganizationID string    `gorm:"column:organization_id;not null;index"`
	UsedQuota      int64     `gorm:"column:used_quota;not null;default:0"`
	RemainQuota    int64     `gorm:"column:remain_quota;not null;default:0"`
	QuotaLimit     *int64    `gorm:"column:quota_limit"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (LocalWorkspaceQuota) TableName() string { return "llm_workspace_quotas" }

func TestImageGenerationFlow(t *testing.T) {
	// 1. Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations using LOCAL structs
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
	apiKeyID := "sk-test-apikey"

	// Create Organization
	db.Create(&LocalOrganization{
		ID:        orgID.String(),
		Name:      "Test Org",
		CreatedAt: time.Now(),
	})

	// Create Provider
	db.Create(&LocalLLMProvider{
		ID:           uuid.New().String(),
		Provider:     "dashscope",
		ProviderName: "Dashscope",
		IsActive:     true,
	})

	// Create API Key
	db.Create(&LocalTenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: orgID.String(),
		Key:            "test-key-hash",
		Status:         "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		RemainQuota:    1000, // Give some quota just in case
	})

	// Create Model (wanx-v1) with image prices
	modelID := uuid.New()
	imagePrices := []map[string]interface{}{
		{
			"priority": 1,
			"conditions": map[string]interface{}{
				"size": "1024x1024",
			},
			"price": map[string]interface{}{
				"credits": 4, // 4 credits for 1024x1024
			},
		},
	}
	imagePricesJSON, _ := json.Marshal(imagePrices)
	db.Create(&LocalLLMModel{
		ID:          modelID, // UUID
		Model:       "wanx-v1",
		ModelName:   "WanX V1",
		Provider:    "dashscope",
		ImagePrices: imagePricesJSON,
		IsActive:    true,
		CreatedAt:   time.Now(),
	})

	// Create Credential
	cryptoService, _ := shared.DefaultCryptoService()
	encryptedKey, _ := cryptoService.Encrypt("fake-key")

	credID := uuid.New()
	db.Create(&LocalLLMCredential{
		ID:             credID.String(),
		OrganizationID: orgID.String(),
		Name:           "Dashscope Cred",
		Provider:       "dashscope",
		APIKey:         encryptedKey,
		IsActive:       true,
		CreatedAt:      time.Now(),
	})

	// Create Channel (Route)
	modelsJSON, _ := json.Marshal([]string{"wanx-v1"})
	credIDStr := credID.String()
	db.Create(&LocalLLMRoute{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		Name:           "Dashscope Route",
		Provider:       "dashscope",
		Type:           "PRIVATE",
		Models:         modelsJSON,
		IsEnabled:      true,
		Priority:       100,
		CredentialID:   &credIDStr,
		Balance:        100.0,
		CreatedAt:      time.Now(),
	})

	// 3. Mock dashscope adapter
	adapter.GlobalFactory.Register("dashscope", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return &MockAdapter{}, nil
	})

	// 4. Initialize LLMGatewayService
	apiKeyRepo := apikeyrepo.NewAPIKeyRepository(db)

	svc, err := gateway.NewLLMGatewayService(
		db,
		apiKeyRepo,
		adapter.GlobalFactory,
	)
	require.NoError(t, err)

	// Construct apikeymodel.TenantAPIKey to pass to CreateImage
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: orgID.String(),
		Key:            "test-key-hash",
		Status:         "active",
	}

	// 5. Call CreateImage
	n := 1
	req := &adapter.ImageRequest{
		Model:  "wanx-v1",
		Prompt: "A cute cat",
		Size:   "1024x1024",
		N:      &n,
	}

	resp, err := svc.CreateImage(context.Background(), apiKey, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Data)
	assert.Equal(t, "https://fake-image-url.com/image.png", resp.Data[0].URL)

	// 6. Verify Billing
	var attempt LocalBillingAttempt
	err = db.Where("organization_id = ?", orgID.String()).Order("created_at DESC").First(&attempt).Error
	require.NoError(t, err)

	assert.Equal(t, "SETTLED", attempt.Status)
	assert.Equal(t, modelID.String(), *attempt.ModelID)

	// Verify wallet transaction (PreDeduct)
	var walletTx LocalChannelWalletTransaction
	err = db.Order("created_at DESC").First(&walletTx).Error
	require.NoError(t, err)

	// Price 4 credits * N=1 = 4 credits.
	// Transaction amount should be negative for deduction
	assert.Equal(t, int64(-4), walletTx.Amount)
	assert.Equal(t, "prededuct", walletTx.Type)
}
