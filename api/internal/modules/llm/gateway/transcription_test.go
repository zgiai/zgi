package gateway

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const transcriptionTestModel = "volc.seedasr.sauc.duration"

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
		_, _ = fmt.Fprintf(w, `{"code":0,"message":"success","data":{"request_id":%q,"text":"可编辑文本"}}`, requestID)
	}))
	defer server.Close()
	setTranscriptionGatewayTestConfig(t, server.URL)

	service := newTranscriptionGatewayTestService(t, organizationID, true)
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
