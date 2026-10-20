package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/gorm"
)

// AuthorizeBackgroundLinuxExecution rechecks the live publication and the
// provider route or confirmed task before each Shell, export or CLI operation.
// It deliberately does not treat a creator or scheduler as an end user.
func (s *AccessService) AuthorizeBackgroundLinuxExecution(ctx context.Context, run *Run, event invocation.Event) error {
	return s.authorizeBackgroundExecution(ctx, run, event, true)
}

// AuthorizeBackgroundConnectorExecution rechecks the same live published
// employee and task/channel authority as background Linux execution without
// requiring the publication to opt into a Linux sandbox.
func (s *AccessService) AuthorizeBackgroundConnectorExecution(ctx context.Context, run *Run, event invocation.Event) error {
	return s.authorizeBackgroundExecution(ctx, run, event, false)
}

func (s *AccessService) authorizeBackgroundExecution(ctx context.Context, run *Run, event invocation.Event, requireLinux bool) error {
	if s == nil || s.db == nil || run == nil || (run.Surface != SurfaceChannel && run.Surface != SurfaceTask) {
		return ErrNotFound
	}
	access, err := s.publishedAccess(ctx, run.DigitalHumanID, run.OrganizationID, run.WorkspaceID)
	if err != nil {
		return err
	}
	if access.Version == nil || access.Version.ID != run.DigitalHumanVersionID {
		return ErrPublishInvalid
	}
	if requireLinux && (access.Version.ExecutionRuntime == nil || !access.Version.ExecutionRuntime.Enabled) {
		return ErrPublishInvalid
	}
	var frozen Version
	if err := s.db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND state = ?",
		run.DigitalHumanVersionID, run.DigitalHumanID, run.OrganizationID, run.WorkspaceID, VersionStatePublished).Take(&frozen).Error; err != nil {
		return backgroundRecordError(err)
	}
	if requireLinux && (frozen.ExecutionRuntime == nil || !frozen.ExecutionRuntime.Enabled) {
		return ErrPublishInvalid
	}
	if run.Surface == SurfaceChannel {
		if event.ReplyTo == nil || run.ConversationID == nil {
			return ErrNotFound
		}
		binding, ok := stringContextValue(event.Context, "channel_binding_id")
		if !ok {
			return ErrNotFound
		}
		return s.authorizeBackgroundChannel(ctx, run, event, binding, run.ConversationID)
	}
	return s.authorizeBackgroundTask(ctx, run, event)
}

func (s *AccessService) authorizeBackgroundChannel(ctx context.Context, run *Run, event invocation.Event, binding string, conversation *uuid.UUID) error {
	if event.ReplyTo == nil {
		return ErrNotFound
	}
	query := s.db.WithContext(ctx).Table("channel_installations AS installations").
		Joins("JOIN external_conversation_mappings AS mappings ON mappings.installation_id = installations.id AND mappings.organization_id = installations.organization_id AND mappings.workspace_id = installations.workspace_id").
		Joins("JOIN channel_bindings AS bindings ON bindings.conversation_mapping_id = mappings.id AND bindings.installation_id = installations.id AND bindings.organization_id = installations.organization_id AND bindings.workspace_id = installations.workspace_id AND bindings.deleted_at IS NULL").
		Where("installations.id = ? AND installations.organization_id = ? AND installations.workspace_id = ? AND installations.provider = ? AND installations.status = ? AND installations.deleted_at IS NULL",
			event.ReplyTo.InstallationID, run.OrganizationID, run.WorkspaceID, event.ReplyTo.Provider, channelruntime.InstallationStatusActive).
		Where("mappings.external_conversation_id = ? AND bindings.subject_type = ? AND bindings.subject_id = ? AND bindings.enabled = ?",
			event.ReplyTo.ConversationID, runtimeidentity.SubjectTypeDigitalHuman, run.DigitalHumanID, true)
	if binding != "" {
		query = query.Where("bindings.id = ?", binding)
	}
	if conversation != nil {
		query = query.Where("mappings.local_conversation_id = ?", *conversation)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrNotFound
	}
	if run.ActorID == nil {
		return ErrNotFound
	}
	return s.authorizeBackgroundAccount(ctx, run, *run.ActorID)
}

