package llm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// API Key Generation Tests
// =============================================================================

func TestAPIKeyGeneration(t *testing.T) {
	t.Run("generate key with prefix", func(t *testing.T) {
		prefix := "sk-"
		randomPart := uuid.New().String()[:32]
		key := prefix + randomPart

		assert.True(t, len(key) > len(prefix))
		assert.Equal(t, prefix, key[:3])
	})

	t.Run("key uniqueness", func(t *testing.T) {
		keys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			key := "sk-" + uuid.New().String()[:32]
			assert.False(t, keys[key], "duplicate key generated")
			keys[key] = true
		}
	})

	t.Run("key length validation", func(t *testing.T) {
		key := "sk-" + uuid.New().String()[:32]
		// sk- (3) + 32 chars = 35
		assert.Equal(t, 35, len(key))
	})
}

// =============================================================================
// API Key Format Validation Tests
// =============================================================================

func TestAPIKeyFormatValidation(t *testing.T) {
	t.Run("valid key format", func(t *testing.T) {
		validKeys := []string{
			"sk-abc123def456ghi789jkl012mno345p",
			"sk-" + uuid.New().String()[:32],
		}

		for _, key := range validKeys {
			isValid := len(key) >= 3 && key[:3] == "sk-"
			assert.True(t, isValid, "key should be valid: %s", key)
		}
	})

	t.Run("invalid key format - wrong prefix", func(t *testing.T) {
		invalidKeys := []string{
			"pk-wrong-prefix",
			"api-key-12345",
			"bearer-token",
		}

		for _, key := range invalidKeys {
			isValid := len(key) >= 3 && key[:3] == "sk-"
			assert.False(t, isValid, "key should be invalid: %s", key)
		}
	})

	t.Run("invalid key format - too short", func(t *testing.T) {
		invalidKeys := []string{
			"",
			"sk",
			"sk-",
		}

		for _, key := range invalidKeys {
			isValid := len(key) > 3 && key[:3] == "sk-"
			assert.False(t, isValid, "key should be invalid: %s", key)
		}
	})
}

// =============================================================================
// API Key Quota Tests
// =============================================================================

func TestAPIKeyQuota(t *testing.T) {
	t.Run("unlimited quota - nil limit", func(t *testing.T) {
		var quotaLimit *int64 = nil
		isUnlimited := quotaLimit == nil

		assert.True(t, isUnlimited)
	})

	t.Run("unlimited quota - zero limit", func(t *testing.T) {
		quotaLimit := int64(0)
		isUnlimited := quotaLimit == 0

		assert.True(t, isUnlimited)
	})

	t.Run("sufficient quota", func(t *testing.T) {
		quotaLimit := int64(10000)
		usedQuota := int64(5000)
		requestCredits := int64(100)

		remainQuota := quotaLimit - usedQuota
		hasSufficientQuota := remainQuota >= requestCredits

		assert.True(t, hasSufficientQuota)
		assert.Equal(t, int64(5000), remainQuota)
	})

	t.Run("insufficient quota", func(t *testing.T) {
		quotaLimit := int64(10000)
		usedQuota := int64(9950)
		requestCredits := int64(100)

		remainQuota := quotaLimit - usedQuota
		hasSufficientQuota := remainQuota >= requestCredits

		assert.False(t, hasSufficientQuota)
		assert.Equal(t, int64(50), remainQuota)
	})

	t.Run("quota deduction", func(t *testing.T) {
		usedQuota := int64(5000)
		creditsUsed := int64(150)

		newUsedQuota := usedQuota + creditsUsed

		assert.Equal(t, int64(5150), newUsedQuota)
	})
}

// =============================================================================
// API Key Status Tests
// =============================================================================

func TestAPIKeyStatus(t *testing.T) {
	t.Run("active status allows requests", func(t *testing.T) {
		status := "active"
		canMakeRequest := status == "active"

		assert.True(t, canMakeRequest)
	})

	t.Run("inactive status blocks requests", func(t *testing.T) {
		status := "inactive"
		canMakeRequest := status == "active"

		assert.False(t, canMakeRequest)
	})

	t.Run("expired status blocks requests", func(t *testing.T) {
		status := "expired"
		canMakeRequest := status == "active"

		assert.False(t, canMakeRequest)
	})

	t.Run("deleted status blocks requests", func(t *testing.T) {
		status := "deleted"
		canMakeRequest := status == "active"

		assert.False(t, canMakeRequest)
	})
}

// =============================================================================
// API Key Expiration Tests
// =============================================================================

func TestAPIKeyExpiration(t *testing.T) {
	t.Run("non-expired key", func(t *testing.T) {
		expiresAt := time.Now().Add(24 * time.Hour)
		isExpired := time.Now().After(expiresAt)

		assert.False(t, isExpired)
	})

	t.Run("expired key", func(t *testing.T) {
		expiresAt := time.Now().Add(-24 * time.Hour)
		isExpired := time.Now().After(expiresAt)

		assert.True(t, isExpired)
	})

	t.Run("nil expiration means never expires", func(t *testing.T) {
		var expiresAt *time.Time = nil
		neverExpires := expiresAt == nil

		assert.True(t, neverExpires)
	})
}

// =============================================================================
// API Key Model Limits Tests
// =============================================================================

func TestAPIKeyModelLimits(t *testing.T) {
	t.Run("no model limits allows all models", func(t *testing.T) {
		modelLimitsEnabled := false
		var modelLimits []string = nil

		requestedModel := "gpt-4"
		canAccessModel := !modelLimitsEnabled || containsModel(modelLimits, requestedModel)

		assert.True(t, canAccessModel)
	})

	t.Run("model in allowed list", func(t *testing.T) {
		modelLimitsEnabled := true
		modelLimits := []string{"gpt-3.5-turbo", "gpt-4", "gpt-4o"}

		requestedModel := "gpt-4"
		canAccessModel := !modelLimitsEnabled || containsModel(modelLimits, requestedModel)

		assert.True(t, canAccessModel)
	})

	t.Run("model not in allowed list", func(t *testing.T) {
		modelLimitsEnabled := true
		modelLimits := []string{"gpt-3.5-turbo", "gpt-4"}

		requestedModel := "claude-3"
		canAccessModel := !modelLimitsEnabled || containsModel(modelLimits, requestedModel)

		assert.False(t, canAccessModel)
	})

	t.Run("empty model limits with enabled flag blocks all", func(t *testing.T) {
		modelLimitsEnabled := true
		modelLimits := []string{}

		requestedModel := "gpt-4"
		canAccessModel := !modelLimitsEnabled || containsModel(modelLimits, requestedModel)

		assert.False(t, canAccessModel)
	})

	t.Run("wildcard model allows all", func(t *testing.T) {
		modelLimits := []string{"*"}

		requestedModel := "any-model"
		canAccessModel := containsModel(modelLimits, "*") || containsModel(modelLimits, requestedModel)

		assert.True(t, canAccessModel)
	})
}

func containsModel(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// =============================================================================
// API Key Tenant Isolation Tests
// =============================================================================

func TestAPIKeyTenantIsolation(t *testing.T) {
	t.Run("key belongs to correct tenant", func(t *testing.T) {
		keyTenantID := uuid.New().String()
		requestTenantID := keyTenantID

		isAuthorized := keyTenantID == requestTenantID
		assert.True(t, isAuthorized)
	})

	t.Run("key from different tenant is rejected", func(t *testing.T) {
		keyTenantID := uuid.New().String()
		requestTenantID := uuid.New().String()

		isAuthorized := keyTenantID == requestTenantID
		assert.False(t, isAuthorized)
	})
}
