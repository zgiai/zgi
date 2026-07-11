package upstreamstate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type passthroughCrypto struct{}

func (passthroughCrypto) Encrypt(value string) (string, error) { return value, nil }
func (passthroughCrypto) Decrypt(value string) (string, error) { return value, nil }

type testCredentialTable struct {
	ID               uuid.UUID `gorm:"column:id;type:text;primaryKey"`
	OrganizationID   uuid.UUID `gorm:"column:organization_id;type:text;not null"`
	Name             string
	ChannelProvider  string `gorm:"column:provider"`
	APIKeyCiphertext string
	APIKeyHash       string
	APIBaseURL       string
	IsActive         bool
}

func (testCredentialTable) TableName() string { return "llm_credentials" }

func openUpstreamStateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE llm_credentials (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			name text NOT NULL,
			provider text NOT NULL,
			api_key_ciphertext text NOT NULL,
			api_key_hash text,
			api_base_url text,
			is_active boolean NOT NULL,
			deleted_at datetime
		)`,
		`CREATE TABLE llm_credential_upstream_states (
			credential_id text PRIMARY KEY,
			organization_id text NOT NULL,
			generation integer NOT NULL DEFAULT 1,
			balance_capability text NOT NULL DEFAULT 'unknown',
			balance_snapshot text,
			balance_observed_at datetime,
			warning_thresholds text NOT NULL DEFAULT '[]',
			availability text NOT NULL DEFAULT 'unknown',
			observation_source text,
			availability_observed_at datetime,
			last_check_at datetime,
			last_check_status text NOT NULL DEFAULT 'unknown',
			last_check_error_kind text,
			next_check_at datetime,
			check_lease_until datetime,
			consecutive_failures integer NOT NULL DEFAULT 0,
			block_reason text,
			cooldown_until datetime,
			guard_strikes integer NOT NULL DEFAULT 0,
			half_open_lease_until datetime,
			manual_retry_requested_at datetime,
			provider_error_code text,
			provider_error_status integer NOT NULL DEFAULT 0,
			created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}

func createTestCredential(t *testing.T, db *gorm.DB) *credentialmodel.TenantCredential {
	t.Helper()
	credential := &credentialmodel.TenantCredential{
		ID:               uuid.New(),
		OrganizationID:   uuid.New(),
		Name:             "test credential",
		ChannelProvider:  "deepseek",
		APIKeyCiphertext: "encrypted-key",
		APIKeyHash:       "hash",
		APIBaseURL:       "https://api.deepseek.com",
		IsActive:         true,
	}
	row := testCredentialTable{
		ID:               credential.ID,
		OrganizationID:   credential.OrganizationID,
		Name:             credential.Name,
		ChannelProvider:  credential.ChannelProvider,
		APIKeyCiphertext: credential.APIKeyCiphertext,
		APIKeyHash:       credential.APIKeyHash,
		APIBaseURL:       credential.APIBaseURL,
		IsActive:         credential.IsActive,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	return credential
}

func boolPointer(value bool) *bool { return &value }

func TestServiceCheckStoresCredentialScopedSnapshot(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)

	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }
	service.getBalance = func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error) {
		return &adapter.Balance{
			Scope:     adapter.BalanceScopeAccount,
			Spendable: boolPointer(true),
			Items: []adapter.BalanceItem{
				{Currency: "CNY", Remaining: decimal.RequireFromString("12.5")},
				{Currency: "USD", Remaining: decimal.RequireFromString("3.25")},
			},
		}, nil
	}

	state, err := service.Check(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.BalanceCapability != BalanceCapabilitySupported {
		t.Fatalf("BalanceCapability = %q, want %q", state.BalanceCapability, BalanceCapabilitySupported)
	}
	if state.Availability != AvailabilityAvailable {
		t.Fatalf("Availability = %q, want %q", state.Availability, AvailabilityAvailable)
	}
	if state.BalanceSnapshot == nil || len(state.BalanceSnapshot.Items) != 2 {
		t.Fatalf("BalanceSnapshot = %#v, want two items", state.BalanceSnapshot)
	}
	if got := state.BalanceSnapshot.Items[1].Remaining; got != "3.25" {
		t.Fatalf("Items[1].Remaining = %q, want %q", got, "3.25")
	}
	if state.BalanceObservedAt == nil || !state.BalanceObservedAt.Equal(now) {
		t.Fatalf("BalanceObservedAt = %v, want %v", state.BalanceObservedAt, now)
	}
}

