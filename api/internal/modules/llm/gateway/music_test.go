package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

const musicGatewayTestModel = "music-3.0"

var errMusicDestinationWrite = errors.New("music destination write failed")

type failingMusicWriter struct{}

func (failingMusicWriter) Write([]byte) (int, error) {
	return 0, errMusicDestinationWrite
}

func TestGenerateMusicUsesStableRequestIDAndCompleteOfficialStream(t *testing.T) {
	organizationID := uuid.New()
	requestID := uuid.NewString()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got, want := r.URL.Path, "/v1/internal/audio/music/generations"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get(headerZGIRequestID); got != requestID {
			t.Fatalf("request id = %q, want %q", got, requestID)
		}
		if got := r.Header.Get(headerZGIBillingOrganizationID); got != organizationID.String() {
			t.Fatalf("billing organization = %q", got)
		}
		var request adapter.MusicRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != musicGatewayTestModel || request.Mode != adapter.MusicModeInstrumental || request.Prompt != "warm piano" {
			t.Fatalf("music request = %#v", request)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Trailer", "X-ZGI-Stream-Status")
		_, _ = w.Write([]byte("MP3"))
		w.Header().Set("X-ZGI-Stream-Status", "complete")
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)

	service := newMusicGatewayTestService(t, organizationID, true)
	var audio bytes.Buffer
	err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &MusicRequest{
		RequestID:      requestID,
		Model:          musicGatewayTestModel,
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateMusic() error = %v", err)
	}
	if got, want := audio.String(), "MP3"; got != want {
		t.Fatalf("audio = %q, want %q", got, want)
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestGenerateMusicRejectsModelWithoutCapabilityBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)

	service := newMusicGatewayTestService(t, organizationID, false)
	err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &MusicRequest{
		RequestID:      uuid.NewString(),
		Model:          musicGatewayTestModel,
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if !adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("GenerateMusic() error = %v, want capability unsupported", err)
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
}

func TestGenerateMusicRejectsNonCanonicalFormatBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)

	service := newMusicGatewayTestService(t, organizationID, true)
	err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &MusicRequest{
		RequestID:      uuid.NewString(),
		Model:          musicGatewayTestModel,
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: " mp3 ",
	}, &bytes.Buffer{})
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateMusic() error = %v, want ErrInvalidRequest", err)
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
}

func TestGenerateMusicRejectsNonCanonicalRequestIDBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)
	service := newMusicGatewayTestService(t, organizationID, true)

	for _, requestID := range []string{" " + uuid.NewString() + " ", uuid.Nil.String()} {
		err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
			ID:             uuid.NewString(),
			OrganizationID: organizationID.String(),
		}, &MusicRequest{
			RequestID:      requestID,
			Model:          musicGatewayTestModel,
			Mode:           adapter.MusicModeInstrumental,
			Prompt:         "warm piano",
			ResponseFormat: "mp3",
		}, &bytes.Buffer{})
		if !errors.Is(err, adapter.ErrInvalidRequest) {
			t.Fatalf("GenerateMusic(%q) error = %v, want ErrInvalidRequest", requestID, err)
		}
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
}

func TestGenerateMusicReportsOfficialProviderFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)
	service := newMusicGatewayTestService(t, organizationID, true)

	err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &MusicRequest{
		RequestID:      uuid.NewString(),
		Model:          musicGatewayTestModel,
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("GenerateMusic() error = nil, want provider rejection")
	}
	if len(recorder.events) != 1 || recorder.events[0].Attributes["use_system_provider"] != true {
		t.Fatalf("events = %#v, want one system-channel failure", recorder.events)
	}
}

func TestGenerateMusicDoesNotReportDestinationWriteFailure(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = io.WriteString(w, "MP3")
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)
	recorder := withGatewayObservabilityRecorder(t)
	service := newMusicGatewayTestService(t, organizationID, true)

	err := service.GenerateMusic(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &MusicRequest{
		RequestID:      uuid.NewString(),
		Model:          musicGatewayTestModel,
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, failingMusicWriter{})
	if !errors.Is(err, errMusicDestinationWrite) {
		t.Fatalf("GenerateMusic() error = %v, want destination write error", err)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %#v, want no provider failure", recorder.events)
	}
}

func TestCompensateMusicDeliveryDoesNotDependOnCurrentModelRoute(t *testing.T) {
	organizationID := uuid.New()
	requestID := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/internal/audio/music/delivery-compensations"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get(headerZGIBillingOrganizationID); got != organizationID.String() {
			t.Fatalf("billing organization = %q", got)
		}
		if got := r.Header.Get(headerZGIRequestID); got != requestID {
			t.Fatalf("request id = %q", got)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"billing_status":"compensated"}}`))
	}))
	defer server.Close()
	setMusicGatewayTestConfig(t, server.URL)

	service := &llmGatewayServiceImpl{
		db:             openGatewayCatalogDB(t),
		adapterFactory: adapter.GlobalFactory,
	}
	err := service.CompensateMusicDelivery(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, requestID)
	if err != nil {
		t.Fatalf("CompensateMusicDelivery() error = %v", err)
	}
}

func newMusicGatewayTestService(t *testing.T, organizationID uuid.UUID, enabled bool) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "minimax", musicGatewayTestModel)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", musicGatewayTestModel).
		Updates(map[string]any{
			"music_generation": enabled,
			"use_cases":        llmmodel.StringArray{string(llmmodel.UseCaseMusicGen)},
		}).Error; err != nil {
		t.Fatal(err)
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		IsOfficial:      true,
		IsEnabled:       true,
		Models:          []string{musicGatewayTestModel},
		OfficialProviderModels: []channelmodel.ProviderModel{{
			Provider: "minimax",
			Model:    musicGatewayTestModel,
		}},
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

func setMusicGatewayTestConfig(t *testing.T, consoleURL string) {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{
			APIURL:         consoleURL,
			InternalAPIKey: "test-internal-key",
		},
	}
	t.Cleanup(func() { config.GlobalConfig = previous })
}
