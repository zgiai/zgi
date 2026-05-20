package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
)

func TestPrepareWorkflowEventsSSEHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/workflow-runs/run-1/events", nil)
	c.Params = gin.Params{{Key: "workflow_run_id", Value: "run-1"}}

	prepareWorkflowEventsSSE(c)

	response := recorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	assertHeader(t, response, "Content-Type", "text/event-stream")
	assertHeader(t, response, "Cache-Control", "no-cache, no-transform")
	assertHeader(t, response, "Connection", "keep-alive")
	assertHeader(t, response, "X-Accel-Buffering", "no")
}

func TestPrepareWorkflowStreamSSEHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/console/api/agents/app-1/workflows/draft/run", nil)

	prepareWorkflowStreamSSE(c)

	response := recorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	assertHeader(t, response, "Content-Type", "text/event-stream")
	assertHeader(t, response, "Cache-Control", "no-cache, no-transform")
	assertHeader(t, response, "Connection", "keep-alive")
	assertHeader(t, response, "X-Accel-Buffering", "no")
	if got := recorder.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty opener for test recorder", got)
	}
}

func TestSendWorkflowSSEPing(t *testing.T) {
	recorder := httptest.NewRecorder()

	sendWorkflowSSEPing(context.Background(), recorder)

	if got := recorder.Body.String(); got != "event: ping\n\n" {
		t.Fatalf("ping body = %q, want SSE ping frame", got)
	}
}

func TestSendWorkflowSSEKeepAlive(t *testing.T) {
	recorder := httptest.NewRecorder()

	sendWorkflowSSEKeepAlive(context.Background(), recorder)

	if got := recorder.Body.String(); got != ": ping\n\n" {
		t.Fatalf("keepalive body = %q, want SSE comment", got)
	}
}

func TestSendWorkflowRunEventsContinuesOnPauseWhenRequested(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	service := workflowpause.NewService(db)
	run := &WorkflowRunLog{
		ID:       "run-" + uuid.NewString(),
		TenantID: uuid.NewString(),
		AgentID:  uuid.NewString(),
	}

	for _, eventType := range []string{
		"node_started",
		workflowpause.EventWorkflowPaused,
		"node_finished",
	} {
		if err := service.AppendEvent(ctx, workflowpause.AppendEventParams{
			TenantID:      run.TenantID,
			AppID:         run.AgentID,
			WorkflowRunID: run.ID,
			EventType:     eventType,
			EventData:     map[string]interface{}{"event_type": eventType},
		}); err != nil {
			t.Fatalf("append event %s: %v", eventType, err)
		}
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)
	handler := &WorkflowHandler{}

	sequence, terminal := handler.sendWorkflowRunEvents(c, service, run, 0, true, false, 0)
	if terminal {
		t.Fatal("sendWorkflowRunEvents should not terminate on workflow_paused when continueOnPause is true")
	}
	if sequence != 3 {
		t.Fatalf("last sequence = %d, want 3", sequence)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"event":"workflow_paused"`) || !strings.Contains(body, `"event":"node_finished"`) {
		t.Fatalf("SSE body should include pause and later events, got %q", body)
	}
}

func TestSendWorkflowRunEventsSkipsHistoricalMessagesOnly(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	service := workflowpause.NewService(db)
	run := &WorkflowRunLog{
		ID:       "run-" + uuid.NewString(),
		TenantID: uuid.NewString(),
		AgentID:  uuid.NewString(),
	}

	appendWorkflowEventForReplayTest(t, ctx, service, run, workflowEventMessage, map[string]interface{}{"answer": "old-1"})
	appendWorkflowEventForReplayTest(t, ctx, service, run, workflowEventMessage, map[string]interface{}{"answer": "old-2"})
	appendWorkflowEventForReplayTest(t, ctx, service, run, "node_started", map[string]interface{}{"node_id": "answer-1"})
	appendWorkflowEventForReplayTest(t, ctx, service, run, workflowEventMessage, map[string]interface{}{"answer": "new"})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)
	handler := &WorkflowHandler{}

	sequence, terminal := handler.sendWorkflowRunEvents(c, service, run, 0, true, false, 2)
	if terminal {
		t.Fatal("sendWorkflowRunEvents should not terminate without terminal events")
	}
	if sequence != 4 {
		t.Fatalf("last sequence = %d, want 4", sequence)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "old-1") || strings.Contains(body, "old-2") {
		t.Fatalf("historical message events should be skipped, got %q", body)
	}
	if !strings.Contains(body, `"event":"node_started"`) {
		t.Fatalf("non-message replay event should be sent, got %q", body)
	}
	if !strings.Contains(body, `"answer":"new"`) {
		t.Fatalf("message after replay cutoff should be sent, got %q", body)
	}
}

func TestSendWorkflowRunEventsIncludesHistoricalMessagesWhenRequested(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	service := workflowpause.NewService(db)
	run := &WorkflowRunLog{
		ID:       "run-" + uuid.NewString(),
		TenantID: uuid.NewString(),
		AgentID:  uuid.NewString(),
	}

	appendWorkflowEventForReplayTest(t, ctx, service, run, workflowEventMessage, map[string]interface{}{"answer": "old"})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)
	handler := &WorkflowHandler{}

	sequence, terminal := handler.sendWorkflowRunEvents(c, service, run, 0, true, true, 1)
	if terminal {
		t.Fatal("sendWorkflowRunEvents should not terminate without terminal events")
	}
	if sequence != 1 {
		t.Fatalf("last sequence = %d, want 1", sequence)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"answer":"old"`) {
		t.Fatalf("historical message should be sent with includeMessageEvents, got %q", body)
	}
}

func TestShouldSkipWorkflowReplayMessage(t *testing.T) {
	messageEvent := workflowpause.RunEventPayload{Sequence: 2, Event: workflowEventMessage}
	nodeEvent := workflowpause.RunEventPayload{Sequence: 2, Event: "node_finished"}
	futureMessageEvent := workflowpause.RunEventPayload{Sequence: 1, Event: workflowEventMessage}

	if !shouldSkipWorkflowReplayMessage(messageEvent, false, 2) {
		t.Fatal("historical message event should be skipped")
	}
	if shouldSkipWorkflowReplayMessage(messageEvent, true, 2) {
		t.Fatal("message event should be included when explicitly requested")
	}
	if shouldSkipWorkflowReplayMessage(messageEvent, false, 1) {
		t.Fatal("future message event should not be skipped")
	}
	if shouldSkipWorkflowReplayMessage(nodeEvent, false, 2) {
		t.Fatal("non-message event should not be skipped")
	}
	if shouldSkipWorkflowReplayMessage(futureMessageEvent, false, 0) {
		t.Fatal("message after zero replay cutoff should not be skipped")
	}
}

func appendWorkflowEventForReplayTest(t *testing.T, ctx context.Context, service *workflowpause.Service, run *WorkflowRunLog, eventType string, data map[string]interface{}) {
	t.Helper()
	if err := service.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      run.TenantID,
		AppID:         run.AgentID,
		WorkflowRunID: run.ID,
		EventType:     eventType,
		EventData:     data,
	}); err != nil {
		t.Fatalf("append event %s: %v", eventType, err)
	}
}

func assertHeader(t *testing.T, response *http.Response, key, want string) {
	t.Helper()
	if got := response.Header.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