func TestServiceCheckFailurePreservesLastTrustedSnapshot(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	observedAt := time.Date(2026, time.July, 9, 8, 0, 0, 0, time.UTC)
	original := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        1,
		BalanceCapability: BalanceCapabilitySupported,
		BalanceSnapshot: &BalanceSnapshot{
			Scope: string(adapter.BalanceScopeAccount),
			Items: []BalanceAmount{{Currency: "CNY", Remaining: "9.5"}},
		},
		BalanceObservedAt: &observedAt,
		Availability:      AvailabilityAvailable,
	}
	if err := db.Create(original).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	service := NewService(db, passthroughCrypto{})
	service.getBalance = func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error) {
		return nil, errors.New("temporary upstream failure")
	}

	if _, err := service.Check(context.Background(), credential.OrganizationID, credential.ID); err == nil {
		t.Fatal("Check() error = nil, want upstream failure")
	}

	state, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if state.BalanceSnapshot == nil || state.BalanceSnapshot.Items[0].Remaining != "9.5" {
		t.Fatalf("BalanceSnapshot = %#v, want preserved snapshot", state.BalanceSnapshot)
	}
	if state.LastCheckStatus != CheckStatusFailed {
		t.Fatalf("LastCheckStatus = %q, want %q", state.LastCheckStatus, CheckStatusFailed)
	}
}

func TestServiceCheckStoresUnsupportedAndPermissionDeniedAsCapabilityResults(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		checkErr   error
		capability BalanceCapability
		status     CheckStatus
	}{
		{
			name:       "unsupported",
			checkErr:   fmt.Errorf("%w: no verified endpoint", adapter.ErrCapabilityUnsupported),
			capability: BalanceCapabilityUnsupported,
			status:     CheckStatusUnsupported,
		},
		{
			name:       "permission denied",
			checkErr:   adapter.NewAdapterError("401", "not allowed", 401, adapter.ErrAuthFailed),
			capability: BalanceCapabilityPermissionDenied,
			status:     CheckStatusFailed,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := openUpstreamStateTestDB(t)
			credential := createTestCredential(t, db)
			observedAt := time.Now().Add(-time.Minute)
			cooldownUntil := time.Now().Add(time.Hour)
			if err := db.Create(&State{
				CredentialID:      credential.ID,
				OrganizationID:    credential.OrganizationID,
				Generation:        1,
				BalanceCapability: BalanceCapabilitySupported,
				BalanceSnapshot: &BalanceSnapshot{
					Scope: string(adapter.BalanceScopeAccount),
					Items: []BalanceAmount{{Currency: "USD", Remaining: "1"}},
				},
				BalanceObservedAt: &observedAt,
				Availability:      AvailabilityExhausted,
				LastCheckStatus:   CheckStatusSuccess,
				WarningThresholds: []WarningThreshold{{Currency: "USD", Amount: "5"}},
				BlockReason:       GuardReasonBalanceExhausted,
				CooldownUntil:     &cooldownUntil,
				GuardStrikes:      1,
			}).Error; err != nil {
				t.Fatalf("create previous state: %v", err)
			}
			service := NewService(db, passthroughCrypto{})
			service.getBalance = func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error) {
				return nil, testCase.checkErr
			}

			state, err := service.Check(context.Background(), credential.OrganizationID, credential.ID)
			if err != nil {
				t.Fatalf("Check() error = %v, want capability result", err)
			}
			if state.BalanceCapability != testCase.capability || state.LastCheckStatus != testCase.status {
				t.Fatalf("capability result = %q/%q, want %q/%q", state.BalanceCapability, state.LastCheckStatus, testCase.capability, testCase.status)
			}
			if state.NextCheckAt != nil {
				t.Fatalf("NextCheckAt = %v, want polling stopped", state.NextCheckAt)
			}
			if state.BalanceSnapshot != nil || state.BalanceObservedAt != nil || state.Availability != AvailabilityUnknown {
				t.Fatalf("stale balance observation was not cleared: %#v", state)
			}
			if state.BlockReason != "" || state.CooldownUntil != nil || state.GuardStrikes != 0 {
				t.Fatalf("stale guard was not cleared: %#v", state)
			}
			if len(state.WarningThresholds) != 1 {
				t.Fatalf("WarningThresholds = %#v, want preserved settings", state.WarningThresholds)
			}
		})
	}
}

