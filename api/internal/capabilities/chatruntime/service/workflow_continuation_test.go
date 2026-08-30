package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/gorm"
)

func TestWorkflowSummaryLLMRequestUsesOriginalMessageModelAndWorkflowOutputs(t *testing.T) {
	provider := "openai"
	message := &runtimemodel.Message{
		ModelProvider:   &provider,
		ModelName:       "gpt-4.1-mini",
		ModelParameters: map[string]interface{}{"temperature": 0.2, "max_tokens": 256},
	}
	continuation := &WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		OriginalQuery: "请处理这个任务",
	}
	req := workflowSummaryLLMRequest(message, continuation, WorkflowContinuationSummaryRequest{
		Status:  "succeeded",
		Outputs: map[string]interface{}{"answer": "任务结果"},
	})

	if req.Provider != "openai" || req.Model != "gpt-4.1-mini" {
		t.Fatalf("request model = %s/%s, want openai/gpt-4.1-mini", req.Provider, req.Model)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("temperature = %#v, want 0.2", req.Temperature)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 256 {
		t.Fatalf("max_tokens = %#v, want 256", req.MaxTokens)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(req.Messages))
	}
	systemPrompt, ok := req.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("system content type = %T, want string", req.Messages[0].Content)
	}
	for _, want := range []string{"Use only the workflow outputs", "Do not answer the original user request yourself", "workflow outputs do not contain the requested business result"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("summary system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
	content, ok := req.Messages[1].Content.(string)
	if !ok {
		t.Fatalf("user content type = %T, want string", req.Messages[1].Content)
	}
	for _, want := range []string{"请处理这个任务", "run-1", "succeeded", "任务结果"} {
		if !strings.Contains(content, want) {
			t.Fatalf("summary prompt missing %q:\n%s", want, content)
		}
	}
}

func TestWorkflowSummaryLLMRequestHidesPublishedFailureReason(t *testing.T) {
	detail := "no enabled route for deepseek-chat"
	for _, source := range []string{runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI} {
		t.Run(source, func(t *testing.T) {
			req := workflowSummaryLLMRequest(&runtimemodel.Message{}, &WorkflowApprovalContinuation{
				WorkflowRunID: "run-1",
				OriginalQuery: "请运行工作流",
				Caller:        Caller{Source: source},
			}, WorkflowContinuationSummaryRequest{
				Status: "failed",
				Error:  detail,
				Outputs: map[string]interface{}{
					"failure_reason": detail,
				},
			})
			if len(req.Messages) != 2 {
				t.Fatalf("messages len = %d, want 2", len(req.Messages))
			}
			systemPrompt, _ := req.Messages[0].Content.(string)
			userPrompt, _ := req.Messages[1].Content.(string)
			combined := systemPrompt + "\n" + userPrompt
			if strings.Contains(combined, detail) {
				t.Fatalf("published summary prompt exposed detailed error:\n%s", combined)
			}
			for _, want := range []string{publishedWorkflowFailureError, "only that the workflow run failed", "do not state or infer a specific reason"} {
				if !strings.Contains(combined, want) {
					t.Fatalf("published summary prompt missing %q:\n%s", want, combined)
				}
			}
		})
	}
}

func TestWorkflowSummaryLLMRequestKeepsConsoleDebugFailureReason(t *testing.T) {
	detail := "no enabled route for deepseek-chat"
	req := workflowSummaryLLMRequest(&runtimemodel.Message{}, &WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		Caller:        Caller{Source: runtimemodel.ConversationSourceConsole},
	}, WorkflowContinuationSummaryRequest{Status: "failed", Error: detail})
	userPrompt, _ := req.Messages[1].Content.(string)
	if !strings.Contains(userPrompt, detail) {
		t.Fatalf("console summary prompt = %q, want detailed error", userPrompt)
	}
}

