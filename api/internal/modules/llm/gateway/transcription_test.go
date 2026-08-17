package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/config"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const transcriptionTestModel = "volc.seedasr.sauc.duration"

var errTranscriptionUploadInterrupted = errors.New("transcription upload interrupted")

type failingTranscriptionReader struct{}

func (failingTranscriptionReader) Read([]byte) (int, error) {
	return 0, errTranscriptionUploadInterrupted
}

func TestTranscribeUsesOfficialHTTPRouteAndReturnsFinalText(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got := r.URL.Path; got != "/v1/internal/audio/transcriptions" {
			t.Errorf("path = %q, want /v1/internal/audio/transcriptions", got)
		}
		if got := r.Header.Get(headerZGIBillingOrganizationID); got != organizationID.String() {
			t.Errorf("billing organization = %q, want %q", got, organizationID)
		}
		if got := r.Header.Get(headerZGIModelName); got != transcriptionTestModel {
			t.Errorf("model = %q, want %q", got, transcriptionTestModel)
		}
		if r.Header.Get("X-Internal-Timestamp") == "" || r.Header.Get("X-Internal-Signature") == "" {
			t.Error("internal HMAC headers are missing")
		}
		requestID := r.Header.Get(headerZGIRequestID)
		w.Header().Set("X-ZGI-Settlement-ID", "transcription-settlement")
		w.Header().Set("X-ZGI-Official-Points", "333")
		w.Header().Set("X-ZGI-Settlement-Status", "settled")
		_, _ = fmt.Fprintf(w, `{"code":0,"message":"success","data":{"request_id":%q,"text":"可编辑文本"}}`, requestID)
	}))
	defer server.Close()
	setTranscriptionGatewayTestConfig(t, server.URL)

	service := newTranscriptionGatewayTestService(t, organizationID, true)
	enablePlatformMediaUsageRecorderForTest(t, service)
	contentRecorder := &invocationContentRecorder{
		config: config.LLMInvocationContentConfig{MaxBytes: 64 * 1024},
		queue:  make(chan invocationContentRecord, 1),
	}
	service.invocationContent = contentRecorder
	response, err := service.Transcribe(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &TranscriptionRequest{
		Model: transcriptionTestModel,
		Audio: bytes.NewReader([]byte("pcm-audio")),
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if response == nil || response.RequestID == "" || response.Text != "可编辑文本" {
		t.Fatalf("response = %#v, want request ID and final text", response)
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
	content := requireInvocationContentRecord(t, contentRecorder, "transcription")
	if content.OrganizationID != organizationID.String() || !strings.Contains(content.InputText, "binary omitted") || content.OutputText != "可编辑文本" {
		t.Fatalf("transcription content = %#v", content)
	}
	if strings.Contains(content.InputJSON, "pcm-audio") {
		t.Fatalf("transcription binary leaked into content snapshot: %s", content.InputJSON)
	}
	bill := requirePlatformMediaUsageBill(t, service, response.RequestID)
	if bill.Status != usageBillStatusSuccess || bill.TotalPoints != 333 || bill.RemoteDeductionID == nil || *bill.RemoteDeductionID != "transcription-settlement" {
		t.Fatalf("transcription usage bill = %#v", bill)
	}
}

func TestTranscribeRejectsModelWithoutTranscriptionBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setTranscriptionGatewayTestConfig(t, server.URL)

	service := newTranscriptionGatewayTestService(t, organizationID, false)
	_, err := service.Transcribe(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &TranscriptionRequest{
		Model: transcriptionTestModel,
		Audio: bytes.NewReader([]byte("pcm-audio")),
	})
	if !adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("Transcribe() error = %v, want capability unsupported", err)
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want fail before dispatch", got)
	}
}

func TestTranscribeReportsOfficialProviderFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	setTranscriptionGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)

	service := newTranscriptionGatewayTestService(t, organizationID, true)
	_, err := service.Transcribe(t.Context(), &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID.String()}, &TranscriptionRequest{
		Model: transcriptionTestModel, Audio: bytes.NewReader([]byte("pcm")),
	})
	if err == nil {
		t.Fatal("Transcribe() error = nil, want provider rejection")
	}
	if len(recorder.events) != 1 || recorder.events[0].Name != "llm.provider.request_failed" || recorder.events[0].Attributes["use_system_provider"] != true {
		t.Fatalf("events = %#v, want one system-channel failure", recorder.events)
	}
}

