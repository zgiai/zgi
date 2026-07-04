package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func TestBuildUsageBill_NormalizesTimes(t *testing.T) {
	service := &BillingService{}

	t.Run("clamps settled time to request time when clock moves backward", func(t *testing.T) {
		requestCreatedAt := time.Date(2026, 5, 28, 21, 59, 49, 818000000, time.UTC)
		settledAt := requestCreatedAt.Add(-1 * time.Second)

		bill, err := service.buildUsageBill(testUsageBillContext(requestCreatedAt, settledAt), usageBillStatusSuccess, nil, nil)
		if err != nil {
			t.Fatalf("buildUsageBill returned error: %v", err)
		}

		if !bill.SettledAt.Equal(requestCreatedAt) {
			t.Fatalf("SettledAt=%s, want %s", bill.SettledAt, requestCreatedAt)
		}
		if bill.SettledAt.Before(bill.RequestCreatedAt) {
			t.Fatalf("SettledAt=%s before RequestCreatedAt=%s", bill.SettledAt, bill.RequestCreatedAt)
		}
	})

	t.Run("keeps ordered times and converts them to utc", func(t *testing.T) {
		location := time.FixedZone("CST", 8*60*60)
		requestCreatedAt := time.Date(2026, 5, 28, 21, 59, 49, 818000000, location)
		settledAt := requestCreatedAt.Add(2 * time.Second)

		bill, err := service.buildUsageBill(testUsageBillContext(requestCreatedAt, settledAt), usageBillStatusSuccess, nil, nil)
		if err != nil {
			t.Fatalf("buildUsageBill returned error: %v", err)
		}

		if !bill.RequestCreatedAt.Equal(requestCreatedAt.UTC()) {
			t.Fatalf("RequestCreatedAt=%s, want %s", bill.RequestCreatedAt, requestCreatedAt.UTC())
		}
		if bill.RequestCreatedAt.Location() != time.UTC {
			t.Fatalf("RequestCreatedAt location=%s, want UTC", bill.RequestCreatedAt.Location())
		}
		if !bill.SettledAt.Equal(settledAt.UTC()) {
			t.Fatalf("SettledAt=%s, want %s", bill.SettledAt, settledAt.UTC())
		}
		if bill.SettledAt.Location() != time.UTC {
			t.Fatalf("SettledAt location=%s, want UTC", bill.SettledAt.Location())
		}
	})

	t.Run("fills zero times in valid order", func(t *testing.T) {
		bill, err := service.buildUsageBill(testUsageBillContext(time.Time{}, time.Time{}), usageBillStatusSuccess, nil, nil)
		if err != nil {
			t.Fatalf("buildUsageBill returned error: %v", err)
		}

		if bill.RequestCreatedAt.IsZero() {
			t.Fatal("RequestCreatedAt is zero")
		}
		if bill.SettledAt.IsZero() {
			t.Fatal("SettledAt is zero")
		}
		if bill.SettledAt.Before(bill.RequestCreatedAt) {
			t.Fatalf("SettledAt=%s before RequestCreatedAt=%s", bill.SettledAt, bill.RequestCreatedAt)
		}
	})
}

func TestBuildUsageBill_PartialKeepsRecordedUsageAndPoints(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.Status = billingContextStatusPartial
	bc.PromptTokens = 10
	bc.CompletionTokens = 5
	bc.TotalTokens = 0
	bc.ActualCredits = 7

	bill, err := service.buildUsageBill(bc, usageBillStatusPartial, nil, nil)
	if err != nil {
		t.Fatalf("buildUsageBill returned error: %v", err)
	}
	if bill.PromptTokens != 10 || bill.CompletionTokens != 5 || bill.TotalTokens != 15 {
		t.Fatalf("tokens = %d/%d/%d, want 10/5/15", bill.PromptTokens, bill.CompletionTokens, bill.TotalTokens)
	}
	if bill.PrivatePoints != 7 || bill.TotalPoints != 7 {
		t.Fatalf("points = private %d total %d, want 7/7", bill.PrivatePoints, bill.TotalPoints)
	}
}

func TestBuildUsageBill_FailedZeroesUsageAndPoints(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.Status = "error"
	bc.PromptTokens = 10
	bc.CompletionTokens = 5
	bc.TotalTokens = 15
	bc.ActualCredits = 7

	bill, err := service.buildUsageBill(bc, usageBillStatusFailed, nil, nil)
	if err != nil {
		t.Fatalf("buildUsageBill returned error: %v", err)
	}
	if bill.PromptTokens != 0 || bill.CompletionTokens != 0 || bill.TotalTokens != 0 {
		t.Fatalf("tokens = %d/%d/%d, want 0/0/0", bill.PromptTokens, bill.CompletionTokens, bill.TotalTokens)
	}
	if bill.TotalPoints != 0 {
		t.Fatalf("total points = %d, want 0", bill.TotalPoints)
	}
}

func TestBuildUsageBill_KeepsPricingAuditFields(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.PricingSource = PricingSourceCodeDefaultFallback
	bc.UsageSource = UsageSourceProviderUsage
	bc.PricingSnapshot = datatypes.JSON([]byte(`{"rule_id":"token.chat.input.default"}`))

	bill, err := service.buildUsageBill(bc, usageBillStatusSuccess, nil, nil)
	if err != nil {
		t.Fatalf("buildUsageBill returned error: %v", err)
	}
	if bill.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default", bill.PricingSource)
	}
	if bill.UsageSource != UsageSourceProviderUsage {
		t.Fatalf("usage source = %q, want provider usage", bill.UsageSource)
	}
	if string(bill.PricingSnapshot) != `{"rule_id":"token.chat.input.default"}` {
		t.Fatalf("pricing snapshot = %s", string(bill.PricingSnapshot))
	}
}

func TestBillingContextNeedsTokenReprice(t *testing.T) {
	t.Run("does not reprice explicit zero pricing", func(t *testing.T) {
		bc := testUsageBillContext(time.Time{}, time.Time{})
		bc.ActualCredits = 0
		bc.PromptTokens = 100
		bc.CompletionTokens = 50
		bc.PricingSource = PricingSourceCodeDefaultFallback

		if billingContextNeedsTokenReprice(bc) {
			t.Fatal("billingContextNeedsTokenReprice = true, want false for already-priced zero credits")
		}
	})

	t.Run("reprices legacy context without pricing source", func(t *testing.T) {
		bc := testUsageBillContext(time.Time{}, time.Time{})
		bc.ActualCredits = 0
		bc.PromptTokens = 100
		bc.CompletionTokens = 50
		bc.PricingSource = ""

		if !billingContextNeedsTokenReprice(bc) {
			t.Fatal("billingContextNeedsTokenReprice = false, want true for unpriced token usage")
		}
	})
}

func testUsageBillContext(requestCreatedAt, settledAt time.Time) *BillingContext {
	channelID := uuid.New()
	return &BillingContext{
		APIKeyID:          uuid.NewString(),
		OrganizationID:    uuid.NewString(),
		AttemptID:         uuid.NewString(),
		RequestID:         uuid.NewString(),
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    uuid.NewString(),
		ModelID:           uuid.New(),
		ModelName:         "gpt-4o-mini",
		ProviderID:        uuid.New(),
		ProviderName:      "openai",
		ChannelID:         &channelID,
		BillingLane:       UsageBillingLanePrivate,
		UseSystemProvider: false,
		Status:            "success",
		RequestCreatedAt:  requestCreatedAt,
		SettledAt:         settledAt,
		ActualCredits:     1,
	}
}
