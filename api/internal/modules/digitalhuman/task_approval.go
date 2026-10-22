package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

type channelApprovalMessageLister interface {
	ListConversationMessagesByCaller(context.Context, service.Scope, service.Caller, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error)
}

type channelFinalDeliveryStore interface {
	EnqueueRunChannelFinalDelivery(context.Context, uuid.UUID, string, string, time.Time) error
}

type channelApprovalRunLookup interface {
	RunByMessageID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*Run, error)
}

// Channel approvals discover these capabilities by type assertion; pin them
// on the production implementations so a missing method fails the build
// instead of every real channel approval at runtime.
var (
	_ channelFinalDeliveryStore    = (*GormRepository)(nil)
	_ channelApprovalRunLookup     = (*GormRepository)(nil)
	_ channelApprovalMessageLister = service.Service(nil)
)

// ContinueTaskApproval resumes a paused DigitalHuman conversation after the
// approval form in the composer is submitted. No synthetic chat prompt is
// appended and all task mutations remain behind the same owner check used by
// task REST handlers.
func (s *RuntimeService) ContinueTaskApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID, messageID uuid.UUID,
	request service.DigitalHumanTaskApprovalRequest,
	onEvent func(service.StreamEvent) error,
) (*service.ChatResult, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	approvalRuntime, ok := s.chat.(digitalHumanTaskApprovalRuntime)
	if !ok {
		return nil, fmt.Errorf("digital human task approval runtime is unavailable")
	}
	if s.taskService == nil {
		return nil, fmt.Errorf("digital human task service is unavailable")
	}
	_ = runtime
	// A confirmed or declined proposal is already a terminal task-tool result.
	// Only the explicit revision path may expose the mutation tool again. This
	// prevents the Agent Loop from turning one approval into a second proposal
	// after it observes the newly applied task state.
	allowTaskMutationTools := strings.EqualFold(strings.TrimSpace(request.Action), "revise")
	continuationConfig, run, err := s.taskApprovalContinuationConfig(ctx, scope, digitalHumanID, messageID, allowTaskMutationTools)
	if err != nil {
		return nil, err
	}
	if run != nil {
		// The resumed Agent loop is owned by this run, exactly like the other
		// DigitalHuman continuations. Run-scoped capabilities such as delegated
		// Connector access require it; without it the follow-up answer fails.
		caller.RunID = cloneUUIDPointer(&run.ID)
	}
	// A run the approval held resumes now and records where the continuation
	// ends, so the run log and API clients following the run see it finish.
	held := run != nil && run.Status == RunStatusWaiting && run.WaitingApprovalOrigin == taskApprovalOrigin
	if held {
		if err := s.resumeHeldWorkflowRun(ctx, run); err != nil {
			return nil, err
		}
	}
	result, err := approvalRuntime.RunDigitalHumanTaskApprovalContinuation(
		ctx,
		runtimeScope,
		caller,
		continuationConfig,
		conversationID,
		messageID,
		request,
		s.applyTaskApproval(scope, digitalHumanID),
		onEvent,
	)
	if held {
		switch {
		case err != nil:
			// A rejected answer leaves the approval waiting; hold the run again
			// unless the message moved on.
			if waiter, ok := s.store.(runWaitingStore); !ok ||
				waiter.MarkRunWaiting(context.WithoutCancel(ctx), run.ID, runtimemodel.MessageStatusWaitingApproval, s.now()) != nil {
				s.settleWorkflowContinuationRun(ctx, run, "", "", nil)
			}
		case result != nil:
			s.settleWorkflowContinuationRun(ctx, run, result.Status, result.Answer, result.Metadata)
		}
	}
	return result, err
}

