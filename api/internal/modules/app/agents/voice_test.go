package agents

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
)

func TestVoiceServiceTranscribesWithDefaultSpeechToTextModel(t *testing.T) {
	resolver := &voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{
		Provider: "doubao",
		Model:    "volc.seedasr.sauc.duration",
	}}
	transcriber := &voiceTranscriberStub{response: &adapter.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "editable transcript",
	}}
	service := NewVoiceService(resolver, transcriber)
	audio := []byte{1, 2, 3, 4}

	result, err := service.Transcribe(t.Context(), uuid.NewString(), bytes.NewReader(audio))
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if result.Text != "editable transcript" {
		t.Fatalf("transcript = %q, want editable transcript", result.Text)
	}
	if resolver.useCase != llmmodel.UseCaseSpeechToText {
		t.Fatalf("use case = %q, want %q", resolver.useCase, llmmodel.UseCaseSpeechToText)
	}
	if transcriber.organizationID == "" || transcriber.request == nil {
		t.Fatalf("transcriber call = organization %q, request %#v", transcriber.organizationID, transcriber.request)
	}
	if transcriber.request.Model != "volc.seedasr.sauc.duration" {
		t.Fatalf("model = %q, want volc.seedasr.sauc.duration", transcriber.request.Model)
	}
	if got, readErr := io.ReadAll(transcriber.request.Audio); readErr != nil || !bytes.Equal(got, audio) {
		t.Fatalf("audio = %v, %v; want %v", got, readErr, audio)
	}
}

func TestVoiceServiceRejectsEmptyTranscriptAsNoSpeech(t *testing.T) {
	service := NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		&voiceTranscriberStub{response: &adapter.TranscriptionResponse{RequestID: uuid.NewString(), Text: "  "}},
	)

	_, err := service.Transcribe(t.Context(), uuid.NewString(), bytes.NewReader([]byte{0, 0}))
	if !errors.Is(err, ErrNoSpeechDetected) {
		t.Fatalf("Transcribe() error = %v, want ErrNoSpeechDetected", err)
	}
}

func TestVoiceServiceAvailabilityRequiresDefaultSpeechToTextModel(t *testing.T) {
	organizationID := uuid.NewString()
	tests := []struct {
		name      string
		model     *llmdefaultservice.ResolvedModel
		available bool
	}{
		{name: "configured", model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}, available: true},
		{name: "missing", model: nil, available: false},
		{name: "empty", model: &llmdefaultservice.ResolvedModel{}, available: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewVoiceService(
				&voiceModelResolverStub{model: test.model},
				&voiceTranscriberStub{},
			)

			available, err := service.Available(t.Context(), organizationID)

			if err != nil {
				t.Fatalf("Available() error = %v", err)
			}
			if available != test.available {
				t.Fatalf("Available() = %v, want %v", available, test.available)
			}
		})
	}
}

type voiceModelResolverStub struct {
	model   *llmdefaultservice.ResolvedModel
	err     error
	useCase llmmodel.UseCase
}

func (s *voiceModelResolverStub) ResolveUseCase(_ context.Context, _ string, useCase llmmodel.UseCase, _, _ *string) (*llmdefaultservice.ResolvedModel, error) {
	s.useCase = useCase
	return s.model, s.err
}

func (s *voiceModelResolverStub) ResolveModelType(_ context.Context, _ string, _, _ *string, _ sharedmodel.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return s.model, s.err
}

type voiceTranscriberStub struct {
	organizationID string
	request        *adapter.TranscriptionRequest
	response       *adapter.TranscriptionResponse
	err            error
	calls          int
	calledAt       time.Time
	deadline       time.Time
	hasDeadline    bool
}

func (s *voiceTranscriberStub) Transcribe(ctx context.Context, organizationID string, request *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error) {
	s.calls++
	s.organizationID = organizationID
	s.request = request
	s.calledAt = time.Now()
	s.deadline, s.hasDeadline = ctx.Deadline()
	return s.response, s.err
}
