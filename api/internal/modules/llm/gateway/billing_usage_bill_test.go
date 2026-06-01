package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
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
