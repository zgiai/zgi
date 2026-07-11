package upstreamstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	checkLeaseDuration    = 30 * time.Second
	providerTimeout       = 10 * time.Second
	dueCheckBatchSize     = 100
	dueCheckConcurrency   = 4
	observationStaleAfter = 2 * time.Hour
)

func IsLow(state *State) bool {
	return state != nil && state.BalanceSnapshot != nil && snapshotIsLow(state.BalanceSnapshot, state.WarningThresholds)
}

func IsStale(state *State, now time.Time) bool {
	return state != nil && state.BalanceObservedAt != nil && now.Sub(*state.BalanceObservedAt) > observationStaleAfter
}

var (
	ErrStateNotFound      = errors.New("upstream state not found")
	ErrCredentialNotFound = errors.New("credential not found")
	ErrCheckInProgress    = errors.New("upstream check already in progress")
	ErrStaleGeneration    = errors.New("upstream check belongs to a stale credential generation")
	ErrInvalidThresholds  = errors.New("invalid upstream warning thresholds")
	ErrRetryNotRequired   = errors.New("upstream credential is not blocked")
	ErrRetryInProgress    = errors.New("upstream credential retry is already in progress")
)

type balanceGetter func(context.Context, *credentialmodel.TenantCredential, string) (*adapter.Balance, error)

type Service struct {
	db            *gorm.DB
	repository    Repository
	crypto        shared.CryptoService
	getBalance    balanceGetter
	now           func() time.Time
	jitter        func() time.Duration
	providerMu    sync.Mutex
	providerSlots map[string]chan struct{}
}

func NewService(db *gorm.DB, crypto shared.CryptoService) *Service {
	service := &Service{
		db:            db,
		repository:    NewRepository(db),
		crypto:        crypto,
		now:           time.Now,
		providerSlots: make(map[string]chan struct{}),
		jitter:        func() time.Duration { return time.Duration(rand.IntN(11)) * time.Minute },
	}
	service.getBalance = service.getProviderBalance
	return service
}

func (s *Service) ScheduleCheck(ctx context.Context, organizationID, credentialID uuid.UUID) error {
	if err := s.ensureCredential(ctx, organizationID, credentialID); err != nil {
		return err
	}
	state, err := s.ensureState(ctx, organizationID, credentialID)
	if err != nil {
		return err
	}
	if state.BalanceCapability == BalanceCapabilityUnsupported || state.BalanceCapability == BalanceCapabilityPermissionDenied {
		return nil
	}
	return s.repository.UpdateUnversioned(ctx, organizationID, credentialID, map[string]any{
		"next_check_at": s.now(),
		"updated_at":    s.now(),
	})
}

func (s *Service) RunDueChecks(ctx context.Context) (int, error) {
	now := s.now()
	stats, err := s.repository.DueStats(ctx, now)
	if err != nil {
		return 0, err
	}
	recordBacklogMetrics(ctx, stats)
	checks, err := s.repository.ListDueChecks(ctx, now, dueCheckBatchSize)
	if err != nil {
		return 0, err
	}
	jobs := make(chan DueCheck)
	var workers sync.WaitGroup
	for range dueCheckConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for check := range jobs {
				_, checkErr := s.Check(ctx, check.OrganizationID, check.CredentialID)
				if errors.Is(checkErr, ErrCredentialNotFound) {
					_ = s.repository.UpdateUnversioned(ctx, check.OrganizationID, check.CredentialID, map[string]any{
						"last_check_status":     CheckStatusFailed,
						"last_check_error_kind": "credential_inactive",
						"next_check_at":         nil,
						"updated_at":            s.now(),
					})
				}
			}
		}()
	}
	for _, check := range checks {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return len(checks), ctx.Err()
		case jobs <- check:
		}
	}
	close(jobs)
	workers.Wait()
	return len(checks), nil
}

func (s *Service) Get(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	return s.repository.Get(ctx, organizationID, credentialID)
}

