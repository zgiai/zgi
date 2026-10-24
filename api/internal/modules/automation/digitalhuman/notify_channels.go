package digitalhumantask

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// notifyChannelsKey stores a task's team channels in its delivery config.
const notifyChannelsKey = "notify_channel_ids"

// SetNotifyChannels lets tasks report to team notification channels. Without
// it a task cannot choose any.
func (s *Service) SetNotifyChannels(channels digitalhuman.TaskNotifyChannels) {
	s.notifyChannels = channels
}

// taskNotifyChannels lists the team channels a task reports to. A task that
// is no longer visible to its workspace reports to none of them.
func taskNotifyChannels(row *automationmodel.AutomationTask) []uuid.UUID {
	if row == nil || !teamReportableTask(row) || storedTaskVisibility(row) != digitalhuman.TaskVisibilityWorkspace {
		return nil
	}
	return configNotifyChannels(row.DeliveryConfig)
}

func configNotifyChannels(config map[string]any) []uuid.UUID {
	raw, _ := config[notifyChannelsKey].([]any)
	ids := make([]uuid.UUID, 0, len(raw))
	for _, value := range raw {
		if id, err := uuid.Parse(fmt.Sprint(value)); err == nil && id != uuid.Nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// withNotifyChannels returns config with ids as its team channels.
func withNotifyChannels(config map[string]any, ids []uuid.UUID) map[string]any {
	next := make(map[string]any, len(config)+1)
	for key, value := range config {
		next[key] = value
	}
	delete(next, notifyChannelsKey)
	if len(ids) > 0 {
		values := make([]any, len(ids))
		for i, id := range ids {
			values[i] = id.String()
		}
		next[notifyChannelsKey] = values
	}
	return next
}

func normalizeNotifyChannels(ids []uuid.UUID) ([]uuid.UUID, error) {
	var result []uuid.UUID
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, digitalhuman.ErrInvalidRequest
		}
		if !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	if len(result) > digitalhuman.MaxTaskNotifyChannels {
		return nil, digitalhuman.ErrInvalidRequest
	}
	return result, nil
}

// checkNotifyChannels confirms ids are active channels of the workspace.
func (s *Service) checkNotifyChannels(ctx context.Context, organizationID, workspaceID string, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if s.notifyChannels == nil {
		return digitalhuman.ErrInvalidRequest
	}
	organization, orgErr := uuid.Parse(organizationID)
	workspace, workspaceErr := uuid.Parse(workspaceID)
	if orgErr != nil || workspaceErr != nil {
		return digitalhuman.ErrInvalidRequest
	}
	ok, err := s.notifyChannels.ActiveWorkspaceChannels(ctx, organization, workspace, ids)
	if err != nil {
		return fmt.Errorf("check task notification channels: %w", err)
	}
	if !ok {
		return digitalhuman.ErrInvalidRequest
	}
	return nil
}

// notifyChannelAccess says how access may change row's team channels. Only
// a workspace member's task that the workspace can see reports to team
// channels. Its requester chooses them; a task center manager may remove
// them but never point someone else's reports somewhere new.
func notifyChannelAccess(row *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) digitalhuman.TaskNotifyChannelAccess {
	if access.CurrentTaskID != nil || !teamReportableTask(row) || storedTaskVisibility(row) != digitalhuman.TaskVisibilityWorkspace {
		return ""
	}
	if access.Surface == digitalhuman.SurfaceConsole && taskRequestedBy(row, access) {
		return digitalhuman.TaskNotifyChannelsManage
	}
	if taskCenterManager(access) {
		return digitalhuman.TaskNotifyChannelsRemove
	}
	return ""
}

// teamReportableTask reports whether row has a workspace member as its
// requester: a task center task, or a member's own task. External end users'
// and channel conversations' tasks never report to team channels.
func teamReportableTask(row *automationmodel.AutomationTask) bool {
	switch digitalhuman.TaskOwnerKind(pointerValue(row.OwnerKind)) {
	case digitalhuman.TaskOwnerDigitalHuman:
		return true
	case digitalhuman.TaskOwnerUserSubject:
		return pointerValue(row.OwnerSubjectKind) == string(usersubject.Account)
	default:
		return false
	}
}

// changeNotifyChannels applies a requested set of team channels to task,
// whose visibility already reflects the same update.
func (s *Service) changeNotifyChannels(ctx context.Context, task *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext, current, requested []uuid.UUID) ([]uuid.UUID, error) {
	ids, err := normalizeNotifyChannels(requested)
	if err != nil {
		return nil, err
	}
	var added []uuid.UUID
	for _, id := range ids {
		if !slices.Contains(current, id) {
			added = append(added, id)
		}
	}
	if len(added) == 0 && len(ids) == len(current) {
		return ids, nil
	}
	switch notifyChannelAccess(task, access) {
	case digitalhuman.TaskNotifyChannelsManage:
		return ids, s.checkNotifyChannels(ctx, task.OrganizationID, task.WorkspaceID, added)
	case digitalhuman.TaskNotifyChannelsRemove:
		if len(added) > 0 {
			return nil, digitalhuman.ErrTaskVisibilityForbidden
		}
		return ids, nil
	default:
		return nil, digitalhuman.ErrTaskVisibilityForbidden
	}
}

// reportReviewKey stores which of a task's reports wait for review in its
// delivery config; the default is not stored.
const reportReviewKey = "report_review"

// taskReportReview is a task's report review, the default when unset.
func taskReportReview(row *automationmodel.AutomationTask) digitalhuman.TaskReportReview {
	if row != nil {
		switch review := digitalhuman.TaskReportReview(fmt.Sprint(row.DeliveryConfig[reportReviewKey])); review {
		case digitalhuman.TaskReportReviewNone, digitalhuman.TaskReportReviewRequired:
			return review
		}
	}
	return digitalhuman.TaskReportReviewExceptions
}

// withReportReview returns config with review, removed for the default.
func withReportReview(config map[string]any, review digitalhuman.TaskReportReview) map[string]any {
	next := make(map[string]any, len(config)+1)
	for key, value := range config {
		next[key] = value
	}
	delete(next, reportReviewKey)
	if review == digitalhuman.TaskReportReviewNone || review == digitalhuman.TaskReportReviewRequired {
		next[reportReviewKey] = string(review)
	}
	return next
}

// reportDigestKey stores a scheduled task's report digest in its delivery
// config.
const reportDigestKey = "report_digest"

// taskReportDigest is a task's digest; only a scheduled (cron) task has one.
func taskReportDigest(row *automationmodel.AutomationTask) digitalhuman.TaskReportDigest {
	if row == nil || row.ScheduleType != automationmodel.AutomationScheduleTypeCron {
		return digitalhuman.TaskReportDigestNone
	}
	digest := digitalhuman.TaskReportDigest(fmt.Sprint(row.DeliveryConfig[reportDigestKey]))
	if digest == digitalhuman.TaskReportDigestDaily || digest == digitalhuman.TaskReportDigestWeekly {
		return digest
	}
	return digitalhuman.TaskReportDigestNone
}

// withReportDigest returns config with digest, removed when empty.
func withReportDigest(config map[string]any, digest digitalhuman.TaskReportDigest) map[string]any {
	next := make(map[string]any, len(config)+1)
	for key, value := range config {
		next[key] = value
	}
	delete(next, reportDigestKey)
	if digest != digitalhuman.TaskReportDigestNone {
		next[reportDigestKey] = string(digest)
	}
	return next
}