// taskApprovalContinuationConfig restores the immutable execution snapshot and
// the subject-scoped task tools for an approval continuation. The initial run
// receives these tools from TaskRuntimeProvider during Execute, but closures
// cannot be serialized into the pending approval metadata. Rebuilding them
// here keeps a revise action in the same Agent Loop instead of handing the
// model a continuation that has no way to call manage_digital_human_task.
func (s *RuntimeService) taskApprovalContinuationConfig(
	ctx context.Context,
	scope Scope,
	digitalHumanID, messageID uuid.UUID,
	allowTaskMutationTools bool,
) (service.RunConfig, *Run, error) {
	lookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return service.RunConfig{}, nil, nil
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, messageID)
	if err != nil {
		return service.RunConfig{}, nil, err
	}
	config, err := s.taskApprovalContinuationConfigForRun(ctx, run, allowTaskMutationTools)
	if err != nil {
		return service.RunConfig{}, nil, err
	}
	return config, run, nil
}

func (s *RuntimeService) taskApprovalContinuationConfigForRun(ctx context.Context, run *Run, allowTaskMutationTools bool) (service.RunConfig, error) {
	if run == nil {
		return service.RunConfig{}, ErrRunNotFound
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return service.RunConfig{}, err
	}
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		return service.RunConfig{}, err
	}
	s.attachAttachmentReader(&config, run, resolved, event, nil)
	if s.tasks == nil {
		return config, nil
	}
	preparation, err := s.tasks.Prepare(ctx, run, event)
	if err != nil {
		return service.RunConfig{}, err
	}
	if preparation == nil {
		return config, nil
	}
	if addendum := strings.TrimSpace(preparation.SystemPromptAddendum); addendum != "" {
		if strings.TrimSpace(config.SystemPrompt) != "" {
			config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + addendum
		} else {
			config.SystemPrompt = addendum
		}
	}
	if allowTaskMutationTools {
		config.RuntimeTools = append(config.RuntimeTools, preparation.Tools...)
	} else {
		// Keep read-only task context available for the final answer, but remove
		// the mutation tool. The authoritative mutation has already been applied
		// above; removing this tool makes an accidental second approval impossible
		// even if the model ignores the continuation instruction.
		for _, tool := range preparation.Tools {
			if strings.EqualFold(strings.TrimSpace(tool.Definition.Function.Name), taskManageToolName) {
				continue
			}
			config.RuntimeTools = append(config.RuntimeTools, tool)
		}
	}
	return config, nil
}

// ContinueChannelTaskApproval consumes an explicit confirmation message from
// a channel conversation. Channels do not have a composer that can be
// replaced with the approval form, so the user's short confirmation is routed
// to the same continuation and task mutation path instead of starting a new
// model turn.
// HasPendingChannelApproval reports whether the channel conversation is
// waiting on an approval. It is the cheap admission check run inside the
// provider's acknowledgement window; ContinueChannelTaskApproval does the work.
func (s *RuntimeService) HasPendingChannelApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID uuid.UUID,
) (bool, error) {
	pending, _, err := s.pendingChannelApproval(ctx, scope, digitalHumanID, conversationID)
	return pending != nil, err
}

// ChannelApprovalReply is the channel message answering a pending approval,
// with the sender identity needed to decide whether it may approve.
type ChannelApprovalReply struct {
	Text             string
	SenderExternalID string
	ConversationType channelruntime.ConversationType
}

