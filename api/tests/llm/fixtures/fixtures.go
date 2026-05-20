package fixtures

import (
	"time"

	"github.com/google/uuid"
)

// Test constants
var (
	TestTenantID      = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	TestAPIKey        = "sk-zgi-test-key-12345678901234567890"
	TestAPIKeyInvalid = "sk-invalid-key"
	TestAPIKeyExpired = "sk-expired-key"
	TestRouteID       = uuid.MustParse("00000000-0000-0000-0000-000000000010")
)

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// TestChatCompletionRequests returns test chat completion requests
func TestChatCompletionRequests() map[string]ChatCompletionRequest {
	return map[string]ChatCompletionRequest{
		"simple": {
			Model: "gpt-4o-test",
			Messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
		},
		"invalid_model": {
			Model: "non-existent-model",
			Messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
		},
	}
}

// APIKeyFixture represents an API key test fixture
type APIKeyFixture struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	Key                string
	Name               string
	Status             string
	QuotaLimit         int64
	UsedQuota          int64
	ModelLimitsEnabled bool
	ModelLimits        []string
	ExpiresAt          *time.Time
}

// TestAPIKeys returns test API key fixtures
func TestAPIKeys() []APIKeyFixture {
	return []APIKeyFixture{
		{
			ID:         uuid.MustParse("00000000-0000-0000-0000-000000000101"),
			TenantID:   TestTenantID,
			Key:        TestAPIKey,
			Name:       "Test Key Active",
			Status:     "active",
			QuotaLimit: 1000000,
			UsedQuota:  0,
		},
		{
			ID:         uuid.MustParse("00000000-0000-0000-0000-000000000102"),
			TenantID:   TestTenantID,
			Key:        TestAPIKeyExpired,
			Name:       "Test Key Expired",
			Status:     "expired",
			QuotaLimit: 1000000,
			UsedQuota:  0,
		},
		{
			ID:         uuid.MustParse("00000000-0000-0000-0000-000000000103"),
			TenantID:   TestTenantID,
			Key:        "sk-zgi-exhausted-key-12345678901234",
			Name:       "Test Key Exhausted",
			Status:     "active",
			QuotaLimit: 100,
			UsedQuota:  100,
		},
		{
			ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000104"),
			TenantID:           TestTenantID,
			Key:                "sk-zgi-limited-key-12345678901234",
			Name:               "Test Key Limited",
			Status:             "active",
			QuotaLimit:         1000000,
			UsedQuota:          0,
			ModelLimitsEnabled: true,
			ModelLimits:        []string{"gpt-4o-test"},
		},
	}
}

// RouteFixture represents a route test fixture
type RouteFixture struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	Name                string
	MatchModels         string
	RouteType           string
	CredentialIDs       string
	LoadBalanceStrategy string
	Priority            int
	Weight              int
	IsOfficial          bool
	IsEnabled           bool
}

// CredentialFixture represents a credential test fixture
type CredentialFixture struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Name         string
	ProviderName string
	APIKey       string
	BaseURL      string
	IsActive     bool
}
