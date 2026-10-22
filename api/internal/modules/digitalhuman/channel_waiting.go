package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// A channel has no confirmation card or question form, so a run that stops to
// wait sends what it waits for as a message, and the next reply answers it.

const (
	channelReplyTaskApproval    = "回复「确认」执行，回复「取消」放弃；也可以直接说明需要修改的地方。"
	channelReplyWorkflow        = "回复「同意」或「拒绝」。"
	channelReplyConnector       = "回复「确认」执行这个操作，回复「取消」放弃。"
	channelReplyQuestion        = "直接回复即可，多个问题可以分行依次回答。"
	channelConnectorReplyNotice = "这个操作需要明确确认：请回复「确认」执行，或回复「取消」放弃。"
	// A group conversation without a known requester cannot say who may
	// confirm an operation in a connected system, so it is confirmed in the console.
	channelConnectorGroupNotice   = "这个操作需要由发起人确认，但当前无法识别发起人。请到 ZGI 控制台的运行记录中确认。"
	channelUnsupportedWaitNotice  = "这一步需要在 ZGI 控制台中处理，处理完成后会继续执行。"
	channelWorkflowQuestionNotice = "这个问题来自工作流，需要在 ZGI 控制台或网页应用中回答。"
)

// ChannelConnectorDecision is the outcome of confirming or cancelling a
// connected-system operation from a channel reply.
type ChannelConnectorDecision struct {
	// State is completed, cancelled, failed, or unknown.
	State     string
	Title     string
	Summary   string
	ErrorCode string
}

// ChannelConnectorConfirmer decides a pending connector confirmation for the
// account that owns it, through the same continuation the console uses.
type ChannelConnectorConfirmer interface {
	DecideChannelConnectorConfirmation(ctx context.Context, organizationID, ownerAccountID, receipt uuid.UUID, confirm bool) (ChannelConnectorDecision, error)
}

// SetChannelConnectorConfirmer lets channel replies decide connector
// confirmations. Without it the channel user is told to use the console.
func (s *RuntimeService) SetChannelConnectorConfirmer(confirmer ChannelConnectorConfirmer) {
	s.channelConnectors = confirmer
}

// deliverChannelWaitingPrompt sends what a channel run now waits for, with
// how to answer it. It is best effort: the run already waits durably, and a
// lost prompt can be answered from the console.
func (s *RuntimeService) deliverChannelWaitingPrompt(ctx context.Context, run *Run, messageID uuid.UUID, status, answer string, metadata map[string]interface{}) {
	text := channelWaitingPromptText(run, status, answer, metadata)
	if text == "" || run == nil {
		return
	}
	deliveryStore, ok := s.store.(channelFinalDeliveryStore)
	if !ok {
		return
	}
	// One prompt per message and wait: a resumed loop that waits again on the
	// same message asks anew.
	key := "waiting:" + messageID.String() + ":" + status
	if err := deliveryStore.EnqueueRunChannelFinalDelivery(context.WithoutCancel(ctx), run.ID, key, text, s.now()); err != nil {
		logger.WarnContext(ctx, "failed to deliver digital human channel waiting prompt", "run_id", run.ID.String(), "status", status, err)
	}
}