func (s *Service) GetMany(ctx context.Context, organizationID uuid.UUID, credentialIDs []uuid.UUID) (map[uuid.UUID]*State, error) {
	return s.repository.GetMany(ctx, organizationID, credentialIDs)
}

func (s *Service) UpdateSettings(ctx context.Context, organizationID, credentialID uuid.UUID, thresholds []WarningThreshold) (*State, error) {
	normalized, err := normalizeThresholds(thresholds)
	if err != nil {
		return nil, err
	}
	if err := s.ensureCredential(ctx, organizationID, credentialID); err != nil {
		return nil, err
	}
	state, err := s.ensureState(ctx, organizationID, credentialID)
	if err != nil {
		return nil, err
	}
	if len(normalized) > 0 && !thresholdCurrenciesMatchSnapshot(normalized, state.BalanceSnapshot) {
		return nil, ErrInvalidThresholds
	}
	now := s.now()
	thresholdsJSON, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode upstream warning thresholds: %w", err)
	}
	updates := map[string]any{"warning_thresholds": datatypes.JSON(thresholdsJSON), "updated_at": now}
	if len(normalized) > 0 || state.Availability == AvailabilityExhausted {
		updates["next_check_at"] = now
	} else {
		updates["next_check_at"] = nil
	}
	if err := s.repository.UpdateUnversioned(ctx, organizationID, credentialID, updates); err != nil {
		return nil, fmt.Errorf("update upstream settings: %w", err)
	}
	state.WarningThresholds = normalized
	return s.Get(ctx, organizationID, credentialID)
}

func (s *Service) Check(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	state, err := s.ensureState(ctx, organizationID, credentialID)
	if err != nil {
		return nil, err
	}
	credential, err := s.loadCredential(ctx, organizationID, credentialID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	leaseUntil := now.Add(checkLeaseDuration)
	acquired, err := s.acquireCheckLease(ctx, state, now, leaseUntil)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrCheckInProgress
	}

	apiKey, err := s.crypto.Decrypt(credential.APIKeyCiphertext)
	if err != nil {
		recordCheckMetric(ctx, credential.ChannelProvider, "credential_decrypt_failed")
		if saveErr := s.saveCheckFailure(ctx, state, now, "credential_decrypt_failed"); saveErr != nil {
			return nil, saveErr
		}
		return nil, fmt.Errorf("decrypt credential: %w", err)
	}
	providerCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	var balance *adapter.Balance
	err = s.withProviderSlot(providerCtx, credential.ChannelProvider, func() error {
		var getErr error
		balance, getErr = s.getBalance(providerCtx, credential, apiKey)
		return getErr
	})
	if err != nil {
		return s.handleCheckError(ctx, state, credential.ChannelProvider, now, err)
	}
	snapshot, err := normalizeBalance(balance)
	if err != nil {
		recordCheckMetric(ctx, credential.ChannelProvider, "invalid_response")
		if saveErr := s.saveCheckFailure(ctx, state, now, "invalid_balance_snapshot"); saveErr != nil {
			return nil, saveErr
		}
		return nil, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode upstream balance snapshot: %w", err)
	}
	availability := AvailabilityAvailable
	if snapshot.Spendable != nil && !*snapshot.Spendable {
		availability = AvailabilityExhausted
	}
	nextCheckAt := nextSuccessfulCheck(now, snapshot, state.WarningThresholds, s.jitter())
	updates := map[string]any{
		"balance_capability":        BalanceCapabilitySupported,
		"balance_snapshot":          datatypes.JSON(snapshotJSON),
		"balance_observed_at":       now,
		"availability":              availability,
		"observation_source":        ObservationSourceBalanceAPI,
		"availability_observed_at":  now,
		"last_check_at":             now,
		"last_check_status":         CheckStatusSuccess,
		"last_check_error_kind":     "",
		"next_check_at":             nextCheckAt,
		"consecutive_failures":      0,
		"manual_retry_requested_at": nil,
		"provider_error_code":       "",
		"provider_error_status":     0,
		"updated_at":                now,
	}
	if availability == AvailabilityAvailable {
		updates["block_reason"] = ""
		updates["cooldown_until"] = nil
		updates["guard_strikes"] = 0
		updates["half_open_lease_until"] = nil
	} else {
		strikes, cooldownUntil := nextGuardCooldown(state, GuardReasonBalanceExhausted, false, now)
		updates["block_reason"] = GuardReasonBalanceExhausted
		updates["cooldown_until"] = cooldownUntil
		updates["guard_strikes"] = strikes
		updates["half_open_lease_until"] = nil
	}
	updated, err := s.repository.Update(ctx, state, updates)
	if err != nil {
		return nil, fmt.Errorf("save upstream balance snapshot: %w", err)
	}
	if !updated {
		recordCheckMetric(ctx, credential.ChannelProvider, "stale_generation")
		return nil, ErrStaleGeneration
	}
	recordCheckMetric(ctx, credential.ChannelProvider, "success")
	return s.Get(ctx, organizationID, credentialID)
}

