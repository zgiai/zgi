package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAgentSpeechStreamsMP3WithAuthenticatedAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{audio: []byte("mp3-stream")}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newAgentSpeechRequest(t, "/agents/"+ids.agentID.String()+"/runtime/audio/speech")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Fatalf("content type = %q, want audio/mpeg", got)
	}
	if recorder.Body.String() != "mp3-stream" {
		t.Fatalf("body = %q, want mp3-stream", recorder.Body.String())
	}
	if synthesizer.organizationID != ids.organizationID.String() {
		t.Fatalf("organization = %q, want %q", synthesizer.organizationID, ids.organizationID)
	}
	if !synthesizer.hasDeadline {
		t.Fatal("speech generation context has no deadline")
	}
	if remaining := time.Until(synthesizer.deadline); remaining <= 0 || remaining > agentSpeechGenerationTimeout {
		t.Fatalf("speech generation deadline remaining = %s, want within %s", remaining, agentSpeechGenerationTimeout)
	}
}

func TestWebAppAgentSpeechUsesPublishedAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{audio: []byte("published-mp3")}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newAgentSpeechRequest(t, "/webapps/"+ids.webAppID.String()+"/runtime/audio/speech")
	c.Params = gin.Params{{Key: "web_app_id", Value: ids.webAppID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.GenerateWebAppAgentSpeech(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if synthesizer.organizationID != ids.organizationID.String() {
		t.Fatalf("organization = %q, want %q", synthesizer.organizationID, ids.organizationID)
	}
}

func TestWebAppRuntimeConfigExposesVoiceCapabilitiesWithoutProviderIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
	}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		&voiceSynthesizerStub{},
	))
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		&voiceTranscriberStub{},
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/webapps/"+ids.webAppID.String()+"/config", nil)
	c.Params = gin.Params{{Key: "web_app_id", Value: ids.webAppID.String()}}

	handler.GetWebAppRuntimeConfig(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Data struct {
			Features struct {
				SpeechToText struct {
					Enabled bool `json:"enabled"`
				} `json:"speech_to_text"`
				TextToSpeech struct {
					Enabled bool `json:"enabled"`
				} `json:"text_to_speech"`
			} `json:"features"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.Features.TextToSpeech.Enabled {
		t.Fatal("text_to_speech.enabled = false, want true")
	}
	if !body.Data.Features.SpeechToText.Enabled {
		t.Fatal("speech_to_text.enabled = false, want true")
	}
	if strings.Contains(recorder.Body.String(), "verified-voice") {
		t.Fatal("runtime config leaked the provider voice identifier")
	}
	if strings.Contains(recorder.Body.String(), "volc.seedasr.sauc.duration") {
		t.Fatal("runtime config leaked the provider transcription model identifier")
	}
}

func TestAgentSpeechRejectsClientControlledVoiceBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/speech", bytes.NewBufferString(`{"input":"answer","voice":"attacker-voice"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if synthesizer.calls != 0 {
		t.Fatalf("synthesizer calls = %d, want 0", synthesizer.calls)
	}
}

func TestAgentSpeechRejectsOversizedInputBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(map[string]string{"input": strings.Repeat("声", 5001)})
	if err != nil {
		t.Fatal(err)
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/speech", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	var responseBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &responseBody); err != nil {
		t.Fatal(err)
	}
	if responseBody.Code != "SPEECH_INPUT_TOO_LARGE" {
		t.Fatalf("code = %q, want SPEECH_INPUT_TOO_LARGE", responseBody.Code)
	}
	if synthesizer.calls != 0 {
		t.Fatalf("synthesizer calls = %d, want 0", synthesizer.calls)
	}
}

func TestAgentSpeechRejectsInvalidUTF8BeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/speech", bytes.NewReader([]byte{'{', '"', 'i', 'n', 'p', 'u', 't', '"', ':', '"', 0xff, '"', '}'}))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if synthesizer.calls != 0 {
		t.Fatalf("synthesizer calls = %d, want 0", synthesizer.calls)
	}
}

func TestAgentSpeechDoesNotAppendJSONAfterAudioStarts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	synthesizer := &voiceSynthesizerStub{audio: []byte("partial-mp3"), err: errors.New("stream interrupted")}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		synthesizer,
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newAgentSpeechRequest(t, "/agents/"+ids.agentID.String()+"/runtime/audio/speech")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Body.String() != "partial-mp3" {
		t.Fatalf("body = %q, want only partial-mp3", recorder.Body.String())
	}
	if len(c.Errors) != 1 {
		t.Fatalf("gin errors = %d, want 1", len(c.Errors))
	}
}

func TestAgentSpeechRejectsEmptySuccessfulStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetSpeechService(NewSpeechService(
		&voiceModelResolverStub{model: speechResolvedModel()},
		&voiceSynthesizerStub{},
	))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newAgentSpeechRequest(t, "/agents/"+ids.agentID.String()+"/runtime/audio/speech")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.GenerateAgentSpeech(c)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "SPEECH_GENERATION_FAILED" {
		t.Fatalf("code = %q, want SPEECH_GENERATION_FAILED", body.Code)
	}
}

func TestAgentSpeechMapsBillingAndAvailabilityErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "cancelled", err: context.Canceled, wantStatus: statusClientClosedRequest, wantCode: "REQUEST_CANCELLED"},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantCode: "SPEECH_TIMEOUT"},
		{name: "balance", err: gateway.ErrInsufficientBalance, wantStatus: http.StatusPaymentRequired, wantCode: "INSUFFICIENT_BALANCE"},
		{name: "workspace quota", err: gateway.ErrInsufficientQuota, wantStatus: http.StatusTooManyRequests, wantCode: "INSUFFICIENT_QUOTA"},
		{name: "provider quota", err: adapter.ErrQuotaExhausted, wantStatus: http.StatusServiceUnavailable, wantCode: "SPEECH_UNAVAILABLE"},
		{name: "model unavailable", err: ErrSpeechUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "SPEECH_UNAVAILABLE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			handleSpeechError(c, test.err)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, test.wantCode)
			}
		})
	}
}

func newAgentSpeechRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{"input": "Readable answer"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func speechResolvedModel() *llmdefaultservice.ResolvedModel {
	return &llmdefaultservice.ResolvedModel{
		Model: "seed-tts-2.0",
		Params: map[string]interface{}{
			"default_voice": "verified-voice",
		},
	}
}