func TestServiceCheckUnsupportedPreservesProviderGuard(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	observedAt := time.Date(2026, time.July, 10, 7, 0, 0, 0, time.UTC)
	cooldownUntil := observedAt.Add(time.Hour)
	state := &State{
		CredentialID:           credential.ID,
		OrganizationID:         credential.OrganizationID,
		Generation:             1,
		BalanceCapability:      BalanceCapabilityUnknown,
		Availability:           AvailabilityExhausted,
		ObservationSource:      ObservationSourceProviderError,
		AvailabilityObservedAt: &observedAt,
		BlockReason:            GuardReasonBillingUnavailable,
		CooldownUntil:          &cooldownUntil,
		GuardStrikes:           1,
		ProviderErrorCode:      "Arrearage",
		ProviderErrorStatus:    400,
		WarningThresholds:      []WarningThreshold{},
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	service := NewService(db, passthroughCrypto{})
	service.getBalance = func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error) {
		return nil, fmt.Errorf("%w: no verified endpoint", adapter.ErrCapabilityUnsupported)
	}
	checked, err := service.Check(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if checked.BalanceCapability != BalanceCapabilityUnsupported {
		t.Fatalf("BalanceCapability = %q, want unsupported", checked.BalanceCapability)
	}
	if checked.BlockReason != GuardReasonBillingUnavailable || checked.Availability != AvailabilityExhausted {
		t.Fatalf("provider guard was cleared: %#v", checked)
	}
	if checked.ProviderErrorCode != "Arrearage" || checked.ProviderErrorStatus != 400 {
		t.Fatalf("provider evidence was cleared: %#v", checked)
	}
}

func TestRecordProviderErrorClassifiesQwenExactCodes(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		status      int
		cause       error
		wantPrecise bool
		wantReason  GuardReason
		wantAvail   Availability
	}{
		{name: "arrearage", code: "Arrearage", status: 400, cause: adapter.ErrBillingUnavailable, wantPrecise: true, wantReason: GuardReasonBillingUnavailable, wantAvail: AvailabilityExhausted},
		{name: "free tier", code: "AllocationQuota.FreeTierOnly", status: 403, cause: adapter.ErrQuotaExhausted, wantPrecise: true, wantReason: GuardReasonQuotaExhausted, wantAvail: AvailabilityExhausted},
		{name: "prepaid overdue", code: "PrepaidBillOverdue", status: 429, cause: adapter.ErrBillingUnavailable, wantPrecise: true, wantReason: GuardReasonBillingUnavailable, wantAvail: AvailabilityExhausted},
		{name: "openai prepaid overdue", code: "PrepaidBillOverdue", status: 429, cause: adapter.ErrRateLimited, wantPrecise: true, wantReason: GuardReasonBillingUnavailable, wantAvail: AvailabilityExhausted},
		{name: "throttling", code: "Throttling.AllocationQuota", status: 429, cause: adapter.ErrRateLimited, wantAvail: AvailabilityUnknown},
		{name: "unknown wording", code: "UnknownAccountState", status: 400, cause: adapter.ErrUpstreamError, wantAvail: AvailabilityUnknown},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := openUpstreamStateTestDB(t)
			credential := createTestCredential(t, db)
			if err := db.Create(&State{
				CredentialID: credential.ID, OrganizationID: credential.OrganizationID, Generation: 1,
				BalanceCapability: BalanceCapabilityUnknown, Availability: AvailabilityUnknown,
				LastCheckStatus: CheckStatusUnknown, WarningThresholds: []WarningThreshold{},
			}).Error; err != nil {
				t.Fatalf("create state: %v", err)
			}
			service := NewService(db, passthroughCrypto{})
			providerErr := adapter.NewAdapterError(testCase.code, testCase.name, testCase.status, testCase.cause)
			precise, err := service.RecordProviderError(context.Background(), credential.OrganizationID, credential.ID, 1, "qwen", false, providerErr)
			if err != nil {
				t.Fatalf("RecordProviderError() error = %v", err)
			}
			if precise != testCase.wantPrecise {
				t.Fatalf("precise = %t, want %t", precise, testCase.wantPrecise)
			}
			state, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if state.BlockReason != testCase.wantReason || state.Availability != testCase.wantAvail {
				t.Fatalf("state = %#v, want reason %q availability %q", state, testCase.wantReason, testCase.wantAvail)
			}
			if testCase.wantPrecise && (state.ProviderErrorCode != testCase.code || state.ProviderErrorStatus != testCase.status) {
				t.Fatalf("provider evidence = %q/%d, want %q/%d", state.ProviderErrorCode, state.ProviderErrorStatus, testCase.code, testCase.status)
			}
			if testCase.wantPrecise && state.BalanceCapability != BalanceCapabilityUnsupported {
				t.Fatalf("BalanceCapability = %q, want unsupported for Qwen", state.BalanceCapability)
			}
			if testCase.wantPrecise {
				if err := service.RecordProviderSuccess(context.Background(), credential.OrganizationID, credential.ID, 1); err != nil {
					t.Fatalf("RecordProviderSuccess() error = %v", err)
				}
				recovered, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
				if err != nil {
					t.Fatalf("Get() after success error = %v", err)
				}
				if recovered.BlockReason != "" || recovered.CooldownUntil != nil || recovered.GuardStrikes != 0 || recovered.Availability != AvailabilityAvailable {
					t.Fatalf("recovered state = %#v, want cleared guard", recovered)
				}
			}
		})
	}
}

