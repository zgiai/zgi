package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestClientGenerateSpeechRejectsInvalidRequestBeforeResolvingCredentials(t *testing.T) {
	client := &llmClientImpl{}

	err := client.GenerateSpeech(t.Context(), "organization-id", nil, io.Discard)

	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateSpeech() error = %v, want ErrInvalidRequest", err)
	}
}

func TestClientGenerateSpeechUsesOrganizationSystemKeyAndStreamsAudio(t *testing.T) {
	organizationID := uuid.NewString()
	systemKey := &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID, IsInternal: true}
	keyRepo := &transcriptionAPIKeyRepositoryStub{key: systemKey}
	gatewayStub := &speechGatewayStub{audio: []byte("mp3-stream")}
	client := &llmClientImpl{gateway: gatewayStub, apiKeyRepo: keyRepo}
	var audio bytes.Buffer

	err := client.GenerateSpeech(t.Context(), organizationID, &adapter.SpeechRequest{
		Model:          "seed-tts-2.0",
		Input:          "answer",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &audio)

	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}
	if gatewayStub.apiKey != systemKey {
		t.Fatalf("gateway key = %#v, want system key %#v", gatewayStub.apiKey, systemKey)
	}
	if gatewayStub.request == nil || gatewayStub.request.Model != "seed-tts-2.0" {
		t.Fatalf("gateway request = %#v, want seed-tts-2.0", gatewayStub.request)
	}
	if gatewayStub.request.Input != "answer" || gatewayStub.request.Voice != "verified-voice" {
		t.Fatalf("gateway request = %#v, want input and verified voice", gatewayStub.request)
	}
	if audio.String() != "mp3-stream" {
		t.Fatalf("audio = %q, want mp3-stream", audio.String())
	}
}

type speechGatewayStub struct {
	gateway.LLMGatewayService
	apiKey  *apikeymodel.TenantAPIKey
	request *gateway.SpeechRequest
	audio   []byte
}

func (s *speechGatewayStub) GenerateSpeech(_ context.Context, apiKey *apikeymodel.TenantAPIKey, request *gateway.SpeechRequest, dst io.Writer) error {
	s.apiKey = apiKey
	s.request = request
	_, err := dst.Write(s.audio)
	return err
}