func (s *RuntimeService) ContinueChannelTaskApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID uuid.UUID,
	reply ChannelApprovalReply,
) (bool, error) {
	pending, pendingKind, err := s.pendingChannelApproval(ctx, scope, digitalHumanID, conversationID)
	if err != nil || pending == nil {
		return false, err
	}
	text := reply.Text
	// A group conversation is shared: only the member who started the run
	// that is now waiting may answer its approval. Anyone else is told to wait
	// and nothing is submitted.
	if notice, err := s.channelApprovalSenderNotice(ctx, scope, digitalHumanID, pending, reply); err != nil {
		return false, err
	} else if notice != "" {
		if err := s.deliverChannelWorkflowApprovalMessage(ctx, scope, digitalHumanID, pending, "approval-sender:"+uuid.NewString(), notice); err != nil {
			return false, err
		}
		return true, nil
	}
	workspaceID := scope.WorkspaceID
	callerID := digitalHumanID
	caller := service.Caller{
		Type:   runtimemodel.ConversationCallerDigitalHuman,
		ID:     &callerID,
		Source: digitalHumanChannelConversationSource,
	}
	runtimeScope := service.Scope{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    &workspaceID,
		AccountID:      scope.ActorID,
	}
	action, feedback := channelTaskApprovalAction(text)
	if action == "" {
		// Channel conversations have no approval form to replace the composer.
		// Once a task proposal is pending, any non-explicit confirmation is
		// treated as the user's requested revision and sent back to the Agent
		// Loop instead of being interpreted as an implicit approval.
		action = "revise"
		feedback = strings.TrimSpace(text)
		if feedback == "" {
			return false, nil
		}
	}
	channelScope := scope
	channelScope.Surface = SurfaceChannel
	channelScope.PrincipalType = string(invocation.PrincipalAccount)
	channelScope.PrincipalID = scope.ActorID
	if pendingKind != "digital_human_task" {
		return s.continueChannelWorkflowApproval(ctx, channelScope, digitalHumanID, conversationID, pending, action, feedback)
	}
	approvalRuntime, ok := s.chat.(digitalHumanTaskApprovalRuntime)
	if !ok || s.taskService == nil {
		return false, fmt.Errorf("digital human channel task approval runtime is unavailable")
	}
	runLookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return false, fmt.Errorf("digital human channel task approval run lookup is unavailable")
	}
	pendingRun, err := runLookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, pending.ID)
	if err != nil {
		return false, err
	}
	if pendingRun == nil {
		return false, ErrRunNotFound
	}
	allowTaskMutationTools := action == "revise"
	continuationConfig, err := s.taskApprovalContinuationConfigForRun(ctx, pendingRun, allowTaskMutationTools)
	if err != nil {
		return false, err
	}
	caller.RunID = cloneUUIDPointer(&pendingRun.ID)
	result, err := approvalRuntime.RunDigitalHumanTaskApprovalContinuation(
		ctx,
		runtimeScope,
		caller,
		continuationConfig,
		conversationID,
		pending.ID,
		service.DigitalHumanTaskApprovalRequest{
			ApprovalToken: stringValue(workflowContinuationRecord(pending.Metadata)["approval_token"]),
			Action:        action,
			Inputs:        map[string]interface{}{"supplement": feedback},
		},
		s.applyTaskApproval(channelScope, digitalHumanID),
		nil,
	)
	if err != nil {
		return false, err
	}
	if result != nil && strings.TrimSpace(result.Answer) != "" {
		deliveryStore, ok := s.store.(channelFinalDeliveryStore)
		if !ok {
			return false, fmt.Errorf("digital human channel approval delivery is unavailable")
		}
		state := workflowContinuationRecord(pending.Metadata)
		key := "approval:" + strings.TrimSpace(stringValue(state["workflow_run_id"]))
		if len(key) > 128 {
			key = "approval:" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
		}
		if err := deliveryStore.EnqueueRunChannelFinalDelivery(ctx, pendingRun.ID, key, result.Answer, time.Now().UTC()); err != nil {
			return false, err
		}
	}
	return true, nil
}

// channelApprovalOtherSenderNotice tells a group member that the pending
// approval belongs to the member who started the request.
const channelApprovalOtherSenderNotice = "这个审批需要由发起请求的成员确认，其他成员无法代为审批。请等待发起人处理。"