func TestServiceCheckRejectsResultFromOldGeneration(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)

	service := NewService(db, passthroughCrypto{})
	service.getBalance = func(ctx context.Context, credential *credentialmodel.TenantCredential, _ string) (*adapter.Balance, error) {
		if err := db.WithContext(ctx).Model(&State{}).
			Where("credential_id = ?", credential.ID).
			Update("generation", gorm.Expr("generation + 1")).Error; err != nil {
			t.Fatalf("increment generation: %v", err)
		}
		return &adapter.Balance{
			Scope:     adapter.BalanceScopeAccount,
			Spendable: boolPointer(true),
			Items:     []adapter.BalanceItem{{Currency: "CNY", Remaining: decimal.NewFromInt(7)}},
		}, nil
	}

	if _, err := service.Check(context.Background(), credential.OrganizationID, credential.ID); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("Check() error = %v, want ErrStaleGeneration", err)
	}
	state, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if state.BalanceSnapshot != nil {
		t.Fatalf("BalanceSnapshot = %#v, want nil", state.BalanceSnapshot)
	}
}

func TestServiceUpdateSettingsRejectsDuplicateCurrency(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	service := NewService(db, passthroughCrypto{})

	_, err := service.UpdateSettings(context.Background(), credential.OrganizationID, credential.ID, []WarningThreshold{
		{Currency: "USD", Amount: "10"},
		{Currency: "usd", Amount: "5"},
	})
	if !errors.Is(err, ErrInvalidThresholds) {
		t.Fatalf("UpdateSettings() error = %v, want ErrInvalidThresholds", err)
	}
}

