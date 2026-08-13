package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const speechTestModel = "seed-tts-2.0"

var errSpeechClientDisconnected = errors.New("speech client disconnected")

type failingSpeechWriter struct{}

func (failingSpeechWriter) Write([]byte) (int, error) {
	return 0, errSpeechClientDisconnected
}

func TestGenerateSpeechUsesOfficialHTTPRouteAndStreamsMP3(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got, want := r.URL.Path, "/v1/internal/audio/speech"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get(headerZGIBillingOrganizationID), organizationID.String(); got != want {
			t.Errorf("billing organization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get(headerZGIModelName), speechTestModel; got != want {
			t.Errorf("model = %q, want %q", got, want)
		}
		if r.Header.Get("X-Internal-Timestamp") == "" || r.Header.Get("X-Internal-Signature") == "" {
			t.Error("internal HMAC headers are missing")
		}
		var request adapter.SpeechRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode speech request: %v", err)
		}
		if request.Model != speechTestModel || request.Input != "你好。" || request.Voice != "verified-voice" || request.ResponseFormat != "mp3" {
			t.Errorf("speech request = %#v", request)
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Trailer", "X-ZGI-Settlement-ID, X-ZGI-Official-Points, X-ZGI-Settlement-Status")
		_, _ = w.Write([]byte("MP3-A"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("MP3-B"))
		w.Header().Set("X-ZGI-Settlement-ID", "speech-settlement")
		w.Header().Set("X-ZGI-Official-Points", "222")
		w.Header().Set("X-ZGI-Settlement-Status", "settled")
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	service := newSpeechGatewayTestService(t, organizationID, true)
	enablePlatformMediaUsageRecorderForTest(t, service)
	contentRecorder := &invocationContentRecorder{
		config: config.LLMInvocationContentConfig{MaxBytes: 64 * 1024},
		queue:  make(chan invocationContentRecord, 1),
	}
	service.invocationContent = contentRecorder
	var audio bytes.Buffer
	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &SpeechRequest{
		Model:          speechTestModel,
		Input:          "你好。",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}
	if got, want := audio.String(), "MP3-AMP3-B"; got != want {
		t.Fatalf("GenerateSpeech() audio = %q, want %q", got, want)
	}
	if got, want := requestCount.Load(), int32(1); got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	content := requireInvocationContentRecord(t, contentRecorder, "speech")
	if content.OrganizationID != organizationID.String() || content.InputText != "你好。" || content.OutputText != "delivered" {
		t.Fatalf("speech content = %#v", content)
	}
	if strings.Contains(content.OutputJSON, "MP3-A") {
		t.Fatalf("speech binary leaked into content snapshot: %s", content.OutputJSON)
	}
	bill := requirePlatformMediaUsageBill(t, service, content.RequestID)
	if bill.Status != usageBillStatusSuccess || bill.TotalPoints != 222 || bill.RemoteDeductionID == nil || *bill.RemoteDeductionID != "speech-settlement" {
		t.Fatalf("speech usage bill = %#v", bill)
	}
}

func TestGenerateSpeechRejectsModelWithoutSpeechCapabilityBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	service := newSpeechGatewayTestService(t, organizationID, false)
	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &SpeechRequest{
		Model:          speechTestModel,
		Input:          "text",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if !adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("GenerateSpeech() error = %v, want capability unsupported", err)
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want fail before dispatch", got)
	}
}

func TestGenerateSpeechReportsOfficialProviderFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)

	service := newSpeechGatewayTestService(t, organizationID, true)
	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID.String()}, &SpeechRequest{
		Model: speechTestModel, Input: "text", Voice: "voice", ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("GenerateSpeech() error = nil, want provider rejection")
	}
	if len(recorder.events) != 1 || recorder.events[0].Name != "llm.provider.stream_failed" || recorder.events[0].Attributes["use_system_provider"] != true {
		t.Fatalf("events = %#v, want one system-channel failure", recorder.events)
	}
}

func TestGenerateSpeechDoesNotReportClientWriteFailureAsProviderFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = io.WriteString(w, "MP3")
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)

	service := newSpeechGatewayTestService(t, organizationID, true)
	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID.String()}, &SpeechRequest{
		Model: speechTestModel, Input: "text", Voice: "voice", ResponseFormat: "mp3",
	}, failingSpeechWriter{})
	if !errors.Is(err, errSpeechClientDisconnected) {
		t.Fatalf("GenerateSpeech() error = %v, want client disconnect", err)
	}
	if !IsClientIOError(err) {
		t.Fatalf("GenerateSpeech() error = %v, want typed client I/O failure", err)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %#v, want no provider failure for client disconnect", recorder.events)
	}
}