// channelApprovalSenderNotice returns a notice when the reply's sender may not
// answer the pending approval, or "" when it may. The initiator is the
// external user recorded on the run that produced the pending message; a run
// without one (for example, a scheduled run) may be answered by any member.
func (s *RuntimeService) channelApprovalSenderNotice(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	pending *runtimemodel.Message,
	reply ChannelApprovalReply,
) (string, error) {
	runLookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return "", fmt.Errorf("digital human channel approval run lookup is unavailable")
	}
	run, err := runLookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, pending.ID)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", ErrRunNotFound
	}
	initiator := channelApprovalInitiator(run)
	if initiator == "" || strings.TrimSpace(reply.SenderExternalID) == initiator {
		return "", nil
	}
	return channelApprovalOtherSenderNotice, nil
}

func channelApprovalInitiator(run *Run) string {
	if run == nil {
		return ""
	}
	var event invocation.Event
	if json.Unmarshal(run.InvocationEvent, &event) != nil || event.Sender.Type != invocation.PrincipalExternalUser {
		return ""
	}
	return strings.TrimSpace(event.Sender.ID)
}

// channelWorkflowApprovalWaitingNotice tells a channel user that the pending
// workflow approval is assigned to designated approvers, not to them.
const channelWorkflowApprovalWaitingNotice = "这一步需要由指定的审批人通过审批通知（如邮件或短信）完成，当前对话无法直接审批。审批完成后会自动继续执行，请耐心等待。"

// continueChannelWorkflowApproval resumes a generic workflow approval from a
// channel. Only an approval the workflow lets the conversation user complete
// (in-conversation approval, exposed as ui_approval_allowed with a token) is
// submitted; an approval assigned to designated approvers is left untouched
// and the user is told to wait for it.
func (s *RuntimeService) continueChannelWorkflowApproval(
	ctx context.Context,
	channelScope Scope,
	digitalHumanID, conversationID uuid.UUID,
	pending *runtimemodel.Message,
	action, feedback string,
) (bool, error) {
	if !s.WorkflowContinuationEnabled() {
		return false, fmt.Errorf("digital human channel workflow continuation is unavailable")
	}
	state := workflowContinuationRecord(pending.Metadata)
	token := strings.TrimSpace(stringValue(state["approval_token"]))
	approvedByCurrentUser, _ := state["ui_approval_allowed"].(bool)
	if !approvedByCurrentUser || token == "" {
		if err := s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, "approval-wait:"+uuid.NewString(), channelWorkflowApprovalWaitingNotice); err != nil {
			return false, err
		}
		return true, nil
	}
	if action == "revise" {
		return false, fmt.Errorf("workflow approval does not support revision from channel")
	}
	request := service.DigitalHumanTaskApprovalRequest{
		Type: "approval", ApprovalToken: token,
		Action: action, Inputs: map[string]interface{}{"supplement": feedback},
	}
	// The channel scope makes the continuation resolve the channel-owned
	// conversation and identifies the explicit channel confirmation.
	answer, err := s.continueWorkflowApproval(ctx, channelScope, digitalHumanID, conversationID, pending.ID, request, nil)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(answer) == "" {
		return true, nil
	}
	key := "approval:" + strings.TrimSpace(stringValue(state["workflow_run_id"]))
	if err := s.deliverChannelWorkflowApprovalMessage(ctx, channelScope, digitalHumanID, pending, key, answer); err != nil {
		return false, err
	}
	return true, nil
}

func (s *RuntimeService) deliverChannelWorkflowApprovalMessage(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, pending *runtimemodel.Message, key, text string) error {
	runLookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return fmt.Errorf("digital human channel approval run lookup is unavailable")
	}
	deliveryStore, ok := s.store.(channelFinalDeliveryStore)
	if !ok {
		return fmt.Errorf("digital human channel approval delivery is unavailable")
	}
	run, err := runLookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, pending.ID)
	if err != nil {
		return err
	}
	if run == nil {
		return ErrRunNotFound
	}
	if len(key) > 128 {
		key = "approval:" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
	}
	return deliveryStore.EnqueueRunChannelFinalDelivery(ctx, run.ID, key, text, time.Now().UTC())
}