func TestWorkflowContinuationMetadataWithStatusPreservesExistingFields(t *testing.T) {
	runID := uuid.NewString()
	metadata := workflowContinuationMetadataWithStatus(map[string]interface{}{
		"agent_workflow_continuation": map[string]interface{}{
			"workflow_run_id": runID,
			"binding_id":      "binding-1",
			"status":          "waiting_approval",
		},
	}, workflowContinuationStatusSummarizing)

	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	if got := firstNonEmptyString(state["workflow_run_id"]); got != runID {
		t.Fatalf("workflow_run_id = %q, want %q", got, runID)
	}
	if got := firstNonEmptyString(state["binding_id"]); got != "binding-1" {
		t.Fatalf("binding_id = %q, want binding-1", got)
	}
	if got := firstNonEmptyString(state["status"]); got != workflowContinuationStatusSummarizing {
		t.Fatalf("status = %q, want %q", got, workflowContinuationStatusSummarizing)
	}
}

func TestWorkflowApprovalContinuationFromMetadataRestoresInvocationIdentity(t *testing.T) {
	continuation := workflowApprovalContinuationFromMetadata(map[string]interface{}{
		"agent_workflow_continuation": map[string]interface{}{
			"workflow_run_id":             "run-1",
			"invocation_id":               "invocation-1",
			"invocation_mode":             "agent_task_tool",
			"invocation_protocol_version": 2,
			"ui_approval_allowed":         true,
		},
	})

	if continuation.InvocationID != "invocation-1" || continuation.InvocationMode != "agent_task_tool" {
		t.Fatalf("unexpected invocation identity: %+v", continuation)
	}
	if continuation.InvocationProtocolVersion != 2 {
		t.Fatalf("invocation protocol version = %d, want 2", continuation.InvocationProtocolVersion)
	}
	if !continuation.UIApprovalAllowed {
		t.Fatal("ui approval capability was not restored")
	}
}

func TestWorkflowContinuationInlineApprovalHonorsSurfaceCapability(t *testing.T) {
	for _, tt := range []struct {
		name    string
		source  string
		allowed bool
		want    bool
	}{
		{name: "draft override", source: runtimemodel.ConversationSourceConsole, allowed: false, want: true},
		{name: "webapp enabled", source: runtimemodel.ConversationSourceWebApp, allowed: true, want: true},
		{name: "webapp disabled", source: runtimemodel.ConversationSourceWebApp, allowed: false, want: false},
		{name: "webapp missing capability", source: runtimemodel.ConversationSourceWebApp, allowed: false, want: false},
		{name: "external api enabled", source: runtimemodel.ConversationSourceExternalAPI, allowed: true, want: true},
		{name: "external api disabled", source: runtimemodel.ConversationSourceExternalAPI, allowed: false, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := workflowContinuationAllowsInlineApproval(
				Caller{Source: tt.source},
				&WorkflowApprovalContinuation{UIApprovalAllowed: tt.allowed},
			)
			if got != tt.want {
				t.Fatalf("workflowContinuationAllowsInlineApproval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkflowTaskContinuationMessageCarriesStructuredToolEvidence(t *testing.T) {
	message := workflowTaskContinuationMessage(&WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		InvocationID:  "invocation-1",
	}, WorkflowContinuationSummaryRequest{
		WorkflowRunID: "run-1",
		Status:        "succeeded",
		Outputs:       map[string]interface{}{"result": "done"},
	})
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("message content type = %T, want string", message.Content)
	}
	for _, expected := range []string{"invocation-1", "run-1", "succeeded", "done", "do not rerun"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("task continuation message missing %q:\n%s", expected, content)
		}
	}
}

func TestWorkflowTaskContinuationMessageHidesPublishedFailureOutputs(t *testing.T) {
	detail := "provider route secret"
	message := workflowTaskContinuationMessage(&WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		InvocationID:  "invocation-1",
		Caller:        Caller{Source: runtimemodel.ConversationSourceWebApp},
	}, WorkflowContinuationSummaryRequest{
		Status:  "exception",
		Error:   detail,
		Outputs: map[string]interface{}{"error_detail": detail},
	})
	content, _ := message.Content.(string)
	if strings.Contains(content, detail) {
		t.Fatalf("published task continuation message exposed failure detail:\n%s", content)
	}
	if !strings.Contains(content, publishedWorkflowFailureError) || !strings.Contains(content, "only that the workflow run failed") {
		t.Fatalf("published task continuation message = %q, want generic failure instruction", content)
	}
}