func TestGenerateSpeechRejectsInvalidUTF8(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	err := service.GenerateSpeech(t.Context(), nil, &SpeechRequest{
		Model:          speechTestModel,
		Input:          string([]byte{0xff}),
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateSpeech() error = %v, want invalid request", err)
	}
}

func TestGenerateSpeechUsesPrivateDoubaoRouteAndSettlesMeteredBilling(t *testing.T) {
	organizationID := uuid.New()
	setSpeechGatewayTestConfig(t, "")

	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	pricing := &fakePricingEngine{meteredQuoteFunc: func(usage MeteredUsage) (PricingQuote, error) {
		return PricingQuote{TotalCredits: usage.Quantity, OutputCredits: usage.Quantity}, nil
	}}
	providerAdapter := &privateSpeechGatewayAdapter{}
	factory := adapter.NewDefaultAdapterFactory()
	factory.Register("doubao", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		if config.APIKey != "test-api-key" {
			t.Fatalf("adapter API key = %q, want decrypted private key", config.APIKey)
		}
		return providerAdapter, nil
	})
	service := newPrivateSpeechGatewayTestService(t, organizationID)
	service.adapterFactory = factory
	service.billing = remote
	service.localBilling = local
	service.pricingEngine = pricing

	var audio bytes.Buffer
	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &SpeechRequest{
		Model:          speechTestModel,
		Input:          "你好。",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}
	if got, want := audio.String(), "PRIVATE-MP3"; got != want {
		t.Fatalf("GenerateSpeech() audio = %q, want %q", got, want)
	}
	if providerAdapter.calls != 1 || providerAdapter.request == nil || providerAdapter.request.Model != speechTestModel {
		t.Fatalf("provider calls/request = %d/%#v", providerAdapter.calls, providerAdapter.request)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 || remote.preDeductCalls != 0 || remote.settleCalls != 0 {
		t.Fatalf("billing calls local=%d/%d remote=%d/%d", local.preDeductCalls, local.settleCalls, remote.preDeductCalls, remote.settleCalls)
	}
	if local.lastSettle == nil || local.lastSettle.ActualCredits != 3 || pricing.lastMeteredUsage.Quantity != 3 {
		t.Fatalf("settlement = %#v, metered usage = %#v", local.lastSettle, pricing.lastMeteredUsage)
	}
}

func TestGenerateSpeechPrivateProviderFailureRollsBackReservation(t *testing.T) {
	organizationID := uuid.New()
	providerErr := errors.New("provider failed")
	providerAdapter := &privateSpeechGatewayAdapter{err: providerErr}
	factory := adapter.NewDefaultAdapterFactory()
	factory.Register("doubao", func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return providerAdapter, nil
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	service := newPrivateSpeechGatewayTestService(t, organizationID)
	service.adapterFactory = factory
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{TotalCredits: 3}}

	err := service.GenerateSpeech(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &SpeechRequest{
		Model:          speechTestModel,
		Input:          "你好。",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if !errors.Is(err, providerErr) {
		t.Fatalf("GenerateSpeech() error = %v, want provider error", err)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 || local.lastSettle == nil {
		t.Fatalf("billing calls/settlement = %d/%d/%#v", local.preDeductCalls, local.settleCalls, local.lastSettle)
	}
	if local.lastSettle.Status != "error" || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("rollback status/actual = %q/%d, want error/0", local.lastSettle.Status, local.lastSettle.ActualCredits)
	}
}

type privateSpeechGatewayAdapter struct {
	chatTraceSuccessAdapter
	calls   int
	request *adapter.SpeechRequest
	err     error
}

func (a *privateSpeechGatewayAdapter) GenerateSpeech(_ context.Context, request *adapter.SpeechRequest, dst io.Writer) (*adapter.SettlementResult, error) {
	a.calls++
	a.request = request
	if a.err != nil {
		return nil, a.err
	}
	_, err := dst.Write([]byte("PRIVATE-MP3"))
	return nil, err
}

func newPrivateSpeechGatewayTestService(t *testing.T, organizationID uuid.UUID) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "doubao", speechTestModel)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", speechTestModel).
		Updates(map[string]any{
			"speech_generation": true,
			"use_cases":         llmmodel.StringArray{string(llmmodel.UseCaseTextToSpeech)},
		}).Error; err != nil {
		t.Fatalf("update speech model: %v", err)
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "doubao",
		IsEnabled:       true,
		Models:          []string{speechTestModel},
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "doubao",
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}
	return &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: adapter.GlobalFactory,
		channelRouter: &ChannelRouter{
			db:                      db,
			organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
			cryptoService:           stubCryptoService{},
			strategyFactory:         NewStrategyFactory(),
			privateModels:           &fakePrivateModelLookup{},
		},
	}
}

func newSpeechGatewayTestService(t *testing.T, organizationID uuid.UUID, speechGeneration bool) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "doubao", speechTestModel)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", speechTestModel).
		Updates(map[string]any{
			"speech_generation": speechGeneration,
			"use_cases":         llmmodel.StringArray{string(llmmodel.UseCaseTextToSpeech)},
		}).Error; err != nil {
		t.Fatalf("update speech model: %v", err)
	}

	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		IsOfficial:      true,
		IsEnabled:       true,
		Models:          []string{speechTestModel},
		OfficialProviderModels: []channelmodel.ProviderModel{{
			Provider: "doubao",
			Model:    speechTestModel,
		}},
	}
	router := &ChannelRouter{
		db:                      db,
		organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
		cryptoService:           stubCryptoService{},
		strategyFactory:         NewStrategyFactory(),
		privateModels:           &fakePrivateModelLookup{},
	}
	return &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: adapter.GlobalFactory,
		channelRouter:  router,
	}
}

func setSpeechGatewayTestConfig(t *testing.T, consoleURL string) {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{
			APIURL:         consoleURL,
			InternalAPIKey: "test-internal-key",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})
}
