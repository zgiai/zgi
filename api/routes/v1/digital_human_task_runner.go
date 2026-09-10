package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

var errDigitalHumanTaskExecutionUnavailable = errors.New("digital human task execution is unavailable")

type digitalHumanAutomationRunner struct {
	runtime *digitalhuman.RuntimeService
	db      *gorm.DB
}

func (r *digitalHumanAutomationRunner) RunDigitalHuman(ctx context.Context, req automationaction.DigitalHumanRunRequest) (*automationaction.DigitalHumanRunResult, error) {
	if r == nil || r.runtime == nil || r.db == nil {
		return nil, fmt.Errorf("digital human runtime is not ready")
	}
	organizationID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		return nil, err
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	digitalHumanID, err := uuid.Parse(req.DigitalHumanID)
	if err != nil {
		return nil, err
	}
	owner, err := taskOwnerFromRunRequest(req)
	if err != nil {
		return nil, errDigitalHumanTaskExecutionUnavailable
	}
	if err := r.preflight(ctx, organizationID, workspaceID, digitalHumanID, owner, req); err != nil {
		return nil, errDigitalHumanTaskExecutionUnavailable
	}
	actorID := digitalHumanID
	scope := digitalhuman.Scope{
		OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID,
		PrincipalType: string(invocation.PrincipalSystem), Surface: digitalhuman.SurfaceTask,
	}
	if owner.Kind == digitalhuman.TaskOwnerUserSubject {
		subjectID := owner.Subject.ID
		scope.UserSubjectKind, scope.UserSubjectID = string(owner.Subject.Kind), &subjectID
		if owner.Subject.Kind == usersubject.Account {
			scope.ActorID = subjectID
		}
	}
	criteria := ""
	if len(req.SuccessCriteria) > 0 {
		criteria = "\nSuccess criteria:\n- " + strings.Join(req.SuccessCriteria, "\n- ")
	}
	query := fmt.Sprintf("Execute the confirmed task %q.\nGoal: %s%s\nWhen finished, provide the final result and any generated files.", req.Name, req.Goal, criteria)
	event := invocation.Event{ID: req.ActionRunID, Type: invocation.EventTypeSchedule, Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: organizationID, WorkspaceID: workspaceID}, Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}, OccurredAt: time.Now().UTC(), Context: map[string]any{"task_id": req.TaskID, "task_run_id": req.TaskRunID, "action_run_id": req.ActionRunID, "task_owner_kind": req.OwnerKind, "origin_surface": req.OriginSurface}, ReplyTo: req.ReplyTo}
	run, _, err := r.runtime.SubmitInvocation(ctx, scope, event, query, nil)
	if err != nil {
		return nil, err
	}
	return &automationaction.DigitalHumanRunResult{RunID: run.ID.String()}, nil
}

func taskOwnerFromRunRequest(req automationaction.DigitalHumanRunRequest) (digitalhuman.TaskOwnerRef, error) {
	switch digitalhuman.TaskOwnerKind(strings.TrimSpace(req.OwnerKind)) {
	case digitalhuman.TaskOwnerDigitalHuman:
		owner := digitalhuman.SharedTaskOwner()
		if strings.TrimSpace(req.OwnerSubjectKind) != "" || strings.TrimSpace(req.OwnerSubjectID) != "" || strings.TrimSpace(req.OwnerConversationID) != "" {
			return digitalhuman.TaskOwnerRef{}, errDigitalHumanTaskExecutionUnavailable
		}
		return owner, nil
	case digitalhuman.TaskOwnerUserSubject:
		subjectID, err := uuid.Parse(strings.TrimSpace(req.OwnerSubjectID))
		owner := digitalhuman.UserTaskOwner(usersubject.Ref{Kind: usersubject.Kind(strings.TrimSpace(req.OwnerSubjectKind)), ID: subjectID})
		if err != nil || strings.TrimSpace(req.OwnerConversationID) != "" || !owner.Valid() {
			return digitalhuman.TaskOwnerRef{}, errDigitalHumanTaskExecutionUnavailable
		}
		return owner, nil
	case digitalhuman.TaskOwnerConversation:
		conversationID, err := uuid.Parse(strings.TrimSpace(req.OwnerConversationID))
		owner := digitalhuman.ConversationTaskOwner(conversationID)
		if err != nil || strings.TrimSpace(req.OwnerSubjectKind) != "" || strings.TrimSpace(req.OwnerSubjectID) != "" || !owner.Valid() {
			return digitalhuman.TaskOwnerRef{}, errDigitalHumanTaskExecutionUnavailable
		}
		return owner, nil
	default:
		return digitalhuman.TaskOwnerRef{}, errDigitalHumanTaskExecutionUnavailable
	}
}

