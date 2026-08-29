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
	appconfig "github.com/zgiai/zgi/api/config"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const (
	maxMusicRouteCandidates = 3
	musicResponseFormatMP3  = "mp3"
	meterOutputTrack        = "output_track"
	meterRequest            = "request"
	baseUnitTrack           = "track"
	baseUnitRequest         = "request"
)

// MusicRequest carries a stable request ID because the same identity is used
// for generation billing and any later delivery compensation.
type MusicRequest struct {
	RequestID      string            `json:"-"`
	Model          string            `json:"model"`
	Mode           adapter.MusicMode `json:"mode"`
	Prompt         string            `json:"prompt"`
	Lyrics         string            `json:"lyrics"`
	ResponseFormat string            `json:"response_format"`
}

// LyricsRequest carries one complete-song lyrics request over the same model
// route used by music generation.
type LyricsRequest struct {
	RequestID string `json:"-"`
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
}

// GenerateLyrics authorizes and routes one lyrics request through an official or private channel.
func (s *llmGatewayServiceImpl) GenerateLyrics(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	request *LyricsRequest,
) (*adapter.LyricsResult, error) {
	if err := validateLyricsGatewayRequest(request); err != nil {
		return nil, err
	}
	request.Model = normalizeRequestedModelName(request.Model)
	if request.Model == "" {
		return nil, ErrMissingModel
	}
	if err := s.checkModelAuthorization(apiKey, nil, request.Model); err != nil {
		return nil, err
	}

	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization ID: %w", err)
	}
	shadowOrganizationID, ownerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve billing organization: %w", err)
	}

	ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, string(llmmodel.UseCaseMusicGen))
	selections, err := s.selectProvidersWithChannelRouter(
		ctx,
		shadowOrganizationID,
		"",
		request.Model,
		maxMusicRouteCandidates,
	)
	if err != nil {
		reportLLMSelectionFailure(ctx, err, request.Model, organizationID.String(), shadowOrganizationID.String())
		return nil, fmt.Errorf("failed to select lyrics provider: %w", err)
	}
	if len(selections) == 0 {
		return nil, reportedNoProviderAvailableError(ctx, request.Model, organizationID.String(), shadowOrganizationID.String())
	}
	if !selections[0].Model.MusicGeneration {
		return nil, fmt.Errorf("%w: model %q does not support music generation", adapter.ErrCapabilityUnsupported, request.Model)
	}

	startedAt := time.Now()
	for _, selection := range selections {
		if selection == nil {
			continue
		}
		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(selection, organizationID))
		if err != nil {
			reportLLMAdapterFailure(ctx, err, selection.Provider.Provider, selection.Model.Model, organizationID.String(), 0, getChannelID(selection), selection.UseSystemProvider, true)
			return nil, fmt.Errorf("failed to create lyrics adapter: %w", err)
		}
		lyricsAdapter, ok := providerAdapter.(adapter.LyricsCapable)
		if !ok {
			continue
		}
		callContext := ctx
		if selection.UseSystemProvider {
			callContext = context.WithValue(ctx, platformProxyContextKey{}, platformProxyMetadata{
				BillingOrganizationID: shadowOrganizationID.String(),
				RequestID:             request.RequestID,
				APIKeyID:              strings.TrimSpace(apiKey.ID),
				ModelName:             request.Model,
				ProviderName:          selection.Provider.Provider,
			})
			result, err := lyricsAdapter.GenerateLyrics(callContext, &adapter.LyricsRequest{
				RequestID: request.RequestID,
				Model:     request.Model,
				Prompt:    request.Prompt,
			})
			if err != nil {
				reportLLMProviderFailure(ctx, err, "llm.provider.request_failed", selection.Provider.Provider, selection.Model.Model, organizationID.String(), 0, getChannelID(selection), true, true)
			}
			return result, err
		}

		usage := MeteredUsage{Operation: PricingOperationLyrics, Meter: meterRequest, BaseUnit: baseUnitRequest, Quantity: 1}
		quote, err := s.quoteMeteredPricing(ctx, selection, usage)
		if err != nil {
			return nil, err
		}
		billingCtx, err := s.beginBillingAttempt(
			ctx,
			apiKey,
			nil,
			selection,
			shadowOrganizationID,
			ownerID,
			quote.TotalCredits,
			false,
			startedAt,
			request.RequestID,
			request.RequestID+":lyrics",
		)
		if err != nil {
			return nil, err
		}
		billingCtx.PricingOperation = PricingOperationLyrics
		if err := s.activateUpstreamProbeForAttempt(ctx, selection, billingCtx); err != nil {
			return nil, err
		}
		result, err := lyricsAdapter.GenerateLyrics(callContext, &adapter.LyricsRequest{
			RequestID: request.RequestID,
			Model:     request.Model,
			Prompt:    request.Prompt,
		})
		responseTime := time.Since(startedAt).Milliseconds()
		if err != nil {
			setBillingFailure(billingCtx, err)
			billingCtx.ResponseTime = responseTime
			s.recordUpstreamProviderError(ctx, selection, billingCtx, err)
			return nil, errors.Join(err, s.rollbackPreDeduction(ctx, billingCtx))
		}
		s.recordUpstreamProviderSuccess(ctx, selection, billingCtx)
		if err := s.settlePrivateMeteredSuccess(ctx, billingCtx, selection, quote, responseTime); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, audioCapabilityError("lyrics", request.Model)
}