func (s *AccessService) authorizeBackgroundTask(ctx context.Context, run *Run, event invocation.Event) error {
	taskID, _ := stringContextValue(event.Context, "task_id")
	taskRunID, _ := stringContextValue(event.Context, "task_run_id")
	actionRunID, _ := stringContextValue(event.Context, "action_run_id")
	if taskID == "" || taskRunID == "" || actionRunID == "" {
		return ErrNotFound
	}
	var task automationmodel.AutomationTask
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND subject_id = ? AND task_kind = ?",
		taskID, run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, automationmodel.AutomationTaskKindDigitalHuman).Take(&task).Error; err != nil {
		return backgroundRecordError(err)
	}
	// Once schedules are marked completed atomically when dispatch advances,
	// before their queued action starts. Only that still-active correlated
	// action below may execute; pause/archive/cancel remain immediate revocations.
	if task.Status != automationmodel.AutomationTaskStatusActive &&
		!(task.Status == automationmodel.AutomationTaskStatusCompleted && task.ScheduleType == automationmodel.AutomationScheduleTypeOnce) {
		return ErrNotFound
	}
	if task.WorkStatus != nil && *task.WorkStatus == automationmodel.AutomationWorkStatusCanceled {
		return ErrNotFound
	}
	var count int64
	if err := s.db.WithContext(ctx).Table("automation_task_runs AS runs").
		Joins("JOIN automation_action_runs AS actions ON actions.task_run_id = runs.id").
		Where("runs.id = ? AND runs.task_id = ? AND runs.status IN ? AND actions.id = ? AND actions.action_type = ? AND actions.status IN ?",
			taskRunID, taskID, []automationmodel.AutomationTaskRunStatus{automationmodel.AutomationTaskRunStatusQueued, automationmodel.AutomationTaskRunStatusRunning},
			actionRunID, automationmodel.AutomationActionTypeRunDigitalHuman, []automationmodel.AutomationActionRunStatus{automationmodel.AutomationActionRunStatusQueued, automationmodel.AutomationActionRunStatusRunning}).
		Where("actions.external_run_id IS NULL OR actions.external_run_id = ?", run.ID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 || task.OwnerKind == nil {
		return ErrNotFound
	}
	owner, _ := stringContextValue(event.Context, "task_owner_kind")
	if owner != string(*task.OwnerKind) {
		return ErrNotFound
	}
	switch TaskOwnerKind(*task.OwnerKind) {
	case TaskOwnerUserSubject:
		if task.OwnerSubjectID == nil || task.OwnerSubjectKind == nil || run.UserSubjectID == nil ||
			*task.OwnerSubjectID != run.UserSubjectID.String() || *task.OwnerSubjectKind != run.UserSubjectKind {
			return ErrNotFound
		}
		if run.UserSubjectKind == string(usersubject.Account) {
			return s.authorizeBackgroundAccount(ctx, run, *run.UserSubjectID)
		}
		if run.UserSubjectKind != string(usersubject.APIEndUser) {
			return ErrNotFound
		}
		var subject usersubject.Subject
		if err := s.db.WithContext(ctx).Where("id = ? AND kind = ? AND issuer_type = ? AND issuer_id = ?", *run.UserSubjectID, usersubject.APIEndUser, "digital_human_api", run.DigitalHumanID).Take(&subject).Error; err != nil {
			return ErrNotFound
		}
	case TaskOwnerDigitalHuman:
		// Scheduler runs intentionally do not persist an interactive ActorID.
		// Their frozen billing principal must still match this confirmed task's
		// creator; it authorizes execution but never owns the environment.
		billing := runtimeScope(run).AccountID
		if run.UserSubjectID != nil || billing == uuid.Nil || task.CreatedBy != billing.String() {
			return ErrNotFound
		}
		return s.authorizeBackgroundAccount(ctx, run, billing)
	case TaskOwnerConversation:
		if run.UserSubjectID != nil || task.OwnerConversationID == nil {
			return ErrNotFound
		}
		provider := strings.TrimPrefix(pointerString(task.SourceRef), "channel:")
		installation, _ := stringContextValue(task.SourceSnapshot, "installation_id")
		conversation, _ := stringContextValue(task.SourceSnapshot, "conversation_id")
		address := &invocation.ReplyAddress{Provider: provider, InstallationID: installation, ConversationID: conversation}
		if event.ReplyTo != nil && (event.ReplyTo.Provider != provider || event.ReplyTo.InstallationID != installation || event.ReplyTo.ConversationID != conversation) {
			return ErrNotFound
		}
		ownerID, err := ChannelTaskConversationID(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID,
			provider, installation, conversation)
		if err != nil || *task.OwnerConversationID != ownerID.String() {
			return ErrNotFound
		}
		event.ReplyTo = address
		return s.authorizeBackgroundChannel(ctx, run, event, "", nil)
	default:
		return ErrNotFound
	}
	return nil
}

func (s *AccessService) authorizeBackgroundAccount(ctx context.Context, run *Run, account uuid.UUID) error {
	var count int64
	err := s.db.WithContext(ctx).Table("accounts AS accounts").
		Joins("JOIN workspace_members AS members ON members.account_id = accounts.id").
		Joins("JOIN workspaces ON workspaces.id = members.workspace_id").
		Where("accounts.id = ? AND accounts.status = ? AND accounts.deleted_at IS NULL AND members.workspace_id = ? AND workspaces.organization_id = ? AND workspaces.status = ?",
			account, "active", run.WorkspaceID, run.OrganizationID, "normal").Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func backgroundRecordError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("load background execution authorization: %w", err)
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