func (s *RuntimeService) pendingChannelApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID uuid.UUID,
) (*runtimemodel.Message, string, error) {
	if s == nil || digitalHumanID == uuid.Nil || conversationID == uuid.Nil {
		return nil, "", ErrInvalidRequest
	}
	lister, ok := s.chat.(channelApprovalMessageLister)
	if !ok {
		return nil, "", nil
	}
	workspaceID := scope.WorkspaceID
	callerID := digitalHumanID
	caller := service.Caller{
		Type:   runtimemodel.ConversationCallerDigitalHuman,
		ID:     &callerID,
		Source: digitalHumanChannelConversationSource,
	}
	runtimeScope := service.Scope{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    &workspaceID,
		AccountID:      scope.ActorID,
	}
	messages, _, err := lister.ListConversationMessagesByCaller(ctx, runtimeScope, caller, conversationID, 1, 50)
	if err != nil {
		// A channel mapping reserves its local conversation ID before the
		// first runtime input is submitted. Until that input creates the
		// conversation, the approval lookup naturally returns not found.
		// Treat that state as "no pending approval" and let the normal
		// channel invocation path create the conversation. Other errors
		// (including permission and storage failures) must still fail closed.
		if errors.Is(err, service.ErrNotFound) {
			return nil, "", nil
		}
		return nil, "", err
	}
	for _, message := range messages {
		if message == nil || message.Status != runtimemodel.MessageStatusWaitingApproval {
			continue
		}
		state := workflowContinuationRecord(message.Metadata)
		kind := strings.TrimSpace(stringValue(state["kind"]))
		if kind == "" {
			// Only DigitalHuman task approvals persist an explicit kind; a
			// generic workflow approval is identified by its workflow run.
			if strings.TrimSpace(stringValue(state["workflow_run_id"])) == "" {
				continue
			}
			kind = "workflow"
		}
		return message, kind, nil
	}
	return nil, "", nil
}

func channelTaskApprovalAction(text string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(text))
	value = strings.Trim(value, " \t\r\n。！？!?，,；;：:")
	switch value {
	case "确认", "确认并执行", "同意", "执行", "好的", "好", "是", "ok", "yes", "approve":
		return "approve", ""
	case "取消", "拒绝", "不同意", "不要", "否", "no", "reject", "decline":
		return "decline", ""
	default:
		return "", ""
	}
}