func (s *Service) withProviderSlot(ctx context.Context, provider string, fn func() error) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	s.providerMu.Lock()
	slot := s.providerSlots[provider]
	if slot == nil {
		slot = make(chan struct{}, 1)
		s.providerSlots[provider] = slot
	}
	s.providerMu.Unlock()

	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) handleCheckError(ctx context.Context, state *State, provider string, now time.Time, checkErr error) (*State, error) {
	switch {
	case errors.Is(checkErr, adapter.ErrCapabilityUnsupported):
		recordCheckMetric(ctx, provider, "unsupported")
		if err := s.saveCapabilityResult(ctx, state, now, BalanceCapabilityUnsupported, CheckStatusUnsupported, "unsupported"); err != nil {
			return nil, err
		}
		return s.Get(ctx, state.OrganizationID, state.CredentialID)
	case errors.Is(checkErr, adapter.ErrAuthFailed):
		recordCheckMetric(ctx, provider, "permission_denied")
		if err := s.saveCapabilityResult(ctx, state, now, BalanceCapabilityPermissionDenied, CheckStatusFailed, "permission_denied"); err != nil {
			return nil, err
		}
		return s.Get(ctx, state.OrganizationID, state.CredentialID)
	default:
		recordCheckMetric(ctx, provider, checkErrorKind(checkErr))
		if err := s.saveCheckFailure(ctx, state, now, checkErrorKind(checkErr)); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("check upstream balance: %w", checkErr)
	}
}

func (s *Service) saveCapabilityResult(ctx context.Context, state *State, now time.Time, capability BalanceCapability, status CheckStatus, errorKind string) error {
	fields := map[string]any{
		"balance_capability":    capability,
		"balance_snapshot":      nil,
		"balance_observed_at":   nil,
		"last_check_at":         now,
		"last_check_status":     status,
		"last_check_error_kind": errorKind,
		"next_check_at":         nil,
		"consecutive_failures":  0,
		"updated_at":            now,
	}
	if state.ObservationSource != ObservationSourceProviderError {
		fields["availability"] = AvailabilityUnknown
		fields["observation_source"] = ""
		fields["availability_observed_at"] = nil
		fields["block_reason"] = ""
		fields["cooldown_until"] = nil
		fields["guard_strikes"] = 0
		fields["half_open_lease_until"] = nil
		fields["manual_retry_requested_at"] = nil
		fields["provider_error_code"] = ""
		fields["provider_error_status"] = 0
	}
	updated, err := s.repository.Update(ctx, state, fields)
	if err != nil {
		return fmt.Errorf("save upstream capability result: %w", err)
	}
	if !updated {
		return ErrStaleGeneration
	}
	return nil
}