func TestServiceUpdateSettingsRejectsCurrencyMissingFromSnapshot(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	state := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        1,
		BalanceCapability: BalanceCapabilitySupported,
		BalanceSnapshot: &BalanceSnapshot{
			Scope: string(adapter.BalanceScopeAccount),
			Items: []BalanceAmount{{Currency: "USD", Remaining: "10"}},
		},
		Availability:      AvailabilityAvailable,
		LastCheckStatus:   CheckStatusSuccess,
		WarningThresholds: []WarningThreshold{},
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})

	_, err := service.UpdateSettings(context.Background(), credential.OrganizationID, credential.ID, []WarningThreshold{
		{Currency: "CNY", Amount: "5"},
	})
	if !errors.Is(err, ErrInvalidThresholds) {
		t.Fatalf("UpdateSettings() error = %v, want ErrInvalidThresholds", err)
	}
}

func TestCheckFailureBackoffSchedule(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{failures: 1, want: 5 * time.Minute},
		{failures: 2, want: 15 * time.Minute},
		{failures: 3, want: time.Hour},
		{failures: 4, want: 6 * time.Hour},
		{failures: 10, want: 6 * time.Hour},
	}
	for _, testCase := range tests {
		if got := checkFailureBackoff(testCase.failures); got != testCase.want {
			t.Fatalf("checkFailureBackoff(%d) = %v, want %v", testCase.failures, got, testCase.want)
		}
	}
}

func TestRepositoryGetManyIsOrganizationScoped(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	first := createTestCredential(t, db)
	second := createTestCredential(t, db)
	for _, credential := range []*credentialmodel.TenantCredential{first, second} {
		if _, err := NewRepository(db).Ensure(context.Background(), credential.OrganizationID, credential.ID); err != nil {
			t.Fatalf("Ensure(%s) error = %v", credential.ID, err)
		}
	}

	states, err := NewRepository(db).GetMany(context.Background(), first.OrganizationID, []uuid.UUID{first.ID, second.ID})
	if err != nil {
		t.Fatalf("GetMany() error = %v", err)
	}
	if len(states) != 1 || states[first.ID] == nil || states[second.ID] != nil {
		t.Fatalf("GetMany() states = %#v, want only first organization", states)
	}
}

func TestNextSuccessfulCheckKeepsExhaustedCredentialUnderObservationWithoutThreshold(t *testing.T) {
	now := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)
	spendable := false
	next := nextSuccessfulCheck(now, &BalanceSnapshot{Spendable: &spendable}, nil, 10*time.Minute)
	if next == nil || !next.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("nextSuccessfulCheck() = %v, want five minutes", next)
	}
}

func TestResetForCredentialTxInvalidatesOldObservations(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	observedAt := time.Date(2026, time.July, 10, 7, 0, 0, 0, time.UTC)
	state := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        4,
		BalanceCapability: BalanceCapabilitySupported,
		BalanceSnapshot: &BalanceSnapshot{
			Scope: string(adapter.BalanceScopeAccount),
			Items: []BalanceAmount{{Currency: "USD", Remaining: "8"}},
		},
		BalanceObservedAt:      &observedAt,
		Availability:           AvailabilityAvailable,
		AvailabilityObservedAt: &observedAt,
		WarningThresholds:      []WarningThreshold{{Currency: "USD", Amount: "10"}},
		LastCheckStatus:        CheckStatusSuccess,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return ResetForCredentialTx(tx, credential.OrganizationID, credential.ID)
	}); err != nil {
		t.Fatalf("ResetForCredentialTx() error = %v", err)
	}

	reset, err := NewRepository(db).Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if reset.Generation != 5 {
		t.Fatalf("Generation = %d, want 5", reset.Generation)
	}
	if reset.BalanceSnapshot != nil || reset.BalanceObservedAt != nil {
		t.Fatalf("balance observation was not cleared: %#v", reset)
	}
	if len(reset.WarningThresholds) != 1 {
		t.Fatalf("WarningThresholds = %#v, want preserved settings", reset.WarningThresholds)
	}
}