func TestValidateWorkflowContinuationBindingMatchesBindingAndAgent(t *testing.T) {
	continuation := &WorkflowApprovalContinuation{BindingID: "binding-1", AgentID: "agent-1"}
	bindings := []AgentWorkflowBinding{{BindingID: "binding-1", AgentID: "agent-other"}}
	if err := validateWorkflowContinuationBinding(continuation, bindings); !errors.Is(err, ErrWorkflowBindingUnavailable) {
		t.Fatalf("validateWorkflowContinuationBinding() agent mismatch error = %v, want ErrWorkflowBindingUnavailable", err)
	}
	bindings = append(bindings, AgentWorkflowBinding{BindingID: "binding-1", AgentID: "agent-1"})
	if err := validateWorkflowContinuationBinding(continuation, bindings); err != nil {
		t.Fatalf("validateWorkflowContinuationBinding() error = %v, want active match", err)
	}
}

func TestWorkflowContinuationAppendStreamEventAddsRuntimeIDs(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	continuation := &WorkflowApprovalContinuation{
		ConversationID: conversationID,
		MessageID:      messageID,
		WorkflowRunID:  "workflow-run-1",
	}

	event, err := (&service{}).AppendWorkflowApprovalContinuationStreamEvent(context.Background(), continuation, "workflow_finished", map[string]interface{}{
		"status": "succeeded",
	})
	if err != nil {
		t.Fatalf("AppendWorkflowApprovalContinuationStreamEvent() error = %v", err)
	}
	if event == nil {
		t.Fatal("AppendWorkflowApprovalContinuationStreamEvent() returned nil event")
	}
	if event.EventType != "workflow_finished" {
		t.Fatalf("event type = %q, want workflow_finished", event.EventType)
	}
	if got := firstNonEmptyString(event.Payload["conversation_id"]); got != conversationID.String() {
		t.Fatalf("conversation_id = %q, want %q", got, conversationID.String())
	}
	if got := firstNonEmptyString(event.Payload["message_id"]); got != messageID.String() {
		t.Fatalf("message_id = %q, want %q", got, messageID.String())
	}
	if got := firstNonEmptyString(event.Payload["workflow_run_id"]); got != "workflow-run-1" {
		t.Fatalf("workflow_run_id = %q, want workflow-run-1", got)
	}
}

func TestWorkflowContinuationEventsHidePublishedFailureReason(t *testing.T) {
	detail := "node failed: no enabled route for deepseek-chat"
	for _, source := range []string{runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI} {
		t.Run(source, func(t *testing.T) {
			continuation := &WorkflowApprovalContinuation{
				ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "workflow-run-1",
				Caller: Caller{Source: source},
			}
			event, err := (&service{}).AppendWorkflowApprovalContinuationStreamEvent(t.Context(), continuation, "node_finished", map[string]interface{}{
				"status": "exception", "error": detail, "message": detail,
				"outputs": map[string]interface{}{"failure_reason": detail},
			})
			if err != nil {
				t.Fatalf("AppendWorkflowApprovalContinuationStreamEvent() error = %v", err)
			}
			if event.Payload["error"] != publishedWorkflowFailureError || event.Payload["message"] != publishedWorkflowFailureError {
				t.Fatalf("event payload = %#v, want generic failure", event.Payload)
			}
			if event.Payload["error_visibility"] != publishedWorkflowErrorVisibility || strings.Contains(fmt.Sprint(event.Payload), detail) {
				t.Fatalf("event payload exposed detailed failure: %#v", event.Payload)
			}
		})
	}
}

