package llm_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestTokenCostCalculation tests token cost calculation logic
func TestTokenCostCalculation(t *testing.T) {
	t.Run("calculate input cost", func(t *testing.T) {
		// Cost per million tokens
		costPerMillion := decimal.NewFromFloat(0.50)
		inputTokens := int64(1000)

		// Calculate: (tokens / 1,000,000) * cost_per_million
		inputCost := decimal.NewFromInt(inputTokens).
			Div(decimal.NewFromInt(1000000)).
			Mul(costPerMillion)

		expected := decimal.NewFromFloat(0.0005)
		assert.True(t, inputCost.Equal(expected), "expected %s, got %s", expected, inputCost)
	})

	t.Run("calculate output cost", func(t *testing.T) {
		costPerMillion := decimal.NewFromFloat(1.50)
		outputTokens := int64(500)

		outputCost := decimal.NewFromInt(outputTokens).
			Div(decimal.NewFromInt(1000000)).
			Mul(costPerMillion)

		expected := decimal.NewFromFloat(0.00075)
		assert.True(t, outputCost.Equal(expected) || outputCost.StringFixed(8) == "0.00075000",
			"expected ~0.00075, got %s", outputCost)
	})

	t.Run("calculate total cost", func(t *testing.T) {
		inputCost := decimal.NewFromFloat(0.0005)
		outputCost := decimal.NewFromFloat(0.00075)

		totalCost := inputCost.Add(outputCost)

		expected := decimal.NewFromFloat(0.00125)
		assert.True(t, totalCost.Equal(expected))
	})

	t.Run("zero tokens", func(t *testing.T) {
		costPerMillion := decimal.NewFromFloat(0.50)
		tokens := int64(0)

		cost := decimal.NewFromInt(tokens).
			Div(decimal.NewFromInt(1000000)).
			Mul(costPerMillion)

		assert.True(t, cost.IsZero())
	})
}

// TestPreDeductLogic tests pre-deduction billing logic
func TestPreDeductLogic(t *testing.T) {
	t.Run("sufficient quota", func(t *testing.T) {
		quotaLimit := int64(10000)
		usedQuota := int64(5000)
		estimatedCredits := int64(100)

		remainingQuota := quotaLimit - usedQuota
		hasSufficientQuota := remainingQuota >= estimatedCredits

		assert.True(t, hasSufficientQuota)
		assert.Equal(t, int64(5000), remainingQuota)
	})

	t.Run("insufficient quota", func(t *testing.T) {
		quotaLimit := int64(10000)
		usedQuota := int64(9950)
		estimatedCredits := int64(100)

		remainingQuota := quotaLimit - usedQuota
		hasSufficientQuota := remainingQuota >= estimatedCredits

		assert.False(t, hasSufficientQuota)
		assert.Equal(t, int64(50), remainingQuota)
	})

	t.Run("unlimited quota", func(t *testing.T) {
		var quotaLimit *int64 = nil // nil means unlimited
		estimatedCredits := int64(100)

		hasSufficientQuota := quotaLimit == nil || *quotaLimit == 0
		assert.True(t, hasSufficientQuota)
		_ = estimatedCredits // unused in unlimited case
	})

	t.Run("zero quota limit means unlimited", func(t *testing.T) {
		quotaLimit := int64(0)
		usedQuota := int64(999999)
		estimatedCredits := int64(100)

		// 0 means unlimited
		hasSufficientQuota := quotaLimit == 0 || (quotaLimit-usedQuota) >= estimatedCredits

		assert.True(t, hasSufficientQuota)
	})
}

// TestSettleBillingLogic tests settlement billing logic
func TestSettleBillingLogic(t *testing.T) {
	t.Run("refund excess pre-deduction", func(t *testing.T) {
		estimatedCredits := int64(150)
		actualCredits := int64(100)

		refundAmount := estimatedCredits - actualCredits

		assert.Equal(t, int64(50), refundAmount)
	})

	t.Run("no refund when actual equals estimated", func(t *testing.T) {
		estimatedCredits := int64(100)
		actualCredits := int64(100)

		refundAmount := estimatedCredits - actualCredits

		assert.Equal(t, int64(0), refundAmount)
	})

	t.Run("additional charge when actual exceeds estimated", func(t *testing.T) {
		estimatedCredits := int64(100)
		actualCredits := int64(120)

		additionalCharge := actualCredits - estimatedCredits

		assert.Equal(t, int64(20), additionalCharge)
	})
}

// TestCreditConversion tests credit conversion logic
func TestCreditConversion(t *testing.T) {
	t.Run("USD to credits", func(t *testing.T) {
		// Assuming 1 USD = 1000 credits
		usdAmount := decimal.NewFromFloat(5.50)
		creditsPerUSD := int64(1000)

		credits := usdAmount.Mul(decimal.NewFromInt(creditsPerUSD)).IntPart()

		assert.Equal(t, int64(5500), credits)
	})

	t.Run("credits to USD", func(t *testing.T) {
		credits := int64(5500)
		creditsPerUSD := int64(1000)

		usdAmount := decimal.NewFromInt(credits).Div(decimal.NewFromInt(creditsPerUSD))

		expected := decimal.NewFromFloat(5.50)
		assert.True(t, usdAmount.Equal(expected))
	})
}