func channelWaitingPromptText(run *Run, status, answer string, metadata map[string]interface{}) string {
	// A run that has just stopped holds its metadata as Go values, not the
	// JSON it is stored as; read it the way it is stored.
	metadata = jsonNormalizedMap(metadata)
	answer = strings.TrimSpace(answer)
	parts := make([]string, 0, 3)
	if answer != "" {
		parts = append(parts, answer)
	}
	switch status {
	case runtimemodel.MessageStatusWaitingQuestion:
		request, _ := metadata["user_input_request"].(map[string]interface{})
		if source, _ := request["source"].(string); strings.EqualFold(strings.TrimSpace(source), "agent_workflow_question_answer") {
			parts = append(parts, channelWorkflowQuestionNotice)
			break
		}
		// The model's own lead-in to its questions, when it wrote no answer.
		if message := strings.TrimSpace(stringValue(request["message"])); message != "" && !strings.Contains(answer, message) {
			parts = append(parts, message)
		}
		if questions := channelQuestionTexts(request); len(questions) > 0 && !containsAll(answer, questions) {
			parts = append(parts, strings.Join(questions, "\n"))
		}
		parts = append(parts, channelReplyQuestion)
	case runtimemodel.MessageStatusWaitingApproval:
		state := workflowContinuationRecord(metadata)
		switch {
		case strings.TrimSpace(stringValue(state["kind"])) == "digital_human_task":
			// The proposal itself, as the console card shows it.
			if details := channelTaskApprovalDetails(state); details != "" {
				parts = append(parts, details)
			} else if answer == "" {
				if name := strings.TrimSpace(stringValue(state["task_name"])); name != "" {
					parts = append(parts, "需要你确认任务「"+name+"」。")
				}
			}
			parts = append(parts, channelReplyTaskApproval)
		case strings.TrimSpace(stringValue(state["workflow_run_id"])) != "":
			if allowed, _ := state["ui_approval_allowed"].(bool); allowed {
				parts = append(parts, channelReplyWorkflow)
			} else {
				parts = append(parts, channelWorkflowApprovalWaitingNotice)
			}
		case run != nil && run.WaitingReceipt != nil:
			if answer == "" {
				parts = append(parts, "数字人需要执行一个外部操作，需要你确认。")
			}
			parts = append(parts, channelReplyConnector)
		default:
			parts = append(parts, channelUnsupportedWaitNotice)
		}
	default:
		parts = append(parts, channelUnsupportedWaitNotice)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func jsonNormalizedMap(value map[string]interface{}) map[string]interface{} {
	if len(value) == 0 {
		return value
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	normalized := map[string]interface{}{}
	if json.Unmarshal(encoded, &normalized) != nil {
		return value
	}
	return normalized
}

func channelQuestionTexts(request map[string]interface{}) []string {
	raw, _ := request["questions"].([]interface{})
	texts := make([]string, 0, len(raw))
	for index, item := range raw {
		question, _ := item.(map[string]interface{})
		text := strings.TrimSpace(stringValue(question["question"]))
		if text == "" {
			text = strings.TrimSpace(stringValue(question["text"]))
		}
		if text == "" {
			continue
		}
		if len(raw) > 1 {
			text = fmt.Sprintf("%d. %s", index+1, text)
		}
		texts = append(texts, text)
	}
	return texts
}

func containsAll(text string, parts []string) bool {
	if text == "" {
		return false
	}
	for _, part := range parts {
		if !strings.Contains(text, strings.TrimLeft(part, "0123456789. ")) {
			return false
		}
	}
	return true
}

// channelQuestionAnswers maps a channel reply onto the pending questions: one
// line per question when the reply has exactly as many lines, otherwise the
// whole reply answers each question.
func channelQuestionAnswers(request map[string]interface{}, reply string) map[string]string {
	raw, _ := request["questions"].([]interface{})
	ids := make([]string, 0, len(raw))
	for index, item := range raw {
		question, _ := item.(map[string]interface{})
		id := strings.TrimSpace(stringValue(question["id"]))
		if id == "" {
			id = fmt.Sprintf("q%d", index+1)
		}
		ids = append(ids, id)
	}
	reply = strings.TrimSpace(reply)
	answers := make(map[string]string, len(ids))
	if len(ids) == 0 || reply == "" {
		return answers
	}
	lines := make([]string, 0, len(ids))
	for _, line := range strings.Split(reply, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, strings.TrimSpace(strings.TrimLeft(line, "0123456789.、)） ")))
		}
	}
	for index, id := range ids {
		if len(ids) > 1 && len(lines) == len(ids) && lines[index] != "" {
			answers[id] = lines[index]
		} else {
			answers[id] = reply
		}
	}
	return answers
}

// settleChannelContinuationRun settles a run resumed from a channel reply.
// The reply's own answer is delivered by the caller, so completion does not
// deliver it again; a run that waits again asks its next question.
func (s *RuntimeService) settleChannelContinuationRun(ctx context.Context, run *Run, result *service.ChatResult, messageID uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	status, answer := "", ""
	var metadata map[string]interface{}
	if result != nil {
		status, answer, metadata = result.Status, result.Answer, result.Metadata
	}
	var err error
	switch status {
	case runtimemodel.MessageStatusCompleted:
		err = s.store.CompleteRun(ctx, run.ID, answer, executionTraceFromMetadata(metadata), s.now())
	case runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion,
		runtimemodel.MessageStatusWaitingBudget, runtimemodel.MessageStatusWaitingClientAction:
		waiter, ok := s.store.(runWaitingStore)
		if !ok {
			err = fmt.Errorf("digital human waiting run resume is unavailable")
			break
		}
		if err = waiter.MarkRunWaiting(ctx, run.ID, status, s.now()); err == nil {
			s.deliverChannelWaitingPrompt(ctx, run, messageID, status, answer, metadata)
		}
	case runtimemodel.MessageStatusStopped:
		err = s.store.FailRun(ctx, run.ID, "digital_human.run.canceled", "The DigitalHuman run was stopped.", s.now())
	default:
		err = s.store.FailRun(ctx, run.ID, workflowContinuationFailedCode, workflowContinuationFailedMessage, s.now())
	}
	if err != nil && !errors.Is(err, ErrRunConflict) {
		logger.WarnContext(ctx, "failed to settle digital human channel continuation run", "run_id", run.ID.String(), "status", status, err)
	}
}

// holdChannelRunAgain puts a resumed run back to waiting after its answer was
// rejected, so the next reply can answer it.
func (s *RuntimeService) holdChannelRunAgain(ctx context.Context, run *Run, reason string) {
	if waiter, ok := s.store.(runWaitingStore); !ok ||
		waiter.MarkRunWaiting(context.WithoutCancel(ctx), run.ID, reason, s.now()) != nil {
		s.settleChannelContinuationRun(ctx, run, nil, uuid.Nil)
	}
}

// continueChannelQuestion answers the model's pending question with the
// channel reply and delivers what the resumed loop answers.
func (s *RuntimeService) continueChannelQuestion(ctx context.Context, channelScope Scope, digitalHumanID, conversationID uuid.UUID, pending *runtimemodel.Message, run *Run, text string) (bool, error) {
	request, _ := pending.Metadata["user_input_request"].(map[string]interface{})
	if source, _ := request["source"].(string); strings.EqualFold(strings.TrimSpace(source), "agent_workflow_question_answer") {
		return true, s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, "question-workflow:"+uuid.NewString(), channelWorkflowQuestionNotice)
	}
	requestID := strings.TrimSpace(stringValue(request["request_id"]))
	answers := channelQuestionAnswers(request, text)
	if requestID == "" || len(answers) == 0 {
		return false, nil
	}
	continuationRuntime, ok := s.chat.(digitalHumanUserInputContinuationRuntime)
	if !ok {
		return false, fmt.Errorf("digital human channel question continuation is unavailable")
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return false, err
	}
	config, cleanup, err := s.userInputContinuationConfig(ctx, run, resolved, event, channelScope)
	if err != nil {
		return false, err
	}
	defer cleanup()
	runtimeScope, caller := channelConversationContext(channelScope, digitalHumanID)
	caller.RunID = cloneUUIDPointer(&run.ID)
	held := run.Status == RunStatusWaiting && run.WaitingReason == runtimemodel.MessageStatusWaitingQuestion
	if held {
		if err := s.resumeHeldWorkflowRun(ctx, run); err != nil {
			return false, err
		}
	}
	result, err := continuationRuntime.RunConfiguredUserInputContinuationStream(
		ctx, runtimeScope, caller, config, conversationID, pending.ID, requestID,
		runtimedto.UserInputContinuationRequest{Answers: answers}, func(service.StreamEvent) error { return nil },
	)
	if err != nil {
		if held {
			s.holdChannelRunAgain(ctx, run, runtimemodel.MessageStatusWaitingQuestion)
		}
		return false, err
	}
	if result != nil && result.Status == runtimemodel.MessageStatusCompleted && strings.TrimSpace(result.Answer) != "" {
		if err := s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, "question:"+pending.ID.String()+":"+requestID, result.Answer); err != nil {
			return false, err
		}
	}
	if held {
		s.settleChannelContinuationRun(ctx, run, result, pending.ID)
	}
	return true, nil
}