func TestProjectWorkflowContinuationEventProtectsFallbackEmission(t *testing.T) {
	detail := "fallback persistence failure: database secret"
	continuation := &WorkflowApprovalContinuation{Caller: Caller{Source: runtimemodel.ConversationSourceWebApp}}
	event := ProjectWorkflowContinuationEvent(continuation, StreamEvent{
		EventType: "error",
		Payload: map[string]interface{}{
			"message": detail,
			"outputs": map[string]interface{}{"provider_error": detail},
		},
	})
	if strings.Contains(fmt.Sprint(event.Payload), detail) {
		t.Fatalf("fallback event exposed detailed failure: %#v", event.Payload)
	}
	if event.Payload["message"] != publishedWorkflowFailureError || event.Payload["error"] != publishedWorkflowFailureError {
		t.Fatalf("fallback event = %#v, want generic failure", event.Payload)
	}
}

func TestWorkflowContinuationEventsKeepConsoleDebugFailureReason(t *testing.T) {
	detail := "node failed: no enabled route for deepseek-chat"
	continuation := &WorkflowApprovalContinuation{
		ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "workflow-run-1",
		Caller: Caller{Source: runtimemodel.ConversationSourceConsole},
	}
	event, err := (&service{}).AppendWorkflowApprovalContinuationStreamEvent(t.Context(), continuation, "workflow_failed", map[string]interface{}{
		"status": "failed", "error": detail,
	})
	if err != nil {
		t.Fatalf("AppendWorkflowApprovalContinuationStreamEvent() error = %v", err)
	}
	if event.Payload["error"] != detail {
		t.Fatalf("console event payload = %#v, want detailed error", event.Payload)
	}
}

func TestTerminalStreamEventIgnoresWaitingMessageEnd(t *testing.T) {
	for _, status := range []string{
		runtimemodel.MessageStatusWaitingApproval,
		runtimemodel.MessageStatusWaitingQuestion,
		"",
	} {
		if isTerminalStreamEvent(StreamEvent{
			EventType: streamEventMessageEnd,
			Payload:   map[string]interface{}{"status": status},
		}) {
			t.Fatalf("message_end status %q should not be terminal", status)
		}
	}
	for _, status := range []string{
		runtimemodel.MessageStatusCompleted,
		runtimemodel.MessageStatusStopped,
		runtimemodel.MessageStatusError,
		"failed",
	} {
		if !isTerminalStreamEvent(StreamEvent{
			EventType: streamEventMessageEnd,
			Payload:   map[string]interface{}{"status": status},
		}) {
			t.Fatalf("message_end status %q should be terminal", status)
		}
	}
}

func TestWorkflowNoDisplayableOutputAnswer(t *testing.T) {
	got := workflowNoDisplayableOutputAnswer("run-empty")
	if !strings.Contains(got, "工作流已运行，但未返回可展示输出") || !strings.Contains(got, "run-empty") {
		t.Fatalf("workflowNoDisplayableOutputAnswer() = %q", got)
	}
}