func (r *digitalHumanAutomationRunner) preflight(ctx context.Context, organizationID, workspaceID, digitalHumanID uuid.UUID, owner digitalhuman.TaskOwnerRef, req automationaction.DigitalHumanRunRequest) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&digitalhuman.Definition{}).
		Where("id = ? AND organization_id = ? AND workspace_id = ? AND status = ? AND published_version_id IS NOT NULL AND deleted_at IS NULL", digitalHumanID, organizationID, workspaceID, digitalhuman.StatusActive).
		Count(&count).Error; err != nil || count != 1 {
		return errDigitalHumanTaskExecutionUnavailable
	}
	switch owner.Kind {
	case digitalhuman.TaskOwnerDigitalHuman:
		if req.OriginSurface != digitalhuman.SurfaceConsole {
			return errDigitalHumanTaskExecutionUnavailable
		}
	case digitalhuman.TaskOwnerUserSubject:
		if req.OriginSurface != digitalhuman.SurfaceConsole {
			return errDigitalHumanTaskExecutionUnavailable
		}
		count = 0
		if err := r.db.WithContext(ctx).Model(&usersubject.Subject{}).
			Where("id = ? AND kind = ?", owner.Subject.ID, owner.Subject.Kind).Count(&count).Error; err != nil || count != 1 {
			return errDigitalHumanTaskExecutionUnavailable
		}
		if owner.Subject.Kind == usersubject.Account {
			count = 0
			if err := r.db.WithContext(ctx).Table("workspace_members").
				Where("workspace_id = ? AND account_id = ?", workspaceID, owner.Subject.ID).Count(&count).Error; err != nil || count != 1 {
				return errDigitalHumanTaskExecutionUnavailable
			}
		}
	case digitalhuman.TaskOwnerConversation:
		if req.OriginSurface != digitalhuman.SurfaceChannel {
			return errDigitalHumanTaskExecutionUnavailable
		}
		installationID, err := uuid.Parse(strings.TrimSpace(req.ChannelInstallationID))
		derivedOwner, deriveErr := digitalhuman.ChannelTaskConversationID(organizationID, workspaceID, digitalHumanID, req.ChannelProvider, req.ChannelInstallationID, req.ChannelConversationID)
		if err != nil || deriveErr != nil || derivedOwner != owner.ConversationID {
			return errDigitalHumanTaskExecutionUnavailable
		}
		if req.ReplyTo != nil && (strings.TrimSpace(req.ReplyTo.InstallationID) != strings.TrimSpace(req.ChannelInstallationID) || strings.TrimSpace(req.ReplyTo.ConversationID) != strings.TrimSpace(req.ChannelConversationID)) {
			return errDigitalHumanTaskExecutionUnavailable
		}
		count = 0
		if err := r.db.WithContext(ctx).Table("channel_installations AS installations").
			Joins("JOIN external_conversation_mappings AS mappings ON mappings.installation_id = installations.id AND mappings.organization_id = installations.organization_id AND mappings.workspace_id = installations.workspace_id").
			Joins("JOIN channel_bindings AS bindings ON bindings.conversation_mapping_id = mappings.id AND bindings.installation_id = installations.id AND bindings.deleted_at IS NULL").
			Where("installations.id = ? AND installations.organization_id = ? AND installations.workspace_id = ? AND installations.provider = ? AND installations.status = ? AND installations.deleted_at IS NULL", installationID, organizationID, workspaceID, strings.ToLower(strings.TrimSpace(req.ChannelProvider)), channelruntime.InstallationStatusActive).
			Where("mappings.external_conversation_id = ? AND bindings.subject_type = ? AND bindings.subject_id = ? AND bindings.enabled = ?", strings.TrimSpace(req.ChannelConversationID), runtimeidentity.SubjectTypeDigitalHuman, digitalHumanID, true).
			Count(&count).Error; err != nil || count != 1 {
			return errDigitalHumanTaskExecutionUnavailable
		}
	default:
		return errDigitalHumanTaskExecutionUnavailable
	}
	return nil
}

func (r *digitalHumanAutomationRunner) GetDigitalHumanRun(ctx context.Context, organization, workspace, digitalHuman, run string) (*automationaction.DigitalHumanRunState, error) {
	organizationID, err := uuid.Parse(organization)
	if err != nil {
		return nil, err
	}
	workspaceID, err := uuid.Parse(workspace)
	if err != nil {
		return nil, err
	}
	digitalHumanID, err := uuid.Parse(digitalHuman)
	if err != nil {
		return nil, err
	}
	runID, err := uuid.Parse(run)
	if err != nil {
		return nil, err
	}
	result, err := r.runtime.GetRun(ctx, digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID}, digitalHumanID, runID)
	if err != nil {
		return nil, err
	}
	return &automationaction.DigitalHumanRunState{Status: string(result.Status), Result: result.ResultSummary, ErrorMessage: result.ErrorMessage}, nil
}

func (r *digitalHumanAutomationRunner) CancelDigitalHumanRun(ctx context.Context, organization, workspace, digitalHuman, run string) error {
	organizationID, err := uuid.Parse(organization)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(workspace)
	if err != nil {
		return err
	}
	digitalHumanID, err := uuid.Parse(digitalHuman)
	if err != nil {
		return err
	}
	runID, err := uuid.Parse(run)
	if err != nil {
		return err
	}
	_, err = r.runtime.CancelRun(ctx, digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: digitalHumanID}, digitalHumanID, runID)
	return err
}