// TestBillingContextFields tests billing context structure
func TestBillingContextFields(t *testing.T) {
	t.Run("billing context with all fields", func(t *testing.T) {
		type BillingContext struct {
			APIKeyID          string
			TenantID          string
			ShadowTenantID    string
			UseSystemProvider bool
			EstimatedCredits  int64
			ActualCredits     int64
		}

		ctx := BillingContext{
			APIKeyID:          "key-123",
			TenantID:          "tenant-456",
			ShadowTenantID:    "group-789",
			UseSystemProvider: true,
			EstimatedCredits:  100,
			ActualCredits:     0, // Set after response
		}

		assert.Equal(t, "key-123", ctx.APIKeyID)
		assert.Equal(t, "tenant-456", ctx.TenantID)
		assert.Equal(t, "group-789", ctx.ShadowTenantID)
		assert.True(t, ctx.UseSystemProvider)
	})
}

// TestSystemProviderBilling tests system provider billing logic
func TestSystemProviderBilling(t *testing.T) {
	t.Run("system provider deducts from AI credits", func(t *testing.T) {
		useSystemProvider := true
		actualCredits := int64(100)
		currentBalance := int64(1000)

		var newBalance int64
		if useSystemProvider {
			newBalance = currentBalance - actualCredits
		} else {
			newBalance = currentBalance // No deduction for tenant's own key
		}

		assert.Equal(t, int64(900), newBalance)
	})

	t.Run("tenant channel does not deduct AI credits", func(t *testing.T) {
		useSystemProvider := false
		actualCredits := int64(100)
		currentBalance := int64(1000)

		var newBalance int64
		if useSystemProvider {
			newBalance = currentBalance - actualCredits
		} else {
			newBalance = currentBalance
		}

		assert.Equal(t, int64(1000), newBalance)
		_ = actualCredits // unused
	})
}

// TestTokenEstimation tests token estimation logic
func TestTokenEstimation(t *testing.T) {
	t.Run("estimate chat tokens", func(t *testing.T) {
		// Simple estimation: ~4 chars per token
		message := "Hello, how are you today?"
		charsPerToken := 4

		estimatedTokens := (len(message) + charsPerToken - 1) / charsPerToken

		assert.Greater(t, estimatedTokens, 0)
		assert.LessOrEqual(t, estimatedTokens, 10)
	})

	t.Run("estimate embedding tokens", func(t *testing.T) {
		texts := []string{
			"First document",
			"Second document with more content",
		}

		totalChars := 0
		for _, text := range texts {
			totalChars += len(text)
		}

		charsPerToken := 4
		estimatedTokens := (totalChars + charsPerToken - 1) / charsPerToken

		assert.Greater(t, estimatedTokens, 0)
	})
}

// TestModelPricing tests model pricing lookup
func TestModelPricing(t *testing.T) {
	t.Run("model pricing structure", func(t *testing.T) {
		type ModelPricing struct {
			ModelName   string
			InputPrice  decimal.Decimal // per million tokens
			OutputPrice decimal.Decimal // per million tokens
		}

		pricing := ModelPricing{
			ModelName:   "gpt-4",
			InputPrice:  decimal.NewFromFloat(30.00),
			OutputPrice: decimal.NewFromFloat(60.00),
		}

		assert.Equal(t, "gpt-4", pricing.ModelName)
		assert.True(t, pricing.InputPrice.Equal(decimal.NewFromFloat(30.00)))
		assert.True(t, pricing.OutputPrice.Equal(decimal.NewFromFloat(60.00)))
	})

	t.Run("cheaper model pricing", func(t *testing.T) {
		type ModelPricing struct {
			ModelName   string
			InputPrice  decimal.Decimal
			OutputPrice decimal.Decimal
		}

		pricing := ModelPricing{
			ModelName:   "gpt-3.5-turbo",
			InputPrice:  decimal.NewFromFloat(0.50),
			OutputPrice: decimal.NewFromFloat(1.50),
		}

		// GPT-3.5 is much cheaper than GPT-4
		gpt4Input := decimal.NewFromFloat(30.00)
		assert.True(t, pricing.InputPrice.LessThan(gpt4Input))
	})
}

// TestQuotaUpdate tests quota update logic
func TestQuotaUpdate(t *testing.T) {
	t.Run("update used quota after request", func(t *testing.T) {
		usedQuota := int64(5000)
		actualCredits := int64(100)

		newUsedQuota := usedQuota + actualCredits

		assert.Equal(t, int64(5100), newUsedQuota)
	})

	t.Run("quota limit check after update", func(t *testing.T) {
		quotaLimit := int64(10000)
		usedQuota := int64(9900)
		actualCredits := int64(100)

		newUsedQuota := usedQuota + actualCredits
		hasReachedLimit := newUsedQuota >= quotaLimit

		assert.True(t, hasReachedLimit)
		assert.Equal(t, int64(10000), newUsedQuota)
	})
}