// musicDestinationWriter remembers downstream write failures so they are not
// attributed to the selected model provider.
type musicDestinationWriter struct {
	dst      io.Writer
	writeErr error
}

func (w *musicDestinationWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	return n, err
}

// GenerateMusic authorizes and routes one complete MP3 stream through an official or private channel.
func (s *llmGatewayServiceImpl) GenerateMusic(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	request *MusicRequest,
	dst io.Writer,
) error {
	if err := validateMusicGatewayRequest(request, dst); err != nil {
		return err
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

	ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, string(llmmodel.UseCaseMusicGen))
	selections, err := s.selectProvidersWithChannelRouter(
		ctx,
		shadowOrganizationID,
		"",
		request.Model,
		maxMusicRouteCandidates,
	)
	if err != nil {
		reportLLMSelectionFailure(ctx, err, request.Model, organizationID.String(), shadowOrganizationID.String())
		return fmt.Errorf("failed to select music provider: %w", err)
	}
	if len(selections) == 0 {
		return reportedNoProviderAvailableError(ctx, request.Model, organizationID.String(), shadowOrganizationID.String())
	}
	if !selections[0].Model.MusicGeneration {
		return fmt.Errorf("%w: model %q does not support music generation", adapter.ErrCapabilityUnsupported, request.Model)
	}

	startedAt := time.Now()
	for _, selection := range selections {
		if selection == nil {
			continue
		}
		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(selection, organizationID))
		if err != nil {
			reportLLMAdapterFailure(ctx, err, selection.Provider.Provider, selection.Model.Model, organizationID.String(), 0, getChannelID(selection), selection.UseSystemProvider, true)
			return fmt.Errorf("failed to create music adapter: %w", err)
		}
		musicAdapter, ok := providerAdapter.(adapter.MusicCapable)
		if !ok {
			continue
		}
		callContext := ctx
		if selection.UseSystemProvider {
			callContext = context.WithValue(ctx, platformProxyContextKey{}, platformProxyMetadata{
				BillingOrganizationID: shadowOrganizationID.String(),
				RequestID:             request.RequestID,
				APIKeyID:              strings.TrimSpace(apiKey.ID),
				ModelName:             request.Model,
				ProviderName:          selection.Provider.Provider,
				IsStreaming:           true,
			})
		}
		destination := &musicDestinationWriter{dst: dst}
		adapterRequest := &adapter.MusicRequest{
			RequestID:      request.RequestID,
			Model:          request.Model,
			Mode:           request.Mode,
			Prompt:         request.Prompt,
			Lyrics:         request.Lyrics,
			ResponseFormat: request.ResponseFormat,
		}
		if selection.UseSystemProvider {
			err = musicAdapter.GenerateMusic(callContext, adapterRequest, destination)
			if err != nil && (destination.writeErr == nil || !errors.Is(err, destination.writeErr)) {
				reportLLMProviderFailure(ctx, err, "llm.provider.stream_failed", selection.Provider.Provider, selection.Model.Model, organizationID.String(), 0, getChannelID(selection), true, true)
			}
			return err
		}

		usage := MeteredUsage{Operation: PricingOperationMusic, Meter: meterOutputTrack, BaseUnit: baseUnitTrack, Quantity: 1}
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
			request.RequestID,
			request.RequestID,
		)
		if err != nil {
			return err
		}
		billingCtx.PricingOperation = PricingOperationMusic
		if err := s.activateUpstreamProbeForAttempt(ctx, selection, billingCtx); err != nil {
			return err
		}
		err = musicAdapter.GenerateMusic(callContext, adapterRequest, destination)
		responseTime := time.Since(startedAt).Milliseconds()
		if err != nil {
			setBillingFailure(billingCtx, err)
			billingCtx.ResponseTime = responseTime
			if destination.writeErr == nil || !errors.Is(err, destination.writeErr) {
				s.recordUpstreamProviderError(ctx, selection, billingCtx, err)
			}
			return errors.Join(err, s.rollbackPreDeduction(ctx, billingCtx))
		}
		s.recordUpstreamProviderSuccess(ctx, selection, billingCtx)
		return s.settlePrivateMeteredSuccess(ctx, billingCtx, selection, quote, responseTime)
	}
	return audioCapabilityError("music", request.Model)
}