// continueChannelConnectorConfirmation confirms or cancels a pending operation
// in a connected system from an explicit channel reply.
func (s *RuntimeService) continueChannelConnectorConfirmation(ctx context.Context, channelScope Scope, digitalHumanID uuid.UUID, pending *runtimemodel.Message, run *Run, reply ChannelApprovalReply, action string) (bool, error) {
	notify := func(text string) (bool, error) {
		return true, s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, "connector-notice:"+uuid.NewString(), text)
	}
	if s.channelConnectors == nil || run.ActorID == nil || *run.ActorID == uuid.Nil || run.WaitingReceipt == nil {
		return notify(channelUnsupportedWaitNotice)
	}
	if channelApprovalInitiator(run) == "" && reply.ConversationType != channelruntime.ConversationTypeDirect {
		return notify(channelConnectorGroupNotice)
	}
	if action != "approve" && action != "decline" {
		return notify(channelConnectorReplyNotice)
	}
	decision, err := s.channelConnectors.DecideChannelConnectorConfirmation(ctx, run.OrganizationID, *run.ActorID, *run.WaitingReceipt, action == "approve")
	if err != nil {
		return false, err
	}
	text := channelConnectorOutcomeText(decision)
	// The decision resumed or ended the run; a completed operation ends it here.
	if decision.State == "completed" {
		if completeErr := s.store.CompleteRun(context.WithoutCancel(ctx), run.ID, text, nil, s.now()); completeErr != nil && !errors.Is(completeErr, ErrRunConflict) {
			logger.WarnContext(ctx, "failed to complete digital human channel connector run", "run_id", run.ID.String(), completeErr)
		}
	}
	return true, s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, "connector:"+run.WaitingReceipt.String(), text)
}

func channelConnectorOutcomeText(decision ChannelConnectorDecision) string {
	title := strings.TrimSpace(decision.Title)
	if title == "" {
		title = "这个操作"
	} else {
		title = "「" + title + "」"
	}
	switch decision.State {
	case "completed":
		if summary := strings.TrimSpace(decision.Summary); summary != "" {
			return "已确认，" + title + "已执行完成。\n\n" + summary
		}
		return "已确认，" + title + "已执行完成。"
	case "cancelled":
		return "已取消，" + title + "不会执行。"
	case "failed":
		return "已确认，但" + title + "执行失败，请到 ZGI 控制台的运行记录查看原因。"
	default:
		return "已确认，但" + title + "的执行结果暂时无法确定，请到 ZGI 控制台的运行记录查看。"
	}
}

// channelConversationContext reads a channel conversation as its
// DigitalHuman, under the installation's account.
func channelConversationContext(scope Scope, digitalHumanID uuid.UUID) (service.Scope, service.Caller) {
	workspaceID := scope.WorkspaceID
	callerID := digitalHumanID
	return service.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: &workspaceID, AccountID: scope.ActorID},
		service.Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, Source: digitalHumanChannelConversationSource}
}