func TestWorkflowContinuationCheckpointsAndStopsPartialAnswer(t *testing.T) {
	messageRepo := &workflowContinuationCheckpointMessageRepo{}
	conversationRepo := &workflowContinuationCheckpointConversationRepo{}
	svc := &service{repos: &repository.Repositories{Message: messageRepo, Conversation: conversationRepo}}
	continuation := &WorkflowApprovalContinuation{
		ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "workflow-run-checkpoint",
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run-checkpoint"},
		},
	}

	if _, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "text_chunk", map[string]interface{}{"text": "first"}); err != nil {
		t.Fatalf("record first chunk: %v", err)
	}
	if messageRepo.partialAnswer != "first" || messageRepo.partialUpdates != 1 {
		t.Fatalf("first checkpoint answer=%q updates=%d", messageRepo.partialAnswer, messageRepo.partialUpdates)
	}
	if _, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "text_chunk", map[string]interface{}{"text": " second"}); err != nil {
		t.Fatalf("record second chunk: %v", err)
	}
	if messageRepo.partialUpdates != 1 {
		t.Fatalf("throttled checkpoint updates=%d, want 1", messageRepo.partialUpdates)
	}
	if _, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "node_finished", map[string]interface{}{"text": "technical node text"}); err != nil {
		t.Fatalf("record technical workflow event: %v", err)
	}
	if messageRepo.partialUpdates != 1 {
		t.Fatalf("technical events must not enter the answer checkpoint; updates=%d", messageRepo.partialUpdates)
	}
	if _, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "workflow_finished", map[string]interface{}{"status": "stopped"}); err != nil {
		t.Fatalf("record terminal event: %v", err)
	}
	if messageRepo.partialAnswer != "first second" || messageRepo.partialUpdates != 2 {
		t.Fatalf("forced checkpoint answer=%q updates=%d", messageRepo.partialAnswer, messageRepo.partialUpdates)
	}
	metadata, err := svc.CompleteWorkflowApprovalContinuation(t.Context(), continuation, "first second", runtimemodel.MessageStatusStopped)
	if err != nil {
		t.Fatalf("stop continuation: %v", err)
	}
	if messageRepo.stoppedAnswer != "first second" || messageRepo.completedUpdates != 0 {
		t.Fatalf("stopped answer=%q completed updates=%d", messageRepo.stoppedAnswer, messageRepo.completedUpdates)
	}
	if !conversationRepo.finished {
		t.Fatal("stopped continuation did not release the active message")
	}
	conversationRepo.finishErr = gorm.ErrRecordNotFound
	if _, err := svc.CompleteWorkflowApprovalContinuation(t.Context(), continuation, "", runtimemodel.MessageStatusStopped); err != nil {
		t.Fatalf("idempotent stop after active message was already cleared: %v", err)
	}
	if messageRepo.stoppedAnswer != "first second" {
		t.Fatalf("idempotent stop erased partial answer: %q", messageRepo.stoppedAnswer)
	}
	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	if status := firstNonEmptyString(state["status"]); status != runtimemodel.MessageStatusStopped {
		t.Fatalf("continuation status = %q, want stopped", status)
	}
}

func TestWorkflowContinuationStoppedAnswerUsesObservedEventText(t *testing.T) {
	messageRepo := &workflowContinuationCheckpointMessageRepo{}
	conversationRepo := &workflowContinuationCheckpointConversationRepo{}
	svc := &service{repos: &repository.Repositories{Message: messageRepo, Conversation: conversationRepo}}
	continuation := &WorkflowApprovalContinuation{
		ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "workflow-run-tail",
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run-tail"},
		},
	}

	for _, chunk := range []string{"七", "零八", "落", "\n", "一心", "一", "意", "\n"} {
		if _, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "text_chunk", map[string]interface{}{"text": chunk}); err != nil {
			t.Fatalf("record chunk %q: %v", chunk, err)
		}
	}
	if _, err := svc.CompleteWorkflowApprovalContinuation(t.Context(), continuation, "七零八落\n一心", runtimemodel.MessageStatusStopped); err != nil {
		t.Fatalf("complete stopped continuation: %v", err)
	}
	if got, want := messageRepo.stoppedAnswer, "七零八落\n一心一意\n"; got != want {
		t.Fatalf("stopped answer = %q, want %q", got, want)
	}
}

