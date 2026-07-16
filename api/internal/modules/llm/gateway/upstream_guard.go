package gateway

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type upstreamCredentialEvidence struct {
	organizationID uuid.UUID
	credentialID   uuid.UUID
	generation     int64
	provider       string
	wouldGuard     bool
	halfOpen       bool
}

func (s *llmGatewayServiceImpl) activateUpstreamProbe(
	ctx context.Context,
	selection *ProviderSelection,
	billingCtx *BillingContext,
) error {
	if selection == nil || !selection.UpstreamProbe {
		return nil
	}
	if s == nil || s.upstreamState == nil || selection.OrganizationID == uuid.Nil || selection.CredentialID == uuid.Nil || selection.CredentialGeneration <= 0 {
		return fmt.Errorf("%w: upstream probe state is unavailable", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	}
	state, err := s.upstreamState.Get(ctx, selection.OrganizationID, selection.CredentialID)
	if err != nil {
		return fmt.Errorf("load upstream probe state: %w", err)
	}
	if state.Generation != selection.CredentialGeneration {
		return fmt.Errorf("%w: credential generation changed", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	}

	decision, err := s.upstreamState.EvaluateGuard(ctx, state, true, selection.UpstreamProbeHasBackup)
	if err != nil {
		return err
	}
	if decision.Block || !decision.HalfOpen {
		return fmt.Errorf("%w: half-open lease unavailable", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	}
	selection.UpstreamHalfOpen = true
	selection.UpstreamWouldGuard = decision.WouldGuard
	if billingCtx != nil {
		billingCtx.UpstreamHalfOpen = true
		billingCtx.UpstreamWouldGuard = decision.WouldGuard
	}
	upstreamstate.RecordHalfOpenMetric(ctx, selection.ChannelProvider, "leased")
	return nil
}

func (s *llmGatewayServiceImpl) activateUpstreamProbeForAttempt(
	ctx context.Context,
	selection *ProviderSelection,
	billingCtx *BillingContext,
) error {
	if err := s.activateUpstreamProbe(ctx, selection, billingCtx); err != nil {
		if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
			return rollbackErr
		}
		return err
	}
	return nil
}

func upstreamEvidence(providerSelection *ProviderSelection, billingCtx *BillingContext) upstreamCredentialEvidence {
	if providerSelection != nil {
		return upstreamCredentialEvidence{
			organizationID: providerSelection.OrganizationID,
			credentialID:   providerSelection.CredentialID,
			generation:     providerSelection.CredentialGeneration,
			provider:       providerSelection.ChannelProvider,
			wouldGuard:     providerSelection.UpstreamWouldGuard,
			halfOpen:       providerSelection.UpstreamHalfOpen,
		}
	}
	if billingCtx == nil {
		return upstreamCredentialEvidence{}
	}
	organizationID, _ := uuid.Parse(billingCtx.OrganizationID)
	return upstreamCredentialEvidence{
		organizationID: organizationID,
		credentialID:   billingCtx.CredentialID,
		generation:     billingCtx.CredentialGeneration,
		provider:       billingCtx.ChannelProvider,
		wouldGuard:     billingCtx.UpstreamWouldGuard,
		halfOpen:       billingCtx.UpstreamHalfOpen,
	}
}

func (s *llmGatewayServiceImpl) recordUpstreamProviderError(
	ctx context.Context,
	providerSelection *ProviderSelection,
	billingCtx *BillingContext,
	providerErr error,
) {
	if s == nil || s.upstreamState == nil {
		return
	}
	evidence := upstreamEvidence(providerSelection, billingCtx)
	if evidence.organizationID == uuid.Nil || evidence.credentialID == uuid.Nil || evidence.generation <= 0 {
		return
	}
	precise, err := s.upstreamState.RecordProviderError(
		ctx,
		evidence.organizationID,
		evidence.credentialID,
		evidence.generation,
		evidence.provider,
		evidence.halfOpen,
		providerErr,
	)
	if err != nil {
		logger.WarnContext(ctx, "failed to record upstream credential provider error", err,
			"credential_id", evidence.credentialID.String(),
			"provider", evidence.provider,
		)
		return
	}
	if evidence.halfOpen {
		outcome := "transient_failure"
		if precise {
			outcome = "precise_failure"
		}
		upstreamstate.RecordHalfOpenMetric(ctx, evidence.provider, outcome)
	}
	if precise {
		logger.InfoContext(ctx, "precise upstream credential error recorded",
			"credential_id", evidence.credentialID.String(),
			"provider", evidence.provider,
		)
	}
}

func (s *llmGatewayServiceImpl) recordUpstreamProviderSuccess(
	ctx context.Context,
	providerSelection *ProviderSelection,
	billingCtx *BillingContext,
) {
	if s == nil || s.upstreamState == nil {
		return
	}
	evidence := upstreamEvidence(providerSelection, billingCtx)
	if (!evidence.wouldGuard && !evidence.halfOpen) || evidence.organizationID == uuid.Nil || evidence.credentialID == uuid.Nil || evidence.generation <= 0 {
		return
	}
	if err := s.upstreamState.RecordProviderSuccess(ctx, evidence.organizationID, evidence.credentialID, evidence.generation); err != nil {
		logger.WarnContext(ctx, "failed to clear upstream credential guard after provider success", err,
			"credential_id", evidence.credentialID.String(),
			"provider", evidence.provider,
		)
		return
	}
	if evidence.halfOpen {
		upstreamstate.RecordHalfOpenMetric(ctx, evidence.provider, "success")
	}
}
