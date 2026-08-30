package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const (
	maxTranscriptionRouteCandidates = 3
	transcriptionSampleRate         = 16000
	transcriptionBitsPerSample      = 16
	transcriptionChannels           = 1
	transcriptionMaxDuration        = 60 * time.Second
)

var ErrTranscriptionAudioTooLong = fmt.Errorf("%w: transcription audio exceeds maximum duration", adapter.ErrInvalidRequest)

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

type clientReadTracker struct {
	src     io.Reader
	readErr error
}

func (r *clientReadTracker) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.readErr == nil {
		r.readErr = err
	}
	return n, err
}

// Transcribe routes one PCM stream through the selected official or private channel.
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
	shadowOrganizationID, ownerID, err := s.resolveShadowContext(ctx, organizationID)
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
		reportLLMSelectionFailure(ctx, err, request.Model, organizationID.String(), shadowOrganizationID.String())
		return nil, fmt.Errorf("failed to select transcription provider: %w", err)
	}
	if len(selections) == 0 {
		return nil, reportedNoProviderAvailableError(ctx, request.Model, organizationID.String(), shadowOrganizationID.String())
	}
	if !selections[0].Model.Transcription {
		return nil, fmt.Errorf("%w: model %q does not support transcription", adapter.ErrCapabilityUnsupported, request.Model)
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
			return nil, fmt.Errorf("failed to create transcription adapter: %w", err)
		}
		transcriptionAdapter, ok := providerAdapter.(adapter.TranscriptionCapable)
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
			return &TranscriptionResponse{RequestID: result.RequestID, Text: result.Text}, nil
		}

		estimatedUsage := transcriptionMeteredUsage(transcriptionMaxDuration.Milliseconds())
		estimatedQuote, err := s.quoteMeteredPricing(ctx, selection, estimatedUsage)
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
			estimatedQuote.TotalCredits,
			true,
			startedAt,
			requestID,
			buildAttemptID(requestID, attemptIndex),
		)
		if err != nil {
			return nil, err
		}
		if err := s.activateUpstreamProbeForAttempt(ctx, selection, billingCtx); err != nil {
			return nil, err
		}
		source := &clientReadTracker{src: request.Audio}
		meteredAudio := newTranscriptionMeteredReader(source, transcriptionMaxAudioBytes())
		result, err := transcriptionAdapter.Transcribe(ctx, &adapter.TranscriptionRequest{
			RequestID: requestID,
			Model:     request.Model,
			Audio:     meteredAudio,
		})
		responseTime := time.Since(startedAt).Milliseconds()
		if err == nil && result == nil {
			err = fmt.Errorf("%w: transcription provider returned no response", adapter.ErrUpstreamError)
		}
		if err != nil {
			setBillingFailure(billingCtx, err)
			billingCtx.ResponseTime = responseTime
			clientReadFailure := source.readErr != nil && errors.Is(err, source.readErr)
			clientLimitFailure := errors.Is(err, ErrTranscriptionAudioTooLong)
			if !clientReadFailure && !clientLimitFailure {
				s.recordUpstreamProviderError(ctx, selection, billingCtx, err)
			}
			resultErr := errors.Join(err, s.rollbackPreDeduction(ctx, billingCtx))
			if clientReadFailure {
				return nil, NewClientIOError(resultErr)
			}
			return nil, resultErr
		}
		s.recordUpstreamProviderSuccess(ctx, selection, billingCtx)
		actualUsage := transcriptionMeteredUsage(transcriptionMilliseconds(meteredAudio.BytesRead()))
		actualQuote, err := repriceLockedMeteredQuote(estimatedQuote, actualUsage)
		if err != nil {
			return nil, errors.Join(err, s.rollbackPreDeduction(ctx, billingCtx))
		}
		if err := s.settlePrivateMeteredSuccess(ctx, billingCtx, selection, actualQuote, responseTime); err != nil {
			return nil, err
		}
		return &TranscriptionResponse{RequestID: result.RequestID, Text: result.Text}, nil
	}

	return nil, audioCapabilityError("transcription", request.Model)
}

type transcriptionMeteredReader struct {
	source    io.Reader
	remaining int64
	bytesRead int64
}

func newTranscriptionMeteredReader(source io.Reader, maxBytes int64) *transcriptionMeteredReader {
	return &transcriptionMeteredReader{source: source, remaining: maxBytes}
}

func (r *transcriptionMeteredReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	limit := int64(len(dst))
	if limit > r.remaining+1 {
		limit = r.remaining + 1
	}
	read, err := r.source.Read(dst[:limit])
	if int64(read) > r.remaining {
		return 0, ErrTranscriptionAudioTooLong
	}
	r.remaining -= int64(read)
	r.bytesRead += int64(read)
	return read, err
}

func (r *transcriptionMeteredReader) BytesRead() int64 {
	return r.bytesRead
}

func (r *transcriptionMeteredReader) Close() error {
	if closer, ok := r.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func transcriptionMaxAudioBytes() int64 {
	bytesPerSecond := int64(transcriptionSampleRate * (transcriptionBitsPerSample / 8) * transcriptionChannels)
	return bytesPerSecond * transcriptionMaxDuration.Milliseconds() / int64(time.Second/time.Millisecond)
}

func transcriptionMilliseconds(audioBytes int64) int64 {
	if audioBytes <= 0 {
		return 0
	}
	bytesPerSecond := int64(transcriptionSampleRate * (transcriptionBitsPerSample / 8) * transcriptionChannels)
	return (audioBytes*1000 + bytesPerSecond - 1) / bytesPerSecond
}

func transcriptionMeteredUsage(quantity int64) MeteredUsage {
	return MeteredUsage{
		Operation: PricingOperationTranscription,
		Meter:     meterInputAudioDuration,
		BaseUnit:  baseUnitMillisecond,
		Quantity:  quantity,
		Dimensions: map[string]string{
			"mode": "streaming_input",
		},
	}
}
