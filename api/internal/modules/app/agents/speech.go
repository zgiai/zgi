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
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
)

const agentSpeechResponseFormat = "mp3"

var ErrSpeechUnavailable = errors.New("text-to-speech model is unavailable")

// VoiceSynthesizer is the model capability consumed by Agent speech playback.
type VoiceSynthesizer interface {
	GenerateSpeech(context.Context, string, *adapter.SpeechRequest, io.Writer) error
}

// SpeechService resolves the organization's TTS model and streams one MP3 response.
type SpeechService struct {
	models      llmdefaultservice.DefaultModelResolver
	synthesizer VoiceSynthesizer
}

// NewSpeechService creates the Agent text-to-speech service.
func NewSpeechService(models llmdefaultservice.DefaultModelResolver, synthesizer VoiceSynthesizer) *SpeechService {
	if models == nil || synthesizer == nil {
		panic("agent speech service requires model resolver and synthesizer")
	}
	return &SpeechService{models: models, synthesizer: synthesizer}
}

type speechTarget struct {
	model string
	voice string
}

// Available reports whether the organization has a TTS model with a trusted default voice.
func (s *SpeechService) Available(ctx context.Context, organizationID string) (bool, error) {
	_, err := s.resolveTarget(ctx, organizationID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrSpeechUnavailable) ||
		errors.Is(err, llmdefaultservice.ErrModelUnavailable) ||
		errors.Is(err, llmdefaultservice.ErrDefaultModelNotFound) {
		return false, nil
	}
	return false, err
}

// Generate resolves the default TTS model and streams one complete text input as MP3.
func (s *SpeechService) Generate(ctx context.Context, organizationID, input string, dst io.Writer) error {
	if _, err := uuid.Parse(strings.TrimSpace(organizationID)); err != nil ||
		strings.TrimSpace(input) == "" ||
		dst == nil {
		return fmt.Errorf("%w: organization, input, and destination are required", adapter.ErrInvalidRequest)
	}
	target, err := s.resolveTarget(ctx, organizationID)
	if err != nil {
		return err
	}
	return s.synthesizer.GenerateSpeech(ctx, organizationID, &adapter.SpeechRequest{
		Model:          target.model,
		Input:          input,
		Voice:          target.voice,
		ResponseFormat: agentSpeechResponseFormat,
	}, dst)
}

func (s *SpeechService) resolveTarget(ctx context.Context, organizationID string) (*speechTarget, error) {
	if _, err := uuid.Parse(strings.TrimSpace(organizationID)); err != nil {
		return nil, fmt.Errorf("%w: organization is required", adapter.ErrInvalidRequest)
	}
	model, err := s.models.ResolveUseCase(ctx, organizationID, llmmodel.UseCaseTextToSpeech, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve text-to-speech model: %w", err)
	}
	if model == nil || strings.TrimSpace(model.Model) == "" {
		return nil, ErrSpeechUnavailable
	}
	voice, ok := model.Params[string(sharedmodel.ModelPropertyKeyDefaultVoice)].(string)
	voice = strings.TrimSpace(voice)
	if !ok || voice == "" {
		return nil, ErrSpeechUnavailable
	}
	return &speechTarget{model: strings.TrimSpace(model.Model), voice: voice}, nil
}