func TestTranscribeDoesNotReportClientUploadFailureAsProviderFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"request_id":"unused","text":"unused"}}`))
	}))
	defer server.Close()
	setTranscriptionGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)

	service := newTranscriptionGatewayTestService(t, organizationID, true)
	enablePlatformMediaUsageRecorderForTest(t, service)
	_, err := service.Transcribe(t.Context(), &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID.String()}, &TranscriptionRequest{
		Model: transcriptionTestModel, Audio: failingTranscriptionReader{},
	})
	if !errors.Is(err, errTranscriptionUploadInterrupted) || !IsClientIOError(err) {
		t.Fatalf("Transcribe() error = %v, want typed client upload failure", err)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %#v, want no provider failure for client upload error", recorder.events)
	}
	waitForUsageBills(t, service.db, 1)
	var bill UsageBill
	if err := service.db.First(&bill).Error; err != nil {
		t.Fatalf("query client input usage bill: %v", err)
	}
	if bill.Status != usageBillStatusFailed || bill.ErrorCode == nil || *bill.ErrorCode != platformMediaClientInputFailureCode {
		t.Fatalf("client input usage bill = %#v", bill)
	}
}

func TestTranscribeUsesPrivateDoubaoSpeechRouteAndSettlesActualAudioDuration(t *testing.T) {
	organizationID := uuid.New()
	audio := bytes.Repeat([]byte{0x2a}, 3200)
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	pricing := &fakePricingEngine{meteredQuoteFunc: func(usage MeteredUsage) (PricingQuote, error) {
		quote := newOutputOnlyUSDQuote(
			decimal.RequireFromString("0.0006"),
			PricingSourceUpstreamModelPrice,
			"transcription/input_audio_duration/millisecond",
			UsageSourceRequestParameters,
			nil,
		)
		return withMeteredPricingBasis(quote, usage, decimal.RequireFromString("0.00000001")), nil
	}}
	providerAdapter := &privateTranscriptionGatewayAdapter{}
	factory := adapter.NewDefaultAdapterFactory()
	factory.Register("doubao-speech", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		if config.APIKey != "test-api-key" {
			t.Fatalf("adapter API key = %q, want decrypted private key", config.APIKey)
		}
		return providerAdapter, nil
	})
	service := newPrivateTranscriptionGatewayTestService(t, organizationID)
	service.adapterFactory = factory
	service.billing = remote
	service.localBilling = local
	service.pricingEngine = pricing

	response, err := service.Transcribe(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &TranscriptionRequest{
		Model: transcriptionTestModel,
		Audio: bytes.NewReader(audio),
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if response == nil || response.Text != "私有渠道文本" || response.RequestID == "" {
		t.Fatalf("response = %#v", response)
	}
	if providerAdapter.calls != 1 || providerAdapter.audioBytes != int64(len(audio)) {
		t.Fatalf("provider calls/audio bytes = %d/%d", providerAdapter.calls, providerAdapter.audioBytes)
	}
	if len(pricing.meteredUsages) != 1 || pricing.meteredUsages[0].Quantity != 60000 {
		t.Fatalf("metered usages = %#v, want one locked reserve quote for 60000", pricing.meteredUsages)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 || remote.preDeductCalls != 0 || remote.settleCalls != 0 {
		t.Fatalf("billing calls local=%d/%d remote=%d/%d", local.preDeductCalls, local.settleCalls, remote.preDeductCalls, remote.settleCalls)
	}
	if local.lastSettle == nil || local.lastSettle.EstimatedCredits != 600 || local.lastSettle.ActualCredits != 1 {
		t.Fatalf("settlement = %#v, want estimated/actual 600/1", local.lastSettle)
	}
}

func TestTranscriptionMeteredReaderRejectsAudioOver60Seconds(t *testing.T) {
	reader := newTranscriptionMeteredReader(
		bytes.NewReader(make([]byte, transcriptionMaxAudioBytes()+1)),
		transcriptionMaxAudioBytes(),
	)
	_, err := io.ReadAll(reader)
	if !errors.Is(err, ErrTranscriptionAudioTooLong) {
		t.Fatalf("ReadAll() error = %v, want ErrTranscriptionAudioTooLong", err)
	}
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("ReadAll() error = %v, want invalid request classification", err)
	}
}

type privateTranscriptionGatewayAdapter struct {
	chatTraceSuccessAdapter
	calls      int
	audioBytes int64
}

func (a *privateTranscriptionGatewayAdapter) Transcribe(_ context.Context, request *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error) {
	a.calls++
	audio, err := io.ReadAll(request.Audio)
	if err != nil {
		return nil, err
	}
	a.audioBytes = int64(len(audio))
	return &adapter.TranscriptionResponse{RequestID: request.RequestID, Text: "私有渠道文本"}, nil
}

func newPrivateTranscriptionGatewayTestService(t *testing.T, organizationID uuid.UUID) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "doubao", transcriptionTestModel)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", transcriptionTestModel).
		Updates(map[string]any{
			"transcription": true,
			"use_cases":     llmmodel.StringArray{string(llmmodel.UseCaseSpeechToText)},
		}).Error; err != nil {
		t.Fatalf("update transcription model: %v", err)
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "doubao-speech",
		IsEnabled:       true,
		Models:          []string{transcriptionTestModel},
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "doubao-speech",
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

func newTranscriptionGatewayTestService(t *testing.T, organizationID uuid.UUID, transcription bool) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "doubao", transcriptionTestModel)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", transcriptionTestModel).
		Updates(map[string]any{
			"transcription": transcription,
			"use_cases":     llmmodel.StringArray{string(llmmodel.UseCaseSpeechToText)},
		}).Error; err != nil {
		t.Fatalf("update transcription model: %v", err)
	}

	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		IsOfficial:      true,
		IsEnabled:       true,
		Models:          []string{transcriptionTestModel},
		OfficialProviderModels: []channelmodel.ProviderModel{{
			Provider: "doubao",
			Model:    transcriptionTestModel,
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

func setTranscriptionGatewayTestConfig(t *testing.T, consoleURL string) {
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
