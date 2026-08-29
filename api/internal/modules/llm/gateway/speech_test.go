package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
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

const speechTestModel = "seed-tts-2.0"

var errSpeechClientDisconnected = errors.New("speech client disconnected")

type failingSpeechWriter struct{}

func (failingSpeechWriter) Write([]byte) (int, error) {
	return 0, errSpeechClientDisconnected
}

func TestClientWriteTrackerCapturesDestinationFailure(t *testing.T) {
	tracker := &clientWriteTracker{dst: failingSpeechWriter{}}
	_, err := tracker.Write([]byte("audio"))
	if !errors.Is(err, errSpeechClientDisconnected) || !errors.Is(tracker.writeErr, errSpeechClientDisconnected) {
		t.Fatalf("write error = %v, tracked = %v, want client disconnect", err, tracker.writeErr)
	}
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
		_, _ = w.Write([]byte("MP3-A"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("MP3-B"))
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	service := newSpeechGatewayTestService(t, organizationID, true)
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
