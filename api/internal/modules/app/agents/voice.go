package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

var (
	ErrVoiceUnavailable = errors.New("speech-to-text model is unavailable")
	ErrNoSpeechDetected = errors.New("no speech was detected")
)

// VoiceTranscriber is the model capability consumed by the Agent voice service.
type VoiceTranscriber interface {
	Transcribe(context.Context, string, *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error)
}

// VoiceService resolves the organization's STT model and returns final editable text.
type VoiceService struct {
	models      llmdefaultservice.DefaultModelResolver
	transcriber VoiceTranscriber
}

// NewVoiceService creates the Agent speech-to-text service.
func NewVoiceService(models llmdefaultservice.DefaultModelResolver, transcriber VoiceTranscriber) *VoiceService {
	if models == nil || transcriber == nil {
		panic("agent voice service requires model resolver and transcriber")
	}
	return &VoiceService{models: models, transcriber: transcriber}
}

// Available reports whether the organization has a usable default STT model.
func (s *VoiceService) Available(ctx context.Context, organizationID string) (bool, error) {
	_, err := s.resolveModel(ctx, organizationID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrVoiceUnavailable) ||
		errors.Is(err, llmdefaultservice.ErrModelUnavailable) ||
		errors.Is(err, llmdefaultservice.ErrDefaultModelNotFound) {
		return false, nil
	}
	return false, err
}

// Transcribe resolves the default STT model and consumes one complete PCM recording.
func (s *VoiceService) Transcribe(ctx context.Context, organizationID string, audio io.Reader) (*adapter.TranscriptionResponse, error) {
	if _, err := uuid.Parse(strings.TrimSpace(organizationID)); err != nil || audio == nil {
		return nil, fmt.Errorf("%w: organization and audio are required", adapter.ErrInvalidRequest)
	}
	model, err := s.resolveModel(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	result, err := s.transcriber.Transcribe(ctx, organizationID, &adapter.TranscriptionRequest{
		Model: model,
		Audio: audio,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: transcription returned no response", adapter.ErrUpstreamError)
	}
	if strings.TrimSpace(result.Text) == "" {
		return nil, ErrNoSpeechDetected
	}
	return result, nil
}

func (s *VoiceService) resolveModel(ctx context.Context, organizationID string) (string, error) {
	if _, err := uuid.Parse(strings.TrimSpace(organizationID)); err != nil {
		return "", fmt.Errorf("%w: organization is required", adapter.ErrInvalidRequest)
	}
	model, err := s.models.ResolveUseCase(ctx, organizationID, llmmodel.UseCaseSpeechToText, nil, nil)
	if err != nil {
		return "", fmt.Errorf("resolve speech-to-text model: %w", err)
	}
	if model == nil || strings.TrimSpace(model.Model) == "" {
		return "", ErrVoiceUnavailable
	}
	return strings.TrimSpace(model.Model), nil
}
