package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const (
	maxSpeechRouteCandidates = 3
	speechResponseFormatMP3  = "mp3"
)

// SpeechRequest carries one complete text input for streamed speech generation.
type SpeechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// GenerateSpeech routes one MP3 stream through the selected official or private channel.
func (s *llmGatewayServiceImpl) GenerateSpeech(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	request *SpeechRequest,
	dst io.Writer,
) error {
	if request == nil ||
		strings.TrimSpace(request.Input) == "" ||
		!utf8.ValidString(request.Input) ||
		strings.TrimSpace(request.Voice) == "" ||
		strings.TrimSpace(request.ResponseFormat) != speechResponseFormatMP3 ||
		dst == nil {
		return fmt.Errorf("%w: input, voice, mp3 format, and destination are required", adapter.ErrInvalidRequest)
	}
	request.Model = normalizeRequestedModelName(request.Model)
	if request.Model == "" {
		return ErrMissingModel
	}
	if err := s.checkModelAuthorization(apiKey, nil, request.Model); err != nil {
		return err
	}

	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization ID: %w", err)
	}
	shadowOrganizationID, ownerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to resolve billing organization: %w", err)
	}

	ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, string(llmmodel.UseCaseTextToSpeech))
	selections, err := s.selectProvidersWithChannelRouter(
		ctx,
		shadowOrganizationID,
		"",
		request.Model,
		maxSpeechRouteCandidates,
	)
	if err != nil {
		reportLLMSelectionFailure(ctx, err, request.Model, organizationID.String(), shadowOrganizationID.String())
		return fmt.Errorf("failed to select speech provider: %w", err)
	}
	if len(selections) == 0 {
		return reportedNoProviderAvailableError(ctx, request.Model, organizationID.String(), shadowOrganizationID.String())
	}
	if !selections[0].Model.SpeechGeneration {
		return fmt.Errorf("%w: model %q does not support speech generation", adapter.ErrCapabilityUnsupported, request.Model)
	}

	requestID := uuid.NewString()
	startedAt := time.Now()
	for attemptIndex, selection := range selections {
		if selection == nil {
			continue
		}
		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(selection, organizationID))
		if err != nil {
			reportLLMAdapterFailure(ctx, err, selection.Provider.Provider, selection.Model.Model, organizationID.String(), 0, getChannelID(selection), true, true)
			return fmt.Errorf("failed to create speech adapter: %w", err)
		}
		speechAdapter, ok := providerAdapter.(adapter.SpeechCapable)
		if !ok {
			continue
		}

		if selection.UseSystemProvider {
			callContext := context.WithValue(ctx, platformProxyContextKey{}, platformProxyMetadata{
				BillingOrganizationID: shadowOrganizationID.String(),
				RequestID:             requestID,
				APIKeyID:              strings.TrimSpace(apiKey.ID),
				ModelName:             request.Model,
				ProviderName:          selection.Provider.Provider,
				IsStreaming:           true,
			})
			return speechAdapter.GenerateSpeech(callContext, &adapter.SpeechRequest{
				RequestID:      requestID,
				Model:          request.Model,
				Input:          request.Input,
				Voice:          request.Voice,
				ResponseFormat: request.ResponseFormat,
			}, dst)
		}

		usage := MeteredUsage{
			Operation: PricingOperationSpeech,
			Meter:     meterInputText,
			BaseUnit:  baseUnitBilledCharacter,
			Quantity:  int64(utf8.RuneCountInString(request.Input)),
		}
		quote, err := s.quoteMeteredPricing(ctx, selection, usage)
		if err != nil {
			return err
		}
		billingCtx, err := s.beginBillingAttempt(
			ctx,
			apiKey,
			nil,
			selection,
			shadowOrganizationID,
			ownerID,
			quote.TotalCredits,
			true,
			startedAt,
			requestID,
			buildAttemptID(requestID, attemptIndex),
		)
		if err != nil {
			return err
		}
		if err := s.activateUpstreamProbeForAttempt(ctx, selection, billingCtx); err != nil {
			return err
		}
		err = speechAdapter.GenerateSpeech(ctx, &adapter.SpeechRequest{
			RequestID:      requestID,
			Model:          request.Model,
			Input:          request.Input,
			Voice:          request.Voice,
			ResponseFormat: request.ResponseFormat,
		}, dst)
		responseTime := time.Since(startedAt).Milliseconds()
		if err != nil {
			setBillingFailure(billingCtx, err)
			billingCtx.ResponseTime = responseTime
			s.recordUpstreamProviderError(ctx, selection, billingCtx, err)
			return errors.Join(err, s.rollbackPreDeduction(ctx, billingCtx))
		}
		s.recordUpstreamProviderSuccess(ctx, selection, billingCtx)
		return s.settlePrivateMeteredSuccess(ctx, billingCtx, selection, quote, responseTime)
	}

	return audioCapabilityError("speech", request.Model)
}
