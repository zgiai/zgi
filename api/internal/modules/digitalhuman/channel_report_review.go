package digitalhuman

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

// A task report delivered to a channel is reviewed by replying 采纳 or
// 返工：意见. Only these explicit forms are read as a review, so ordinary
// conversation is never mistaken for one.

const (
	channelReportReviewHint     = "回复「采纳」确认这次结果，或回复「返工：具体意见」让我重做。"
	channelReportReworkAskHint  = "请说明需要怎么改，例如「返工：补充北京的数据」。"
	channelReportReviewedReason = "channel.task_report.reviewed"
)

// channelReportReviewRuntime reviews the conversation's latest report from a
// channel reply. It reports false when the reply is not a review, or there is
// no report to review, so the reply is handled as an ordinary message.
type channelReportReviewRuntime interface {
	ReviewChannelTaskReport(ctx context.Context, scope Scope, digitalHumanID, ownerConversationID uuid.UUID, reply ChannelApprovalReply) (bool, error)
}

var _ channelReportReviewRuntime = (*RuntimeService)(nil)

// channelReportReviewAction reads an explicit review from a channel reply.
func channelReportReviewAction(text string) (TaskReportReviewAction, string, bool) {
	value := strings.TrimSpace(text)
	trimmed := strings.Trim(value, " \t\r\n。！!")
	switch trimmed {
	case "采纳", "通过", "接受":
		return TaskReportAccept, "", true
	case "返工", "重做":
		return TaskReportRework, "", true
	}
	for _, prefix := range []string{"返工：", "返工:", "返工 ", "重做：", "重做:", "重做 "} {
		if strings.HasPrefix(value, prefix) {
			return TaskReportRework, strings.TrimSpace(strings.TrimPrefix(value, prefix)), true
		}
	}
	return "", "", false
}

// ownerConversationID is the stable channel conversation its tasks belong to.
func (s *RuntimeService) ReviewChannelTaskReport(ctx context.Context, scope Scope, digitalHumanID, ownerConversationID uuid.UUID, reply ChannelApprovalReply) (bool, error) {
	conversationID := ownerConversationID
	action, feedback, ok := channelReportReviewAction(reply.Text)
	if !ok || s == nil || s.taskService == nil || conversationID == uuid.Nil {
		return false, nil
	}
	reports, ok := s.taskService.(TaskReportService)
	if !ok {
		return false, nil
	}
	access := TaskAccessContext{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		DigitalHumanID: digitalHumanID,
		Principal:      invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: scope.ActorID.String()},
		Owner:          ConversationTaskOwner(conversationID),
		Surface:        SurfaceChannel,
	}
	list, err := reports.ListTaskReports(ctx, access, TaskReportListQuery{ReviewStates: []TaskReportReviewState{TaskReportNew}, Page: 1, PageSize: 1})
	if err != nil || list == nil || len(list.Items) == 0 {
		// Without a report waiting for review, 采纳 is just a message.
		return false, err
	}
	report := list.Items[0]
	if report.DigitalHumanRunID == nil || *report.DigitalHumanRunID == uuid.Nil {
		return false, nil
	}
	deliver := func(key, text string) (bool, error) {
		deliveryStore, ok := s.store.(channelFinalDeliveryStore)
		if !ok {
			return false, nil
		}
		return true, deliveryStore.EnqueueRunChannelFinalDelivery(ctx, *report.DigitalHumanRunID, key, text, s.now())
	}
	if action == TaskReportRework && feedback == "" {
		return deliver("report-review-ask:"+uuid.NewString(), channelReportReworkAskHint)
	}
	operationID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("digital-human-channel-report-review:"+report.ID.String()+":"+string(action)))
	if _, err := reports.ReviewTaskReport(ctx, access, report.ID, ReviewTaskReportRequest{Action: action, Feedback: feedback}, TaskMutationContext{
		ActorType: "external", ActorRef: strings.TrimSpace(reply.SenderExternalID), OperationID: operationID,
		Source: SurfaceChannel, SourceConversationID: conversationID.String(), SourceConversationType: string(reply.ConversationType),
	}); err != nil {
		return false, err
	}
	name := strings.TrimSpace(report.TaskName)
	if name != "" {
		name = "「" + name + "」"
	}
	if action == TaskReportAccept {
		return deliver("report-review:"+report.ID.String(), "已采纳"+name+"的这次结果。")
	}
	return deliver("report-review:"+report.ID.String()+":rework", "收到，已按你的意见安排"+name+"重新执行，完成后会再发给你。")
}