func TestRecordProviderErrorOnlyGuardsVerifiedProviderEvidence(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	state := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        1,
		BalanceCapability: BalanceCapabilityUnknown,
		Availability:      AvailabilityUnknown,
		LastCheckStatus:   CheckStatusUnknown,
		WarningThresholds: []WarningThreshold{},
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})

	rateLimitErr := adapter.NewAdapterError("429", "rate limited", 429, adapter.ErrRateLimited)
	precise, err := service.RecordProviderError(context.Background(), credential.OrganizationID, credential.ID, 1, "deepseek", false, rateLimitErr)
	if err != nil {
		t.Fatalf("RecordProviderError(rate limit) error = %v", err)
	}
	if precise {
		t.Fatal("RecordProviderError(rate limit) precise = true, want false")
	}

	paymentErr := adapter.NewAdapterError("402", "insufficient balance", 402, adapter.ErrInsufficientBalance)
	precise, err = service.RecordProviderError(context.Background(), credential.OrganizationID, credential.ID, 1, "deepseek", false, paymentErr)
	if err != nil {
		t.Fatalf("RecordProviderError(payment) error = %v", err)
	}
	if precise {
		t.Fatal("RecordProviderError(payment) precise = true for unverified provider")
	}

	authErr := adapter.NewAdapterError("401", "invalid key", 401, adapter.ErrAuthFailed)
	precise, err = service.RecordProviderError(context.Background(), credential.OrganizationID, credential.ID, 1, "openrouter", false, authErr)
	if err != nil {
		t.Fatalf("RecordProviderError(auth) error = %v", err)
	}
	if precise {
		t.Fatal("RecordProviderError(auth) precise = true for unverified provider")
	}
	unchanged, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if unchanged.BlockReason != "" || unchanged.Availability != AvailabilityUnknown || unchanged.ProviderErrorCode != "" {
		t.Fatalf("unverified provider changed guard state: %#v", unchanged)
	}
}

func TestClassifyProviderGuardErrorRequiresExactQwenAuthCode(t *testing.T) {
	unknown401 := adapter.NewAdapterError("WorkspaceAccessDenied", "not allowed", http.StatusUnauthorized, adapter.ErrAuthFailed)
	if reason, _, _, precise := classifyProviderGuardError("qwen", unknown401); precise || reason != "" {
		t.Fatalf("unknown 401 classified as reason=%q precise=%t, want fail open", reason, precise)
	}

	invalidKey := adapter.NewAdapterError("InvalidApiKey", "invalid key", http.StatusUnauthorized, adapter.ErrAuthFailed)
	reason, availability, _, precise := classifyProviderGuardError("qwen", invalidKey)
	if !precise || reason != GuardReasonAuthInvalid || availability != AvailabilityInvalidKey {
		t.Fatalf("InvalidApiKey classified as reason=%q availability=%q precise=%t", reason, availability, precise)
	}
}

func TestEvaluateGuardHalfOpenAllowsOneConcurrentRequest(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 8, 30, 0, 0, time.UTC)
	cooldownEnded := now.Add(-time.Second)
	state := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        2,
		BalanceCapability: BalanceCapabilitySupported,
		Availability:      AvailabilityExhausted,
		LastCheckStatus:   CheckStatusSuccess,
		WarningThresholds: []WarningThreshold{},
		BlockReason:       GuardReasonBalanceExhausted,
		CooldownUntil:     &cooldownEnded,
		GuardStrikes:      1,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }

	readOnly := service.EvaluateGuardReadOnly(state, true)
	if !readOnly.Block || readOnly.HalfOpen {
		t.Fatalf("read-only decision = %#v, want blocked without half-open lease", readOnly)
	}
	beforeProbe, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() before probe error = %v", err)
	}
	if beforeProbe.HalfOpenLeaseUntil != nil {
		t.Fatalf("read-only guard acquired lease until %v", beforeProbe.HalfOpenLeaseUntil)
	}

	first, err := service.EvaluateGuard(context.Background(), state, true, true)
	if err != nil {
		t.Fatalf("EvaluateGuard(first) error = %v", err)
	}
	if first.Block || !first.HalfOpen {
		t.Fatalf("first decision = %#v, want one half-open request", first)
	}
	second, err := service.EvaluateGuard(context.Background(), state, true, true)
	if err != nil {
		t.Fatalf("EvaluateGuard(second) error = %v", err)
	}
	if !second.Block || second.HalfOpen {
		t.Fatalf("second decision = %#v, want blocked while lease is held", second)
	}
}