func (s *RuntimeService) applyTaskApproval(scope Scope, digitalHumanID uuid.UUID) service.DigitalHumanTaskApprovalHandler {
	return func(ctx context.Context, continuation *service.WorkflowApprovalContinuation, action string, inputs map[string]interface{}) (string, error) {
		if continuation == nil {
			return "", ErrInvalidRequest
		}
		state := workflowContinuationRecord(continuation.Metadata)
		taskID, err := uuid.Parse(strings.TrimSpace(stringValue(state["task_id"])))
		if err != nil || taskID == uuid.Nil {
			return "", ErrInvalidRequest
		}
		ownerKind := TaskOwnerKind(strings.TrimSpace(stringValue(state["owner_kind"])))
		var owner TaskOwnerRef
		if ownerKind == TaskOwnerConversation {
			ownerConversationID, parseErr := uuid.Parse(strings.TrimSpace(stringValue(state["owner_conversation_id"])))
			if parseErr != nil || ownerConversationID == uuid.Nil {
				return "", ErrTaskPolicyRejected
			}
			owner = ConversationTaskOwner(ownerConversationID)
		} else {
			subjectID := scope.UserSubjectID
			subjectKind := usersubject.Kind(strings.TrimSpace(scope.UserSubjectKind))
			if subjectID == nil || *subjectID == uuid.Nil {
				if scope.Surface != SurfaceConsole || scope.ActorID == uuid.Nil {
					return "", ErrTaskPolicyRejected
				}
				id := scope.ActorID
				subjectID = &id
				subjectKind = usersubject.Account
			}
			if subjectKind != usersubject.Account && subjectKind != usersubject.APIEndUser {
				return "", ErrTaskPolicyRejected
			}
			owner = UserTaskOwner(usersubject.Ref{Kind: subjectKind, ID: *subjectID})
		}
		principalType := invocation.PrincipalType(strings.TrimSpace(scope.PrincipalType))
		if principalType == "" {
			principalType = invocation.PrincipalAccount
		}
		principalID := scope.PrincipalID
		if principalID == uuid.Nil {
			principalID = scope.ActorID
		}
		actorType := taskApprovalActorType(scope)
		access := TaskAccessContext{
			OrganizationID: scope.OrganizationID,
			WorkspaceID:    scope.WorkspaceID,
			DigitalHumanID: digitalHumanID,
			Principal:      invocation.PrincipalRef{Type: principalType, ID: principalID.String()},
			Owner:          owner,
			Surface:        scope.Surface,
		}
		task, err := s.taskService.GetTask(ctx, access, taskID)
		if err != nil {
			return "", err
		}
		// The approval form is a decision surface, not a second mutation
		// editor. Confirmation applies the model-owned proposal captured in
		// requested_inputs; the only user-editable value is handled by the
		// separate "revise" action and never enters this mutation path.
		submittedInputs := inputs
		if action == "approve" {
			submittedInputs = nil
		}
		inputs = mergeTaskApprovalInputs(state, submittedInputs)
		operationID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("digital-human-task-approval:"+continuation.WorkflowRunID+":"+action))
		mutation := TaskMutationContext{
			ActorType: actorType, ActorRef: principalID.String(), OperationID: operationID,
			SourceRunID: nil, Source: scope.Surface, SourceConversationID: continuation.ConversationID.String(),
			Proposed: false,
		}
		// Channel approvals resume the task mutation after the original model
		// turn has ended. Rehydrate the immutable reply target from the task so
		// confirming or editing a channel task cannot silently fall back to the
		// task center. The raw address is trusted data captured at creation time,
		// never a value supplied by the approval form.
		if task.DeliveryMode == TaskDeliverySourceConversation && task.DeliveryReplyTo != nil {
			reply := *task.DeliveryReplyTo
			mutation.ReplyTo = &reply
		}
		if action == "revise" {
			// A rejected create proposal should not remain as an active proposed
			// task while the Agent generates its replacement. Existing-task
			// revisions leave the current task untouched.
			if task.WorkStatus == TaskWorkStatusProposed {
				if _, err := s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionDecline, ApplyTaskActionRequest{ExpectedRevision: task.Revision}, mutation); err != nil {
					return "", err
				}
				return "上一版任务提议已作废。", nil
			}
			return "上一版任务变更未生效。", nil
		}
		if action == "decline" {
			// Declining a proposal is a task lifecycle mutation. Declining an
			// approval for an existing task leaves that task unchanged.
			if task.WorkStatus == TaskWorkStatusProposed {
				updated, err := s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionDecline, ApplyTaskActionRequest{ExpectedRevision: task.Revision}, mutation)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("任务“%s”已取消。", updated.Name), nil
			}
			return "已取消这次任务操作。", nil
		}
		expectedRevision := int64Value(state["task_revision"])
		if expectedRevision < 1 {
			expectedRevision = task.Revision
		}
		taskAction := strings.ToLower(strings.TrimSpace(stringValue(state["task_action"])))
		// Older approvals could encode a resume request as set_status=pending
		// while the task was paused. Preserve their intent when the approval is
		// finally submitted instead of leaving the lifecycle paused.
		if taskAction == "set_status" && task.LifecycleStatus == "paused" {
			status := TaskWorkStatus(strings.TrimSpace(stringValue(inputs["work_status"])))
			if status == "" {
				status = TaskWorkStatus(strings.TrimSpace(stringValue(inputs["status"])))
			}
			if status == TaskWorkStatusPending || status == TaskWorkStatusInProgress {
				taskAction = "resume"
			}
		}
		var updated *Task
		switch taskAction {
		case "create":
			// Creation already persisted a proposal so the approval can be
			// retried safely. The proposal values come from the model-owned
			// requested_inputs. User feedback is handled by the separate
			// "revise" approval action and must never be appended directly to the
			// task goal.
			updateRequest := UpdateTaskRequest{ExpectedRevision: expectedRevision}
			if name := strings.TrimSpace(stringValue(inputs["name"])); name != "" && name != task.Name {
				updateRequest.Name = &name
			}
			if goal := strings.TrimSpace(stringValue(inputs["goal"])); goal != "" && goal != task.Goal {
				updateRequest.Goal = &goal
			}
			if updateRequest.Name != nil || updateRequest.Goal != nil {
				editMutation := mutation
				editMutation.OperationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("digital-human-task-approval-edit:"+continuation.WorkflowRunID))
				updated, err = s.taskService.UpdateTask(ctx, access, task.ID, updateRequest, editMutation)
				if err != nil {
					return "", err
				}
				task = updated
				expectedRevision = task.Revision
			}
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionConfirm, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "confirm":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionConfirm, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "decline":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionDecline, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "cancel":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionCancel, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "run_now":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionRunNow, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "pause":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionPause, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "resume":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionResume, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "retry":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionRetry, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "archive":
			updated, err = s.taskService.ApplyTaskAction(ctx, access, task.ID, TaskActionArchive, ApplyTaskActionRequest{ExpectedRevision: expectedRevision}, mutation)
		case "set_status":
			status := TaskWorkStatus(strings.TrimSpace(stringValue(inputs["work_status"])))
			if status == "" {
				status = TaskWorkStatus(strings.TrimSpace(stringValue(inputs["status"])))
			}
			if status == "" {
				status = task.WorkStatus
			}
			updated, err = s.taskService.SetTaskWorkStatus(ctx, access, task.ID, status, expectedRevision, mutation)
		case "schedule":
			schedule, ok := taskScheduleFromAny(inputs["schedule"])
			if !ok {
				schedule = task.Schedule
			}
			request := UpdateTaskRequest{ExpectedRevision: expectedRevision, Schedule: &schedule}
			if lifecycle := strings.TrimSpace(stringValue(inputs["lifecycle_status"])); lifecycle != "" {
				request.LifecycleStatus = &lifecycle
			}
			updated, err = s.taskService.UpdateTask(ctx, access, task.ID, request, mutation)
			if err == nil {
				updated, err = applyApprovedLifecycleTransition(ctx, s.taskService, access, updated, inputs, mutation)
			}
		case "update":
			request := taskUpdateRequestFromApprovalInputs(expectedRevision, inputs)
			updated, err = s.taskService.UpdateTask(ctx, access, task.ID, request, mutation)
			if err == nil {
				updated, err = applyApprovedLifecycleTransition(ctx, s.taskService, access, updated, inputs, mutation)
			}
		default:
			return "", ErrInvalidRequest
		}
		if err != nil {
			return "", err
		}
		if updated == nil {
			updated = task
		}
		return fmt.Sprintf("任务“%s”已%s。", updated.Name, taskApprovalActionLabel(taskAction)), nil
	}
}

