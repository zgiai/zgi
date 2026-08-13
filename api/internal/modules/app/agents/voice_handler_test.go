package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAgentVoiceTranscriptionUsesAuthenticatedAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	transcriber := &voiceTranscriberStub{response: &adapter.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "draft transcript",
	}}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/transcriptions", bytes.NewReader([]byte{1, 2, 3, 4}))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.TranscribeAgentVoice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if transcriber.organizationID != ids.organizationID.String() {
		t.Fatalf("organization = %q, want %q", transcriber.organizationID, ids.organizationID)
	}
	var body struct {
		Data adapter.TranscriptionResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Text != "draft transcript" {
		t.Fatalf("text = %q, want draft transcript", body.Data.Text)
	}
}

func TestAgentVoiceTranscriptionRejectsOversizedAudioBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	transcriber := &voiceTranscriberStub{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/transcriptions", bytes.NewReader(make([]byte, maxAgentVoicePCMBytes+2)))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.TranscribeAgentVoice(c)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if transcriber.calls != 0 {
		t.Fatalf("transcriber calls = %d, want 0", transcriber.calls)
	}
}

func TestAgentVoiceTranscriptionRejectsMisalignedPCMBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	transcriber := &voiceTranscriberStub{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/transcriptions", bytes.NewReader([]byte{1, 2, 3}))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.TranscribeAgentVoice(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if transcriber.calls != 0 {
		t.Fatalf("transcriber calls = %d, want 0", transcriber.calls)
	}
}

func TestAgentVoiceTranscriptionAddsDurationBoundedDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	transcriber := &voiceTranscriberStub{response: &adapter.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "deadline protected",
	}}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/transcriptions", bytes.NewReader(make([]byte, agentVoicePCMBytesPerSecond)))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())
	handler.TranscribeAgentVoice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !transcriber.hasDeadline {
		t.Fatal("transcription context has no deadline")
	}
	if got := transcriber.deadline.Sub(transcriber.calledAt); got < 20*time.Second || got > 22*time.Second {
		t.Fatalf("transcription deadline = %s, want approximately 21s", got)
	}
}

func TestAgentVoiceTranscriptionMapsBillingAndProviderQuotaSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "balance", err: gateway.ErrInsufficientBalance, wantStatus: http.StatusPaymentRequired, wantCode: "INSUFFICIENT_BALANCE"},
		{name: "workspace quota", err: gateway.ErrInsufficientQuota, wantStatus: http.StatusTooManyRequests, wantCode: "INSUFFICIENT_QUOTA"},
		{name: "provider quota", err: adapter.ErrQuotaExhausted, wantStatus: http.StatusServiceUnavailable, wantCode: "VOICE_UNAVAILABLE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			handleVoiceError(c, test.err)

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

func TestWebAppAgentVoiceTranscriptionUsesPublishedAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
	}
	transcriber := &voiceTranscriberStub{response: &adapter.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "published transcript",
	}}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/"+ids.webAppID.String()+"/runtime/audio/transcriptions", bytes.NewReader([]byte{1, 2, 3, 4}))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "web_app_id", Value: ids.webAppID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.TranscribeWebAppAgentVoice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if transcriber.organizationID != ids.organizationID.String() {
		t.Fatalf("organization = %q, want %q", transcriber.organizationID, ids.organizationID)
	}
}

func TestAgentVoiceTranscriptionDoesNotDependOnChatPromptResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	transcriber := &voiceTranscriberStub{response: &adapter.TranscriptionResponse{
		RequestID: uuid.NewString(),
		Text:      "voice still works",
	}}
	handler := NewAgentsHandler(&agentVoiceAccessService{ids: ids}, nil, nil, nil, nil, &noopChatRuntimeService{})
	handler.SetVoiceService(NewVoiceService(
		&voiceModelResolverStub{model: &llmdefaultservice.ResolvedModel{Model: "volc.seedasr.sauc.duration"}},
		transcriber,
	))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime/audio/transcriptions", bytes.NewReader([]byte{1, 2, 3, 4}))
	c.Request.Header.Set("Content-Type", "audio/pcm")
	c.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.TranscribeAgentVoice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if transcriber.calls != 1 {
		t.Fatalf("transcriber calls = %d, want 1", transcriber.calls)
	}
}

type agentVoiceAccessService struct {
	AgentsService
	ids webAppRuntimePermissionIDs
}

func (s *agentVoiceAccessService) GetAgentDraftRuntimeConfig(_ context.Context, agentID, accountID string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	if agentID != s.ids.agentID.String() || accountID != s.ids.accountID.String() {
		return nil, runtimeservice.ErrNotFound
	}
	return &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     s.ids.agentID.String(),
		WorkspaceID: s.ids.workspaceID.String(),
		Config: dto.AgentConfigResponse{
			AgentID:         s.ids.agentID.String(),
			SystemPrompt:    strings.Repeat("x", agentSystemPromptMaxLength+1),
			ModelParameters: map[string]interface{}{},
		},
	}, nil
}
