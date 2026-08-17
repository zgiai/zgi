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
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
)

func TestSpeechServiceGeneratesWithDefaultTextToSpeechModel(t *testing.T) {
	resolver := &voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{
		Provider: "doubao",
		Model:    "seed-tts-2.0",
		Params: llmsharedtypes.JSONObject{
			"default_voice": " verified-voice ",
		},
	}}
	synthesizer := &voiceSynthesizerStub{audio: []byte("mp3")}
	service := NewSpeechService(resolver, synthesizer)
	var audio bytes.Buffer

	err := service.Generate(t.Context(), uuid.NewString(), "Readable answer", &audio)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resolver.useCase != llmmodel.UseCaseTextToSpeech {
		t.Fatalf("use case = %q, want %q", resolver.useCase, llmmodel.UseCaseTextToSpeech)
	}
	if synthesizer.organizationID == "" || synthesizer.request == nil {
		t.Fatalf("synthesizer call = organization %q, request %#v", synthesizer.organizationID, synthesizer.request)
	}
	if synthesizer.request.Model != "seed-tts-2.0" {
		t.Fatalf("model = %q, want seed-tts-2.0", synthesizer.request.Model)
	}
	if synthesizer.request.Input != "Readable answer" || synthesizer.request.Voice != "verified-voice" {
		t.Fatalf("speech request = %#v, want input and verified voice", synthesizer.request)
	}
	if synthesizer.request.ResponseFormat != "mp3" {
		t.Fatalf("response format = %q, want mp3", synthesizer.request.ResponseFormat)
	}
	if audio.String() != "mp3" {
		t.Fatalf("audio = %q, want mp3", audio.String())
	}
}

func TestSpeechServiceRejectsModelWithoutDefaultVoice(t *testing.T) {
	resolver := &voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "seed-tts-2.0"}}
	synthesizer := &voiceSynthesizerStub{}
	service := NewSpeechService(resolver, synthesizer)

	err := service.Generate(t.Context(), uuid.NewString(), "answer", io.Discard)

	if !errors.Is(err, ErrSpeechUnavailable) {
		t.Fatalf("Generate() error = %v, want %v", err, ErrSpeechUnavailable)
	}
	if resolver.useCase != llmmodel.UseCaseTextToSpeech {
		t.Fatalf("resolver use case = %q, want %q", resolver.useCase, llmmodel.UseCaseTextToSpeech)
	}
	if synthesizer.calls != 0 {
		t.Fatalf("synthesizer calls = %d, want 0", synthesizer.calls)
	}
}

func TestSpeechServiceAvailabilityRequiresTrustedDefaultVoice(t *testing.T) {
	organizationID := uuid.NewString()
	tests := []struct {
		name      string
		params    llmsharedtypes.JSONObject
		available bool
	}{
		{name: "configured", params: llmsharedtypes.JSONObject{"default_voice": "voice-id"}, available: true},
		{name: "missing", params: llmsharedtypes.JSONObject{}, available: false},
		{name: "wrong type", params: llmsharedtypes.JSONObject{"default_voice": true}, available: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewSpeechService(&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{
				Model:  "seed-tts-2.0",
				Params: test.params,
			}}, &voiceSynthesizerStub{})

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

type voiceSynthesizerStub struct {
	organizationID string
	request        *adapter.SpeechRequest
	audio          []byte
	err            error
	calls          int
	deadline       time.Time
	hasDeadline    bool
}

func (s *voiceSynthesizerStub) GenerateSpeech(ctx context.Context, organizationID string, request *adapter.SpeechRequest, dst io.Writer) error {
	s.calls++
	s.organizationID = organizationID
	s.request = request
	s.deadline, s.hasDeadline = ctx.Deadline()
	if len(s.audio) > 0 {
		_, _ = dst.Write(s.audio)
	}
	return s.err
}