func TestEvaluateProbeEligibilityIsReadOnly(t *testing.T) {
	now := time.Date(2026, time.July, 10, 9, 30, 0, 0, time.UTC)
	cooldownEnded := now.Add(-time.Second)
	state := &State{
		BlockReason:   GuardReasonBillingUnavailable,
		CooldownUntil: &cooldownEnded,
	}
	service := &Service{now: func() time.Time { return now }}

	eligible, requiresBackup := service.EvaluateProbeEligibility(state, true, true)
	if !eligible || !requiresBackup {
		t.Fatalf("eligibility = %t/%t, want automatic probe requiring backup", eligible, requiresBackup)
	}
	if state.HalfOpenLeaseUntil != nil {
		t.Fatalf("eligibility check mutated lease: %v", state.HalfOpenLeaseUntil)
	}

	retryRequestedAt := now.Add(-time.Minute)
	state.ManualRetryRequestedAt = &retryRequestedAt
	eligible, requiresBackup = service.EvaluateProbeEligibility(state, true, false)
	if !eligible || requiresBackup {
		t.Fatalf("manual eligibility = %t/%t, want probe without backup", eligible, requiresBackup)
	}
}

func TestEvaluateGuardRequiresBackupOrManualRetryForHalfOpen(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	cooldownEnded := now.Add(-time.Second)
	state := &State{
		CredentialID: credential.ID, OrganizationID: credential.OrganizationID, Generation: 1,
		BalanceCapability: BalanceCapabilityUnsupported, Availability: AvailabilityExhausted,
		ObservationSource: ObservationSourceProviderError, LastCheckStatus: CheckStatusUnsupported,
		WarningThresholds: []WarningThreshold{}, BlockReason: GuardReasonBillingUnavailable,
		CooldownUntil: &cooldownEnded, GuardStrikes: 1,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }

	withoutBackup, err := service.EvaluateGuard(context.Background(), state, true, false)
	if err != nil {
		t.Fatalf("EvaluateGuard(no backup) error = %v", err)
	}
	if !withoutBackup.Block || withoutBackup.HalfOpen {
		t.Fatalf("decision = %#v, want blocked without backup", withoutBackup)
	}

	retryState, err := service.RequestRetry(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("RequestRetry() error = %v", err)
	}
	if retryState.ManualRetryRequestedAt == nil {
		t.Fatal("ManualRetryRequestedAt = nil, want pending retry")
	}
	manual, err := service.EvaluateGuard(context.Background(), retryState, true, false)
	if err != nil {
		t.Fatalf("EvaluateGuard(manual) error = %v", err)
	}
	if manual.Block || !manual.HalfOpen {
		t.Fatalf("manual decision = %#v, want one half-open request", manual)
	}
	stored, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.ManualRetryRequestedAt != nil || stored.HalfOpenLeaseUntil == nil {
		t.Fatalf("stored retry state = %#v, want consumed request and active lease", stored)
	}
}

func TestAuthInvalidNeverAutomaticallyHalfOpens(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 11, 0, 0, 0, time.UTC)
	cooldownEnded := now.Add(-time.Hour)
	state := &State{
		CredentialID: credential.ID, OrganizationID: credential.OrganizationID, Generation: 1,
		BalanceCapability: BalanceCapabilityUnsupported, Availability: AvailabilityInvalidKey,
		ObservationSource: ObservationSourceProviderError, LastCheckStatus: CheckStatusUnsupported,
		WarningThresholds: []WarningThreshold{}, BlockReason: GuardReasonAuthInvalid,
		CooldownUntil: &cooldownEnded, GuardStrikes: 1,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }

	decision, err := service.EvaluateGuard(context.Background(), state, true, true)
	if err != nil {
		t.Fatalf("EvaluateGuard() error = %v", err)
	}
	if !decision.Block || decision.HalfOpen {
		t.Fatalf("decision = %#v, want auth block until manual retry", decision)
	}
}

