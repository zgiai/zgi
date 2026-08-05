package gateway

import (
	"context"
	"fmt"
	"io"
	"strings"

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

// GenerateSpeech authorizes and routes one MP3 stream to Console's metered TTS endpoint.
func (s *llmGatewayServiceImpl) GenerateSpeech(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	request *SpeechRequest,
	dst io.Writer,
) error {
	if request == nil ||
		strings.TrimSpace(request.Input) == "" ||
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
	shadowOrganizationID, _, err := s.resolveShadowContext(ctx, organizationID)
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
		return fmt.Errorf("failed to select speech provider: %w", err)
	}
	if len(selections) == 0 {
		return ErrNoProviderAvailable
	}
	if !selections[0].Model.SpeechGeneration {
		return fmt.Errorf("%w: model %q does not support speech generation", adapter.ErrCapabilityUnsupported, request.Model)
	}

	requestID := uuid.NewString()
	for _, selection := range selections {
		if selection == nil || !selection.UseSystemProvider {
			continue
		}
		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(selection, organizationID))
		if err != nil {
			return fmt.Errorf("failed to create speech adapter: %w", err)
		}
		speechAdapter, ok := providerAdapter.(adapter.SpeechCapable)
		if !ok {
			continue
		}

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

	return fmt.Errorf("%w: no official speech adapter is available", adapter.ErrCapabilityUnsupported)
}