func (s *Service) saveCheckFailure(ctx context.Context, state *State, now time.Time, errorKind string) error {
	failures := state.ConsecutiveFailures + 1
	nextCheckAt := now.Add(checkFailureBackoff(failures))
	updated, err := s.repository.Update(ctx, state, map[string]any{
		"last_check_at":         now,
		"last_check_status":     CheckStatusFailed,
		"last_check_error_kind": errorKind,
		"next_check_at":         nextCheckAt,
		"consecutive_failures":  failures,
		"updated_at":            now,
	})
	if err != nil {
		return fmt.Errorf("save upstream check failure: %w", err)
	}
	if !updated {
		return ErrStaleGeneration
	}
	return nil
}

func (s *Service) ensureState(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	return s.repository.Ensure(ctx, organizationID, credentialID)
}

func (s *Service) ensureCredential(ctx context.Context, organizationID, credentialID uuid.UUID) error {
	_, err := s.loadCredential(ctx, organizationID, credentialID)
	return err
}

func (s *Service) loadCredential(ctx context.Context, organizationID, credentialID uuid.UUID) (*credentialmodel.TenantCredential, error) {
	var credential credentialmodel.TenantCredential
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND is_active = ?", credentialID, organizationID, true).
		First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("load credential: %w", err)
	}
	return &credential, nil
}

func (s *Service) acquireCheckLease(ctx context.Context, state *State, now, leaseUntil time.Time) (bool, error) {
	acquired, err := s.repository.AcquireCheckLease(ctx, state.OrganizationID, state.CredentialID, now, leaseUntil)
	if err != nil {
		return false, fmt.Errorf("acquire upstream check lease: %w", err)
	}
	return acquired, nil
}

func (s *Service) getProviderBalance(ctx context.Context, credential *credentialmodel.TenantCredential, apiKey string) (*adapter.Balance, error) {
	provider := strings.TrimSpace(strings.ToLower(credential.ChannelProvider))
	if provider != "deepseek" && provider != "openrouter" {
		return nil, fmt.Errorf("%w: provider %s has no verified inference-key balance API", adapter.ErrCapabilityUnsupported, provider)
	}
	spec, err := channelprovider.Resolve(provider)
	if err != nil {
		return nil, err
	}
	cfg := appconfig.Current().LLM
	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName:        spec.AdapterKey,
		APIKey:              apiKey,
		BaseURL:             credential.APIBaseURL,
		Timeout:             providerTimeout,
		MaxRetries:          1,
		GuardOutboundURL:    cfg.OutboundURLGuardEnabled(),
		GuardOutboundDNS:    cfg.GuardOutboundDNS,
		AllowPrivateBaseURL: channelprovider.AllowsPrivateBaseURL(spec.Name),
	})
	if err != nil {
		return nil, err
	}
	return instance.GetBalance(ctx, apiKey)
}

func normalizeBalance(balance *adapter.Balance) (*BalanceSnapshot, error) {
	if balance == nil {
		return nil, fmt.Errorf("balance response is nil")
	}
	items := make([]BalanceAmount, 0, len(balance.Items)+1)
	for _, item := range balance.Items {
		currency := strings.ToUpper(strings.TrimSpace(item.Currency))
		if currency == "" {
			return nil, fmt.Errorf("balance item has no currency")
		}
		items = append(items, BalanceAmount{Currency: currency, Remaining: item.Remaining.String()})
	}
	if len(items) == 0 && !balance.IsUnlimited && strings.TrimSpace(balance.Currency) != "" {
		items = append(items, BalanceAmount{
			Currency:  strings.ToUpper(strings.TrimSpace(balance.Currency)),
			Remaining: balance.Remaining.String(),
		})
	}
	if len(items) == 0 && !balance.IsUnlimited {
		return nil, fmt.Errorf("balance response has no amounts")
	}
	return &BalanceSnapshot{
		Scope:       string(balance.Scope),
		Items:       items,
		Spendable:   balance.Spendable,
		IsUnlimited: balance.IsUnlimited,
	}, nil
}

