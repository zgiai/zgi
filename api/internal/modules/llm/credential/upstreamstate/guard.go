package upstreamstate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const halfOpenLeaseDuration = 15 * time.Minute

type GuardDecision struct {
	WouldGuard bool
	Block      bool
	HalfOpen   bool
	Reason     GuardReason
}

func (s *Service) EvaluateGuard(ctx context.Context, state *State, enforce, allowAutomaticHalfOpen bool) (GuardDecision, error) {
	if state == nil || state.BlockReason == "" {
		return GuardDecision{}, nil
	}
	decision := GuardDecision{WouldGuard: true, Reason: state.BlockReason}
	if !enforce {
		return decision, nil
	}
	now := s.now()
	manual := state.ManualRetryRequestedAt != nil
	if !manual && (!allowAutomaticHalfOpen || state.BlockReason == GuardReasonAuthInvalid || state.CooldownUntil == nil || now.Before(*state.CooldownUntil)) {
		decision.Block = true
		return decision, nil
	}
	acquired, err := s.repository.AcquireHalfOpenLease(ctx, state, now, now.Add(halfOpenLeaseDuration), manual)
	if err != nil {
		return GuardDecision{}, fmt.Errorf("acquire upstream half-open lease: %w", err)
	}
	if !acquired {
		decision.Block = true
		return decision, nil
	}
	decision.HalfOpen = true
	return decision, nil
}

// RequestRetry grants one real request permission without calling the provider.
func (s *Service) RequestRetry(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	state, err := s.Get(ctx, organizationID, credentialID)
	if err != nil {
		return nil, err
	}
	if state.BlockReason == "" {
		return nil, ErrRetryNotRequired
	}
	now := s.now()
	updated, err := s.repository.RequestManualRetry(ctx, state, now)
	if err != nil {
		return nil, fmt.Errorf("request upstream credential retry: %w", err)
	}
	if !updated {
		return nil, ErrRetryInProgress
	}
	return s.Get(ctx, organizationID, credentialID)
}

// EvaluateGuardReadOnly applies the same guard policy without acquiring a
// half-open lease. It is used by prechecks, which must never consume the one
// probe reserved for a real provider request.
func (s *Service) EvaluateGuardReadOnly(state *State, enforce bool) GuardDecision {
	if state == nil || state.BlockReason == "" {
		return GuardDecision{}
	}
	decision := GuardDecision{WouldGuard: true, Reason: state.BlockReason}
	if enforce {
		decision.Block = true
	}
	return decision
}

// EvaluateProbeEligibility determines whether a blocked credential may be
// considered for one real recovery request without acquiring its lease.
func (s *Service) EvaluateProbeEligibility(state *State, enforce, allowAutomaticHalfOpen bool) (eligible, requiresBackup bool) {
	if state == nil || state.BlockReason == "" || !enforce {
		return false, false
	}
	now := s.now()
	if state.HalfOpenLeaseUntil != nil && now.Before(*state.HalfOpenLeaseUntil) {
		return false, false
	}
	if state.ManualRetryRequestedAt != nil {
		return true, false
	}
	if !allowAutomaticHalfOpen || state.BlockReason == GuardReasonAuthInvalid || state.CooldownUntil == nil || now.Before(*state.CooldownUntil) {
		return false, false
	}
	return true, true
}

func (s *Service) RecordProviderError(
	ctx context.Context,
	organizationID, credentialID uuid.UUID,
	generation int64,
	provider string,
	halfOpen bool,
	providerErr error,
) (bool, error) {
	if generation <= 0 || providerErr == nil {
		return false, nil
	}
	state, err := s.Get(ctx, organizationID, credentialID)
	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			return false, nil
		}
		return false, err
	}
	if state.Generation != generation {
		return false, nil
	}

	reason, availability, adapterErr, precise := classifyProviderGuardError(provider, providerErr)
	now := s.now()
	if !precise {
		if !halfOpen {
			return false, nil
		}
		updated, err := s.repository.Update(ctx, state, map[string]any{
			"cooldown_until":            now.Add(5 * time.Minute),
			"half_open_lease_until":     nil,
			"manual_retry_requested_at": nil,
			"updated_at":                now,
		})
		if err != nil {
			return false, fmt.Errorf("save upstream half-open retry: %w", err)
		}
		return false, staleUpdateError(updated)
	}

	strikes := 1
	var cooldownUntil any
	if reason != GuardReasonAuthInvalid {
		nextStrikes, cooldown := nextGuardCooldown(state, reason, halfOpen, now)
		strikes = nextStrikes
		cooldownUntil = cooldown
	}
	updates := map[string]any{
		"availability":              availability,
		"observation_source":        ObservationSourceProviderError,
		"availability_observed_at":  now,
		"block_reason":              reason,
		"cooldown_until":            cooldownUntil,
		"guard_strikes":             strikes,
		"half_open_lease_until":     nil,
		"manual_retry_requested_at": nil,
		"provider_error_code":       sanitizeProviderErrorCode(adapterErr.Code),
		"provider_error_status":     adapterErr.StatusCode,
		"updated_at":                now,
	}
	if providerHasNoBalanceAPI(provider) {
		updates["balance_capability"] = BalanceCapabilityUnsupported
		updates["next_check_at"] = nil
	}
	updated, err := s.repository.Update(ctx, state, updates)
	if err != nil {
		return false, fmt.Errorf("save precise upstream provider error: %w", err)
	}
	return true, staleUpdateError(updated)
}