// CompensateMusicDelivery deliberately bypasses model routing. A model or route
// may be disabled after generation, but that must never prevent a refund.
func (s *llmGatewayServiceImpl) CompensateMusicDelivery(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	requestID string,
) error {
	if apiKey == nil {
		return fmt.Errorf("%w: api key is required", adapter.ErrInvalidRequest)
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(apiKey.OrganizationID))
	if err != nil || organizationID == uuid.Nil {
		return fmt.Errorf("%w: organization id is invalid", adapter.ErrInvalidRequest)
	}
	if !isCanonicalMusicRequestID(requestID) {
		return fmt.Errorf("%w: request id is invalid", adapter.ErrInvalidRequest)
	}
	billingOrganizationID, _, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to resolve billing organization: %w", err)
	}
	privateBilling := s.localBilling
	if privateBilling == nil {
		privateBilling = s.billing
	}
	if compensator, ok := privateBilling.(privateMusicDeliveryCompensator); ok {
		err := compensator.CompensatePrivateMusicDelivery(ctx, billingOrganizationID, requestID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, adapter.ErrMusicCompensationNotFound) {
			return err
		}
	}
	baseURL, err := resolveOfficialRouteBaseURL()
	if err != nil {
		return err
	}
	llmConfig := appconfig.Current().LLM
	providerAdapter, err := s.adapterFactory.CreateAdapter(&adapter.AdapterConfig{
		ProviderName:        "zgi-cloud",
		BaseURL:             baseURL,
		Timeout:             30 * time.Second,
		MaxRetries:          3,
		GuardOutboundURL:    llmConfig.OutboundURLGuardEnabled(),
		GuardOutboundDNS:    llmConfig.GuardOutboundDNS,
		AllowPrivateBaseURL: true,
		AuthHook:            s.buildConsoleAuthHook(billingOrganizationID),
	})
	if err != nil {
		return fmt.Errorf("failed to create music compensation adapter: %w", err)
	}
	compensator, ok := providerAdapter.(adapter.MusicCompensationCapable)
	if !ok {
		return fmt.Errorf("%w: official adapter cannot compensate music delivery", adapter.ErrCapabilityUnsupported)
	}
	callContext := context.WithValue(ctx, platformProxyContextKey{}, platformProxyMetadata{
		BillingOrganizationID: billingOrganizationID.String(),
		RequestID:             requestID,
		APIKeyID:              strings.TrimSpace(apiKey.ID),
	})
	return compensator.CompensateMusicDelivery(callContext, requestID)
}

func validateMusicGatewayRequest(request *MusicRequest, dst io.Writer) error {
	if request == nil || dst == nil {
		return fmt.Errorf("%w: request and destination are required", adapter.ErrInvalidRequest)
	}
	if !isCanonicalMusicRequestID(request.RequestID) {
		return fmt.Errorf("%w: request id is invalid", adapter.ErrInvalidRequest)
	}
	if request.ResponseFormat != musicResponseFormatMP3 {
		return fmt.Errorf("%w: response format must be mp3", adapter.ErrInvalidRequest)
	}
	prompt := strings.TrimSpace(request.Prompt)
	lyrics := strings.TrimSpace(request.Lyrics)
	switch request.Mode {
	case adapter.MusicModeVocal:
		if lyrics == "" {
			return fmt.Errorf("%w: lyrics are required for vocal music", adapter.ErrInvalidRequest)
		}
	case adapter.MusicModeAutoLyrics, adapter.MusicModeInstrumental:
		if prompt == "" || lyrics != "" {
			return fmt.Errorf("%w: prompt is required and lyrics must be empty", adapter.ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: unsupported music mode", adapter.ErrInvalidRequest)
	}
	return nil
}

func isCanonicalMusicRequestID(requestID string) bool {
	parsed, err := uuid.Parse(requestID)
	return err == nil && parsed != uuid.Nil && requestID == parsed.String()
}

func validateLyricsGatewayRequest(request *LyricsRequest) error {
	if request == nil {
		return fmt.Errorf("%w: lyrics request is required", adapter.ErrInvalidRequest)
	}
	if !isCanonicalMusicRequestID(request.RequestID) {
		return fmt.Errorf("%w: request id is invalid", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("%w: model and prompt are required", adapter.ErrInvalidRequest)
	}
	if !utf8.ValidString(request.Prompt) || utf8.RuneCountInString(request.Prompt) > adapter.MaxMusicPromptRunes {
		return fmt.Errorf("%w: prompt is invalid", adapter.ErrInvalidRequest)
	}
	return nil
}