func applyApprovedLifecycleTransition(
	ctx context.Context,
	tasks TaskHandlerService,
	access TaskAccessContext,
	task *Task,
	inputs map[string]interface{},
	mutation TaskMutationContext,
) (*Task, error) {
	if task == nil || tasks == nil {
		return task, nil
	}
	if !strings.EqualFold(strings.TrimSpace(stringValue(inputs["lifecycle_status"])), "active") || task.LifecycleStatus != "paused" {
		return task, nil
	}
	resumeMutation := mutation
	resumeMutation.OperationID = uuid.NewSHA1(mutation.OperationID, []byte("resume"))
	return tasks.ApplyTaskAction(ctx, access, task.ID, TaskActionResume, ApplyTaskActionRequest{ExpectedRevision: task.Revision}, resumeMutation)
}

// taskApprovalActorType maps the trusted invocation surface to the activity
// actor enum accepted by automation_task_activities. Keep this mapping local
// to the approval path so a missing/legacy surface can never write an invalid
// actor type and roll back an otherwise valid task confirmation.
func taskApprovalActorType(scope Scope) string {
	if scope.Surface == SurfaceConsole || scope.PrincipalType == string(invocation.PrincipalAccount) {
		return "account"
	}
	return "external"
}

func workflowContinuationRecord(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return map[string]interface{}{}
	}
	if value, ok := metadata["agent_workflow_continuation"].(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func mergeTaskApprovalInputs(state map[string]interface{}, submitted map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	if requested, ok := state["requested_inputs"].(map[string]interface{}); ok {
		for key, value := range requested {
			merged[key] = value
		}
	}
	for key, value := range submitted {
		merged[key] = value
	}
	return merged
}

func taskScheduleFromAny(value interface{}) (TaskSchedule, bool) {
	switch typed := value.(type) {
	case TaskSchedule:
		return typed, true
	case *TaskSchedule:
		if typed != nil {
			return *typed, true
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == "null" {
		return TaskSchedule{}, false
	}
	var schedule TaskSchedule
	if err := json.Unmarshal(encoded, &schedule); err != nil || schedule.Type == "" {
		return TaskSchedule{}, false
	}
	return schedule, true
}

func taskUpdateRequestFromApprovalInputs(expectedRevision int64, inputs map[string]interface{}) UpdateTaskRequest {
	request := UpdateTaskRequest{ExpectedRevision: expectedRevision}
	if name := strings.TrimSpace(stringValue(inputs["name"])); name != "" {
		request.Name = &name
	}
	if goal := strings.TrimSpace(stringValue(inputs["goal"])); goal != "" {
		request.Goal = &goal
	}
	if values := stringSliceFromAny(inputs["success_criteria"]); values != nil {
		request.SuccessCriteria = &values
	}
	if priority := TaskPriority(strings.TrimSpace(stringValue(inputs["priority"]))); priority != "" {
		request.Priority = &priority
	}
	if schedule, ok := taskScheduleFromAny(inputs["schedule"]); ok {
		request.Schedule = &schedule
	}
	if delivery := TaskDeliveryMode(strings.TrimSpace(stringValue(inputs["delivery_mode"]))); delivery != "" {
		request.DeliveryMode = &delivery
	}
	if lifecycle := strings.TrimSpace(stringValue(inputs["lifecycle_status"])); lifecycle != "" {
		request.LifecycleStatus = &lifecycle
	}
	if timeout := int64Value(inputs["execution_timeout_seconds"]); timeout > 0 {
		value := int(timeout)
		request.ExecutionTimeoutSeconds = &value
	}
	if attempts := int64Value(inputs["max_attempts"]); attempts > 0 {
		value := int(attempts)
		request.MaxAttempts = &value
	}
	return request
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if raw, ok := value.(string); ok {
		return raw
	}
	return fmt.Sprint(value)
}

func int64Value(value interface{}) int64 {
	switch raw := value.(type) {
	case int64:
		return raw
	case int:
		return int64(raw)
	case float64:
		return int64(raw)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		return parsed
	default:
		return 0
	}
}

func taskApprovalActionLabel(action string) string {
	switch action {
	case "create", "confirm":
		return "创建并生效"
	case "update", "schedule":
		return "更新"
	case "cancel":
		return "取消"
	case "run_now":
		return "立即执行"
	case "pause":
		return "暂停"
	case "resume":
		return "恢复"
	case "retry":
		return "重试"
	case "archive":
		return "归档"
	case "set_status":
		return "更新状态"
	case "decline":
		return "拒绝"
	default:
		return "处理完成"
	}
}
