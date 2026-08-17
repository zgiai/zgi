package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
)

func TestResolveFromCandidatesPreservesModelLimits(t *testing.T) {
	svc := &defaultModelService{}
	resolved := svc.resolveFromCandidates(
		llmmodelmodel.UseCaseTextChat,
		nil,
		[]*rankedCandidate{
			{
				model: &llmmodelservice.AvailableModel{
					Provider:        "deepseek",
					Name:            "deepseek-chat",
					ContextWindow:   64000,
					MaxOutputTokens: 8192,
					UseCases:        []string{string(llmmodelmodel.UseCaseTextChat)},
				},
			},
		},
	)

	if resolved == nil {
		t.Fatal("resolved model is nil")
	}
	if got, want := resolved.ContextWindow, 64000; got != want {
		t.Fatalf("context window = %d, want %d", got, want)
	}
	if got, want := resolved.MaxOutputTokens, 8192; got != want {
		t.Fatalf("max output tokens = %d, want %d", got, want)
	}
}

func TestUpsertRejectsTextToSpeechWithoutDefaultVoice(t *testing.T) {
	tests := []struct {
		name   string
		params llmsharedtypes.JSONObject
	}{
		{name: "missing", params: llmsharedtypes.JSONObject{}},
		{name: "wrong type", params: llmsharedtypes.JSONObject{"default_voice": true}},
		{name: "blank", params: llmsharedtypes.JSONObject{"default_voice": "  "}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &defaultModelService{}

			_, err := service.Upsert(
				context.Background(),
				uuid.New(),
				nil,
				llmmodelmodel.UseCaseTextToSpeech,
				"doubao",
				"seed-tts-2.0",
				test.params,
			)

			if !errors.Is(err, ErrInvalidParams) {
				t.Fatalf("Upsert() error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestValidateTextToSpeechVoiceOption(t *testing.T) {
	parameters := llmmodelmodel.ConfigParameters{
		{
			Name:        "default_voice",
			TemplateKey: "default_voice",
			Type:        "string",
			Required:    true,
			Options:     []string{"voice-1", "voice-2"},
		},
	}

	if err := validateTextToSpeechVoiceOption(
		llmsharedtypes.JSONObject{"default_voice": "voice-1"},
		parameters,
	); err != nil {
		t.Fatalf("validateTextToSpeechVoiceOption() error = %v", err)
	}

	tests := []struct {
		name   string
		params llmsharedtypes.JSONObject
		rules  llmmodelmodel.ConfigParameters
	}{
		{
			name:   "voice is not listed",
			params: llmsharedtypes.JSONObject{"default_voice": "voice-3"},
			rules:  parameters,
		},
		{
			name:   "catalog has no voice options",
			params: llmsharedtypes.JSONObject{"default_voice": "voice-1"},
			rules:  llmmodelmodel.ConfigParameters{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTextToSpeechVoiceOption(test.params, test.rules)
			if !errors.Is(err, ErrInvalidParams) {
				t.Fatalf("validateTextToSpeechVoiceOption() error = %v, want ErrInvalidParams", err)
			}
		})
	}
}
