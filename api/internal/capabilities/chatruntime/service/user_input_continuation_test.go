package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeUserInputContinuationResponseRequiresEveryAnswer(t *testing.T) {
	request := map[string]interface{}{
		"questions": []interface{}{
			map[string]interface{}{"id": "target", "question": "Which target?"},
			map[string]interface{}{"question": "Include a summary?"},
		},
	}

	_, err := normalizeUserInputContinuationResponse("ask-1", request, map[string]string{
		"target": "Current Agent",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("normalizeUserInputContinuationResponse() error = %v, want ErrInvalidInput", err)
	}

	response, err := normalizeUserInputContinuationResponse("ask-1", request, map[string]string{
		"target": "Current Agent",
		"q2":     "Yes",
	})
	if err != nil {
		t.Fatalf("normalizeUserInputContinuationResponse() error = %v", err)
	}
	if got := stringFromAny(response["status"]); got != userInputContinuationStatusAnswered {
		t.Fatalf("response status = %q, want answered", got)
	}
	if got, _ := response["answer_count"].(int); got != 2 {
		t.Fatalf("answer_count = %d, want 2", got)
	}
}

func TestResolveUserInputContinuationMetadataKeepsSameTurnEvidence(t *testing.T) {
	request := map[string]interface{}{
		"request_id": "ask-1",
		"message":    "I need one clarification.",
		"questions": []interface{}{
			map[string]interface{}{"id": "target", "question": "Which target?"},
		},
	}
	response, err := normalizeUserInputContinuationResponse("ask-1", request, map[string]string{
		"target": "Current Agent",
	})
	if err != nil {
		t.Fatalf("normalize response: %v", err)
	}
	metadata := resolveUserInputContinuationMetadata(map[string]interface{}{
		"user_input_request": request,
		"user_input_continuation": map[string]interface{}{
			"status":         "waiting_question",
			"original_query": "Update the Agent",
		},
		"operation_plan": map[string]interface{}{
			"status": "running",
		},
	}, request, response)

	if _, exists := metadata["user_input_request"]; exists {
		t.Fatalf("user_input_request still active: %#v", metadata["user_input_request"])
	}
	if !userInputResponseRecorded(metadata, "ask-1") {
		t.Fatalf("user input response was not recorded: %#v", metadata["user_input_responses"])
	}
	continuation := mapFromOperationContext(metadata["user_input_continuation"])
	if got := stringFromAny(continuation["original_query"]); got != "Update the Agent" {
		t.Fatalf("original_query = %q, want preserved query", got)
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["pending_next_action"]); got != userInputPendingActionReplan {
		t.Fatalf("plan pending_next_action = %q, want %q", got, userInputPendingActionReplan)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 1 {
		t.Fatalf("evidence ledger = %#v, want one user input response", ledger)
	}
	if got := stringFromAny(ledger[0]["invocation_id"]); got != "runtime_id:user_input:ask-1" {
		t.Fatalf("invocation_id = %q", got)
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if got := stringFromAny(summary["pending_next_action"]); got != userInputPendingActionReplan {
		t.Fatalf("summary pending_next_action = %q, want %q", got, userInputPendingActionReplan)
	}
}

func TestUserInputContinuationMessageRequiresPlanRevisionBeforeBusinessTools(t *testing.T) {
	message := userInputContinuationMessage(
		&runtimemodel.Message{Query: "Update the Agent after I choose the target"},
		map[string]interface{}{"message": "Choose the target."},
		map[string]interface{}{
			"request_id": "ask-1",
			"answers":    []interface{}{map[string]interface{}{"question_id": "target", "value": "Current Agent"}},
		},
	)
	content := stringFromAny(message.Content)
	for _, want := range []string{"revise the current plan with update_plan", "update_plan first and the next business tool in the same assistant response", "Preserve completed phases"} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message = %q, want %q", content, want)
		}
	}
}

func TestPlanUpdateAfterUserInputClearsReplanState(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"pending_next_action": userInputPendingActionReplan,
			"phase_revision":      2,
		},
		"operation_result_summary": map[string]interface{}{
			"status":              "user_input_received",
			"plan_status":         operationPlanStatusRunning,
			"pending_next_action": userInputPendingActionReplan,
		},
		"user_input_continuation": map[string]interface{}{
			"status":      userInputContinuationStatusAnswered,
			"request_id":  "ask-1",
			"next_action": userInputPendingActionReplan,
		},
	}
	applyPlanUpdateTraceMetadata(metadata, skills.SkillTrace{
		Kind:   "plan_update",
		Status: "success",
		Result: map[string]interface{}{
			"plan": []interface{}{
				map[string]interface{}{"id": "phase-1", "step": "Use the clarified target", "status": "in_progress"},
			},
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["pending_next_action"]); got != "" {
		t.Fatalf("plan pending_next_action = %q, want cleared", got)
	}
	if got := intValueFromAny(plan["phase_revision"]); got != 3 {
		t.Fatalf("plan phase_revision = %d, want 3", got)
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if got := stringFromAny(summary["pending_next_action"]); got != "" {
		t.Fatalf("summary pending_next_action = %q, want cleared", got)
	}
	if got := stringFromAny(summary["status"]); got != operationPlanStatusRunning {
		t.Fatalf("summary status = %q, want running", got)
	}
	continuation := mapFromOperationContext(metadata["user_input_continuation"])
	if got := stringFromAny(continuation["status"]); got != "plan_updated" {
		t.Fatalf("continuation status = %q, want plan_updated", got)
	}
	if got := intValueFromAny(continuation["plan_revision"]); got != 3 {
		t.Fatalf("continuation plan_revision = %d, want 3", got)
	}
}

func TestBeginUserInputContinuationResumesCurrentLeafWithoutCreatingMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		DialogueCount:        1,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Status:         runtimemodel.MessageStatusWaitingQuestion,
		Query:          "Update the Agent after clarifying the target",
		Metadata: map[string]interface{}{
			"user_input_request": map[string]interface{}{
				"request_id": "ask-1",
				"message":    "Choose the target.",
				"questions": []interface{}{
					map[string]interface{}{"id": "target", "question": "Which target?"},
				},
			},
		},
	}
	svc := &service{repos: &repository.Repositories{
		Conversation: fixedUserInputConversationRepo{conversation: conversation},
		Message:      fixedUserInputMessageRepo{message: message},
	}}

	continuation, err := svc.beginUserInputContinuation(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		conversationID,
		messageID,
		"ask-1",
		map[string]string{"target": "Current Agent"},
	)
	if err != nil {
		t.Fatalf("beginUserInputContinuation() error = %v", err)
	}
	if continuation.Message.ID != messageID || continuation.Conversation.CurrentLeafMessageID == nil || *continuation.Conversation.CurrentLeafMessageID != messageID {
		t.Fatalf("continuation changed message identity: %#v", continuation)
	}
	if continuation.Conversation.DialogueCount != 1 {
		t.Fatalf("dialogue_count = %d, want 1", continuation.Conversation.DialogueCount)
	}
	if continuation.Message.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("message status = %q, want streaming", continuation.Message.Status)
	}
	if _, exists := continuation.Message.Metadata["user_input_request"]; exists {
		t.Fatal("resolved request remains active")
	}

	_, err = svc.beginUserInputContinuation(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		conversationID,
		messageID,
		"ask-1",
		map[string]string{"target": "Current Agent"},
	)
	if !IsContinuationAlreadyRunningError(err) {
		t.Fatalf("duplicate begin error = %v, want continuation already running", err)
	}
}

type fixedUserInputConversationRepo struct {
	repository.ConversationRepository
	conversation *runtimemodel.Conversation
}

func (r fixedUserInputConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

type fixedUserInputMessageRepo struct {
	repository.MessageRepository
	message *runtimemodel.Message
}

func (r fixedUserInputMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}
