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

const maxTranscriptionRouteCandidates = 3

// TranscriptionRequest carries one non-replayable PCM stream.
type TranscriptionRequest struct {
	Model string
	Audio io.Reader
}

// TranscriptionResponse contains the final text that clients may edit before sending.
type TranscriptionResponse struct {
	RequestID string `json:"request_id"`
	Text      string `json:"text"`
}

// Transcribe authorizes and routes one PCM stream to Console's metered STT endpoint.
func (s *llmGatewayServiceImpl) Transcribe(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	request *TranscriptionRequest,
) (*TranscriptionResponse, error) {
	if request == nil || request.Audio == nil {
		return nil, fmt.Errorf("%w: audio is required", adapter.ErrInvalidRequest)
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
	shadowOrganizationID, _, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve billing organization: %w", err)
	}

	ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, string(llmmodel.UseCaseSpeechToText))
	selections, err := s.selectProvidersWithChannelRouter(
		ctx,
		shadowOrganizationID,
		"",
		request.Model,
		maxTranscriptionRouteCandidates,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to select transcription provider: %w", err)
	}
	if len(selections) == 0 {
		return nil, ErrNoProviderAvailable
	}
	if !selections[0].Model.Transcription {
		return nil, fmt.Errorf("%w: model %q does not support transcription", adapter.ErrCapabilityUnsupported, request.Model)
	}

	requestID := uuid.NewString()
	for _, selection := range selections {
		if selection == nil || !selection.UseSystemProvider {
			continue
		}
		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(selection, organizationID))
		if err != nil {
			return nil, fmt.Errorf("failed to create transcription adapter: %w", err)
		}
		transcriptionAdapter, ok := providerAdapter.(adapter.TranscriptionCapable)
		if !ok {
			continue
		}

		callContext := context.WithValue(ctx, platformProxyContextKey{}, platformProxyMetadata{
			BillingOrganizationID: shadowOrganizationID.String(),
			RequestID:             requestID,
			APIKeyID:              strings.TrimSpace(apiKey.ID),
			ModelName:             request.Model,
			ProviderName:          selection.Provider.Provider,
		})
		result, err := transcriptionAdapter.Transcribe(callContext, &adapter.TranscriptionRequest{
			RequestID: requestID,
			Model:     request.Model,
			Audio:     request.Audio,
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("%w: transcription provider returned no response", adapter.ErrUpstreamError)
		}
		return &TranscriptionResponse{
			RequestID: result.RequestID,
			Text:      result.Text,
		}, nil
	}

	return nil, fmt.Errorf("%w: no official transcription adapter is available", adapter.ErrCapabilityUnsupported)
}