func normalizeThresholds(thresholds []WarningThreshold) ([]WarningThreshold, error) {
	normalized := make([]WarningThreshold, 0, len(thresholds))
	seen := make(map[string]struct{}, len(thresholds))
	for _, threshold := range thresholds {
		currency := strings.ToUpper(strings.TrimSpace(threshold.Currency))
		amount, err := decimal.NewFromString(strings.TrimSpace(threshold.Amount))
		if err != nil || currency == "" || amount.IsNegative() {
			return nil, ErrInvalidThresholds
		}
		if _, exists := seen[currency]; exists {
			return nil, ErrInvalidThresholds
		}
		seen[currency] = struct{}{}
		normalized = append(normalized, WarningThreshold{Currency: currency, Amount: amount.String()})
	}
	return normalized, nil
}

func thresholdCurrenciesMatchSnapshot(thresholds []WarningThreshold, snapshot *BalanceSnapshot) bool {
	if snapshot == nil || len(snapshot.Items) == 0 {
		return false
	}
	available := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		available[strings.ToUpper(strings.TrimSpace(item.Currency))] = struct{}{}
	}
	for _, threshold := range thresholds {
		if _, ok := available[threshold.Currency]; !ok {
			return false
		}
	}
	return true
}

func nextSuccessfulCheck(now time.Time, snapshot *BalanceSnapshot, thresholds []WarningThreshold, jitter time.Duration) *time.Time {
	if snapshot.Spendable != nil && !*snapshot.Spendable {
		next := now.Add(5 * time.Minute)
		return &next
	}
	if len(thresholds) == 0 {
		return nil
	}
	interval := time.Hour + jitter
	if snapshotIsLow(snapshot, thresholds) {
		interval = 15 * time.Minute
	}
	next := now.Add(interval)
	return &next
}

func snapshotIsLow(snapshot *BalanceSnapshot, thresholds []WarningThreshold) bool {
	thresholdByCurrency := make(map[string]decimal.Decimal, len(thresholds))
	for _, threshold := range thresholds {
		amount, err := decimal.NewFromString(threshold.Amount)
		if err == nil {
			thresholdByCurrency[threshold.Currency] = amount
		}
	}
	for _, item := range snapshot.Items {
		threshold, ok := thresholdByCurrency[item.Currency]
		if !ok {
			continue
		}
		remaining, err := decimal.NewFromString(item.Remaining)
		if err == nil && remaining.LessThanOrEqual(threshold) {
			return true
		}
	}
	return false
}

func checkFailureBackoff(failures int) time.Duration {
	switch failures {
	case 1:
		return 5 * time.Minute
	case 2:
		return 15 * time.Minute
	case 3:
		return time.Hour
	default:
		return 6 * time.Hour
	}
}

func checkErrorKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, adapter.ErrTimeout):
		return "timeout"
	case errors.Is(err, adapter.ErrRateLimited):
		return "rate_limited"
	default:
		return "upstream_error"
	}
}

func ResetForCredentialTx(tx *gorm.DB, organizationID, credentialID uuid.UUID) error {
	now := time.Now()
	return tx.Model(&State{}).
		Where("credential_id = ? AND organization_id = ?", credentialID, organizationID).
		Updates(map[string]any{
			"generation":                gorm.Expr("generation + 1"),
			"balance_capability":        BalanceCapabilityUnknown,
			"balance_snapshot":          nil,
			"balance_observed_at":       nil,
			"availability":              AvailabilityUnknown,
			"observation_source":        "",
			"availability_observed_at":  nil,
			"last_check_at":             nil,
			"last_check_status":         CheckStatusUnknown,
			"last_check_error_kind":     "",
			"next_check_at":             now,
			"check_lease_until":         nil,
			"consecutive_failures":      0,
			"block_reason":              "",
			"cooldown_until":            nil,
			"guard_strikes":             0,
			"half_open_lease_until":     nil,
			"manual_retry_requested_at": nil,
			"provider_error_code":       "",
			"provider_error_status":     0,
			"updated_at":                now,
		}).Error
}