func TestRequestRetryDoesNotReplaceActiveHalfOpenLease(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(time.Minute)
	state := &State{
		CredentialID: credential.ID, OrganizationID: credential.OrganizationID, Generation: 1,
		BalanceCapability: BalanceCapabilityUnsupported, Availability: AvailabilityExhausted,
		ObservationSource: ObservationSourceProviderError, LastCheckStatus: CheckStatusUnsupported,
		WarningThresholds: []WarningThreshold{}, BlockReason: GuardReasonBillingUnavailable,
		HalfOpenLeaseUntil: &leaseUntil, GuardStrikes: 1,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }

	if _, err := service.RequestRetry(context.Background(), credential.OrganizationID, credential.ID); !errors.Is(err, ErrRetryInProgress) {
		t.Fatalf("RequestRetry() error = %v, want ErrRetryInProgress", err)
	}
	stored, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.HalfOpenLeaseUntil == nil || !stored.HalfOpenLeaseUntil.Equal(leaseUntil) || stored.ManualRetryRequestedAt != nil {
		t.Fatalf("stored = %#v, want unchanged active lease", stored)
	}
}

func TestHalfOpenTransientFailureKeepsStrikeAndRetriesSoon(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	now := time.Date(2026, time.July, 10, 9, 0, 0, 0, time.UTC)
	state := &State{
		CredentialID:      credential.ID,
		OrganizationID:    credential.OrganizationID,
		Generation:        1,
		BalanceCapability: BalanceCapabilitySupported,
		Availability:      AvailabilityExhausted,
		LastCheckStatus:   CheckStatusSuccess,
		WarningThresholds: []WarningThreshold{},
		BlockReason:       GuardReasonBalanceExhausted,
		GuardStrikes:      2,
	}
	if err := db.Create(state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	service := NewService(db, passthroughCrypto{})
	service.now = func() time.Time { return now }

	precise, err := service.RecordProviderError(
		context.Background(),
		credential.OrganizationID,
		credential.ID,
		1,
		"deepseek",
		true,
		adapter.NewAdapterError("503", "unavailable", 503, adapter.ErrUpstreamError),
	)
	if err != nil {
		t.Fatalf("RecordProviderError() error = %v", err)
	}
	if precise {
		t.Fatal("RecordProviderError() precise = true, want transient failure")
	}
	updated, err := service.Get(context.Background(), credential.OrganizationID, credential.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.GuardStrikes != 2 {
		t.Fatalf("GuardStrikes = %d, want unchanged 2", updated.GuardStrikes)
	}
	wantRetry := now.Add(5 * time.Minute)
	if updated.CooldownUntil == nil || !updated.CooldownUntil.Equal(wantRetry) {
		t.Fatalf("CooldownUntil = %v, want %v", updated.CooldownUntil, wantRetry)
	}
}

func TestServiceCheckLeaseDeduplicatesConcurrentChecks(t *testing.T) {
	db := openUpstreamStateTestDB(t)
	credential := createTestCredential(t, db)
	service := NewService(db, passthroughCrypto{})
	entered := make(chan struct{})
	release := make(chan struct{})
	service.getBalance = func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error) {
		close(entered)
		<-release
		return &adapter.Balance{
			Scope:     adapter.BalanceScopeAccount,
			Spendable: boolPointer(true),
			Items:     []adapter.BalanceItem{{Currency: "USD", Remaining: decimal.NewFromInt(9)}},
		}, nil
	}

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.Check(context.Background(), credential.OrganizationID, credential.ID)
		firstResult <- err
	}()
	<-entered

	if _, err := service.Check(context.Background(), credential.OrganizationID, credential.ID); !errors.Is(err, ErrCheckInProgress) {
		t.Fatalf("second Check() error = %v, want ErrCheckInProgress", err)
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	if _, err := service.Check(context.Background(), credential.OrganizationID, credential.ID); !errors.Is(err, ErrCheckInProgress) {
		t.Fatalf("immediate Check() after completion error = %v, want retained lease", err)
	}
}