func TestWorkflowContinuationTextReplaceMayShortenOrClearAnswer(t *testing.T) {
	messageRepo := &workflowContinuationCheckpointMessageRepo{}
	svc := &service{repos: &repository.Repositories{
		Message:      messageRepo,
		Conversation: &workflowContinuationCheckpointConversationRepo{},
	}}
	continuation := &WorkflowApprovalContinuation{ConversationID: uuid.New(), MessageID: uuid.New()}

	if err := svc.checkpointWorkflowContinuationAnswer(t.Context(), continuation, "text_chunk", map[string]interface{}{"text": "a much longer draft"}, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.checkpointWorkflowContinuationAnswer(t.Context(), continuation, "text_replace", map[string]interface{}{"text": "short"}, true); err != nil {
		t.Fatal(err)
	}
	if got := continuation.answer; got != "short" {
		t.Fatalf("short replacement = %q, want short", got)
	}
	if err := svc.checkpointWorkflowContinuationAnswer(t.Context(), continuation, "text_replace", map[string]interface{}{"text": ""}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteWorkflowApprovalContinuation(t.Context(), continuation, "stale", runtimemodel.MessageStatusStopped); err != nil {
		t.Fatal(err)
	}
	if got := messageRepo.stoppedAnswer; got != "" {
		t.Fatalf("cleared stopped answer = %q, want empty", got)
	}
}

func TestWorkflowContinuationWithoutObservedTextPreservesDatabaseAnswer(t *testing.T) {
	messageRepo := &workflowContinuationCheckpointMessageRepo{}
	svc := &service{repos: &repository.Repositories{
		Message:      messageRepo,
		Conversation: &workflowContinuationCheckpointConversationRepo{},
	}}
	continuation := &WorkflowApprovalContinuation{ConversationID: uuid.New(), MessageID: uuid.New()}

	if _, err := svc.CompleteWorkflowApprovalContinuation(t.Context(), continuation, "", runtimemodel.MessageStatusStopped); err != nil {
		t.Fatal(err)
	}
	if messageRepo.preserveStoppedUpdates != 1 {
		t.Fatalf("preserve stopped updates = %d, want 1", messageRepo.preserveStoppedUpdates)
	}
	if messageRepo.stoppedAnswerUpdates != 0 {
		t.Fatalf("stopped answer writes = %d, want 0", messageRepo.stoppedAnswerUpdates)
	}
}

type workflowContinuationCheckpointMessageRepo struct {
	repository.MessageRepository
	partialAnswer          string
	stoppedAnswer          string
	partialUpdates         int
	completedUpdates       int
	stoppedAnswerUpdates   int
	preserveStoppedUpdates int
}

func (r *workflowContinuationCheckpointMessageRepo) UpdatePartialAnswer(_ context.Context, _ uuid.UUID, answer string, _ map[string]interface{}) error {
	r.partialAnswer = answer
	r.partialUpdates++
	return nil
}

func (r *workflowContinuationCheckpointMessageRepo) UpdateStoppedAnswer(_ context.Context, _ uuid.UUID, answer string, _ map[string]interface{}) error {
	r.stoppedAnswer = answer
	r.stoppedAnswerUpdates++
	return nil
}

func (r *workflowContinuationCheckpointMessageRepo) UpdateStoppedPreservingAnswer(context.Context, uuid.UUID, map[string]interface{}) error {
	r.preserveStoppedUpdates++
	return nil
}

func (r *workflowContinuationCheckpointMessageRepo) UpdateCompleted(context.Context, uuid.UUID, string, map[string]interface{}) error {
	r.completedUpdates++
	return nil
}

func (r *workflowContinuationCheckpointMessageRepo) UpdateMetadata(context.Context, uuid.UUID, map[string]interface{}) error {
	return nil
}

type workflowContinuationCheckpointConversationRepo struct {
	repository.ConversationRepository
	finished  bool
	finishErr error
}

func (r *workflowContinuationCheckpointConversationRepo) FinishContinuationMessage(context.Context, uuid.UUID, uuid.UUID) error {
	r.finished = true
	return r.finishErr
}