func sanitizeProviderErrorCode(code string) string {
	codeRunes := []rune(strings.TrimSpace(code))
	if len(codeRunes) > 128 {
		codeRunes = codeRunes[:128]
	}
	return string(codeRunes)
}

func providerHasNoBalanceAPI(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qwen", "alibaba", "dashscope", "aliyun":
		return true
	default:
		return false
	}
}

func (s *Service) RecordProviderSuccess(ctx context.Context, organizationID, credentialID uuid.UUID, generation int64) error {
	if generation <= 0 {
		return nil
	}
	state, err := s.Get(ctx, organizationID, credentialID)
	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			return nil
		}
		return err
	}
	if state.Generation != generation {
		return nil
	}
	now := s.now()
	updated, err := s.repository.Update(ctx, state, map[string]any{
		"availability":              AvailabilityAvailable,
		"observation_source":        ObservationSourceProviderError,
		"availability_observed_at":  now,
		"block_reason":              "",
		"cooldown_until":            nil,
		"guard_strikes":             0,
		"half_open_lease_until":     nil,
		"manual_retry_requested_at": nil,
		"provider_error_code":       "",
		"provider_error_status":     0,
		"updated_at":                now,
	})
	if err != nil {
		return fmt.Errorf("clear upstream credential guard: %w", err)
	}
	return staleUpdateError(updated)
}

func classifyProviderGuardError(provider string, providerErr error) (GuardReason, Availability, *adapter.AdapterError, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	var adapterErr *adapter.AdapterError
	if !errors.As(providerErr, &adapterErr) {
		return "", AvailabilityUnknown, nil, false
	}
	switch provider {
	case "qwen", "alibaba", "dashscope", "aliyun":
		normalizedCode := strings.ToLower(strings.TrimSpace(adapterErr.Code))
		switch {
		case normalizedCode == "arrearage":
			return GuardReasonBillingUnavailable, AvailabilityExhausted, adapterErr, true
		case normalizedCode == "prepaidbilloverdue" || normalizedCode == "postpaidbilloverdue":
			return GuardReasonBillingUnavailable, AvailabilityExhausted, adapterErr, true
		case normalizedCode == "allocationquota.freetieronly":
			return GuardReasonQuotaExhausted, AvailabilityExhausted, adapterErr, true
		case normalizedCode == "invalidapikey" && adapterErr.StatusCode == http.StatusUnauthorized && errors.Is(providerErr, adapter.ErrAuthFailed):
			return GuardReasonAuthInvalid, AvailabilityInvalidKey, adapterErr, true
		}
	}
	return "", AvailabilityUnknown, nil, false
}

func nextGuardCooldown(state *State, reason GuardReason, halfOpen bool, now time.Time) (int, time.Time) {
	strikes := 1
	if state != nil && state.BlockReason == reason && state.GuardStrikes > 0 {
		strikes = state.GuardStrikes
		if halfOpen || state.CooldownUntil == nil || !now.Before(*state.CooldownUntil) {
			strikes++
		}
	}
	duration := 30 * time.Minute
	if strikes == 2 {
		duration = 2 * time.Hour
	} else if strikes >= 3 {
		duration = 6 * time.Hour
	}
	return strikes, now.Add(duration)
}

func staleUpdateError(updated bool) error {
	if !updated {
		return ErrStaleGeneration
	}
	return nil
}
