package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

	t.Run("writes usage source column", func(t *testing.T) {
		found := false
		for _, column := range usageBillUpsertColumns {
			if column == "usage_source" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("usageBillUpsertColumns does not contain %q", "usage_source")
		}
	})
}

func TestBuildUsageBill_PartialKeepsRecordedUsageAndPoints(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.Status = billingContextStatusPartial
	bc.PromptTokens = 10
	bc.CacheReadTokens = 3
	bc.CacheWriteTokens = 2
	bc.CompletionTokens = 5
	bc.TotalTokens = 0
	bc.ActualCredits = 7

	bill, err := service.buildUsageBill(bc, usageBillStatusPartial, nil, nil)
	if err != nil {
		t.Fatalf("buildUsageBill returned error: %v", err)
	}
	if bill.PromptTokens != 10 || bill.CacheReadTokens != 3 || bill.CacheWriteTokens != 2 || bill.CompletionTokens != 5 || bill.TotalTokens != 20 {
		t.Fatalf("tokens = %d/%d/%d/%d/%d, want 10/3/2/5/20", bill.PromptTokens, bill.CacheReadTokens, bill.CacheWriteTokens, bill.CompletionTokens, bill.TotalTokens)
	}
	if bill.PrivatePoints != 7 || bill.TotalPoints != 7 {
		t.Fatalf("points = private %d total %d, want 7/7", bill.PrivatePoints, bill.TotalPoints)
	}
}

func TestBuildUsageBillPrivateImageUsagePersistsProviderTokens(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.PricingOperation = PricingOperationImage
	applyUsageToBillingContext(bc, &adapter.Usage{
		InputTokens:  4,
		OutputTokens: 1105,
		TotalTokens:  1109,
	})
	bill, err := service.buildUsageBill(bc, usageBillStatusSuccess, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bill.PromptTokens != 4 || bill.CompletionTokens != 1105 || bill.TotalTokens != 1109 {
		t.Fatalf("image token bill = %d/%d/%d, want 4/1105/1109", bill.PromptTokens, bill.CompletionTokens, bill.TotalTokens)
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

func TestBuildUsageBill_UsesStableSafeFailureCode(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.Status = billingContextStatusError
	setBillingFailure(bc, errors.Join(adapter.ErrTimeout, errors.New("provider secret sk-sensitive")))

	bill, err := service.buildUsageBill(bc, usageBillStatusFailed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bill.ErrorCode == nil || *bill.ErrorCode != "llm.provider.timeout" {
		t.Fatalf("error code=%v, want stable provider timeout", bill.ErrorCode)
	}
	if bill.ErrorMessage == nil || *bill.ErrorMessage == "" {
		t.Fatal("raw internal error should remain available only in the protected billing record")
	}
}

func TestBuildUsageBill_FailedDefaultsToGenericSafeCode(t *testing.T) {
	service := &BillingService{}
	bc := testUsageBillContext(time.Time{}, time.Time{})
	bc.Status = billingContextStatusError
	bill, err := service.buildUsageBill(bc, usageBillStatusFailed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bill.ErrorCode == nil || *bill.ErrorCode != "llm.invocation.failed" {
		t.Fatalf("error code=%v, want generic stable failure", bill.ErrorCode)
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

func TestBuildUsageBill_MediaOperationsAlwaysCarrySourceLabels(t *testing.T) {
	service := &BillingService{}
	mediaOperations := []PricingOperation{
		PricingOperationImage,
		PricingOperationVideo,
		PricingOperationMusic,
		PricingOperationSpeech,
		PricingOperationTranscription,
		PricingOperationLyrics,
	}
	lanes := []UsageBillingLane{UsageBillingLanePrivate, UsageBillingLanePlatform}

	for _, operation := range mediaOperations {
		for _, lane := range lanes {
			bc := testUsageBillContext(time.Time{}, time.Time{})
			bc.PricingOperation = operation
			bc.BillingLane = lane
			bc.UseSystemProvider = lane == UsageBillingLanePlatform
			bc.PricingSource = ""
			bc.UsageSource = ""
			bc.PricingSnapshot = nil

			bill, err := service.buildUsageBill(bc, usageBillStatusSuccess, nil, nil)
			if err != nil {
				t.Fatalf("%s/%s: buildUsageBill returned error: %v", operation, lane, err)
			}
			if bill.PricingSource == "" || bill.UsageSource == "" {
				t.Fatalf("%s/%s: sources = %q/%q, want both labels set", operation, lane, bill.PricingSource, bill.UsageSource)
			}

			var snapshot map[string]string
			if err := json.Unmarshal(bill.PricingSnapshot, &snapshot); err != nil {
				t.Fatalf("%s/%s: unmarshal snapshot: %v", operation, lane, err)
			}
			if snapshot["pricing_source"] != string(bill.PricingSource) || snapshot["usage_source"] != string(bill.UsageSource) {
				t.Fatalf("%s/%s: snapshot sources = %#v, want %q/%q", operation, lane, snapshot, bill.PricingSource, bill.UsageSource)
			}
		}
	}
}

// TestMediaUsageBillsPersistSourceColumns writes new media usage bills through
// the real persistence path and reads the columns back, proving both labels
// reach llm_usage_bills.
func TestMediaUsageBillsPersistSourceColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&UsageBill{}); err != nil {
		t.Fatalf("automigrate usage bills: %v", err)
	}

	service := &BillingService{db: db}
	ctx := context.Background()
	mediaOperations := []PricingOperation{
		PricingOperationImage,
		PricingOperationVideo,
		PricingOperationMusic,
		PricingOperationSpeech,
		PricingOperationTranscription,
		PricingOperationLyrics,
	}
	for _, operation := range mediaOperations {
		bc := testUsageBillContext(time.Now().Add(-time.Second), time.Now())
		bc.PricingOperation = operation
		bc.BillingLane = UsageBillingLanePlatform
		bc.UseSystemProvider = true
		bc.PricingSource = ""
		bc.UsageSource = ""

		if err := service.upsertUsageBill(ctx, db, bc, usageBillStatusSuccess, nil, nil); err != nil {
			t.Fatalf("%s: upsertUsageBill returned error: %v", operation, err)
		}

		var bill UsageBill
		if err := db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
			t.Fatalf("%s: load persisted bill: %v", operation, err)
		}
		if bill.PricingSource != PricingSourcePlatformRemoteAuthority {
			t.Fatalf("%s: persisted pricing_source = %q, want %q", operation, bill.PricingSource, PricingSourcePlatformRemoteAuthority)
		}
		if bill.UsageSource == "" {
			t.Fatalf("%s: persisted usage_source is empty", operation)
		}
	}
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
