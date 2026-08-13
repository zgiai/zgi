package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestClientTranscribeRejectsNilRequestBeforeResolvingCredentials(t *testing.T) {
	client := &llmClientImpl{}
	_, err := client.Transcribe(t.Context(), "organization-id", nil)
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("Transcribe() error = %v, want ErrInvalidRequest", err)
	}
}

func TestClientTranscribeUsesOrganizationSystemKeyAndForwardsAudio(t *testing.T) {
	organizationID := uuid.NewString()
	systemKey := &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID, IsInternal: true}
	keyRepo := &transcriptionAPIKeyRepositoryStub{key: systemKey}
	gatewayStub := &transcriptionGatewayStub{response: &gateway.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "forwarded transcript",
	}}
	client := &llmClientImpl{gateway: gatewayStub, apiKeyRepo: keyRepo}
	audio := []byte{1, 2, 3, 4}

	result, err := client.Transcribe(t.Context(), organizationID, &adapter.TranscriptionRequest{
		Model: "volc.seedasr.sauc.duration",
		Audio: bytes.NewReader(audio),
	})

	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if result == nil || result.Text != "forwarded transcript" {
		t.Fatalf("Transcribe() = %#v, want forwarded transcript", result)
	}
	if keyRepo.organizationID != organizationID {
		t.Fatalf("key organization = %q, want %q", keyRepo.organizationID, organizationID)
	}
	if gatewayStub.apiKey != systemKey {
		t.Fatalf("gateway key = %#v, want system key %#v", gatewayStub.apiKey, systemKey)
	}
	if gatewayStub.request == nil || gatewayStub.request.Model != "volc.seedasr.sauc.duration" {
		t.Fatalf("gateway request = %#v, want transcription model", gatewayStub.request)
	}
	gotAudio, readErr := io.ReadAll(gatewayStub.request.Audio)
	if readErr != nil || !bytes.Equal(gotAudio, audio) {
		t.Fatalf("gateway audio = %v, %v; want %v", gotAudio, readErr, audio)
	}
}

type transcriptionAPIKeyRepositoryStub struct {
	apikeyrepo.APIKeyRepository
	key            *apikeymodel.TenantAPIKey
	organizationID string
}

func (s *transcriptionAPIKeyRepositoryStub) List(_ context.Context, organizationID string, _ map[string]interface{}, _, _ int) ([]*apikeymodel.TenantAPIKey, int64, error) {
	s.organizationID = organizationID
	return []*apikeymodel.TenantAPIKey{s.key}, 1, nil
}

type transcriptionGatewayStub struct {
	gateway.LLMGatewayService
	apiKey   *apikeymodel.TenantAPIKey
	request  *gateway.TranscriptionRequest
	response *gateway.TranscriptionResponse
}

func (s *transcriptionGatewayStub) Transcribe(_ context.Context, apiKey *apikeymodel.TenantAPIKey, request *gateway.TranscriptionRequest) (*gateway.TranscriptionResponse, error) {
	s.apiKey = apiKey
	s.request = request
	return s.response, nil
}
