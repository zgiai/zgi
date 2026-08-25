package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type digitalHumanAutomationRunner struct{ runtime *digitalhuman.RuntimeService }

func (r *digitalHumanAutomationRunner) RunDigitalHuman(ctx context.Context, req automationaction.DigitalHumanRunRequest) (*automationaction.DigitalHumanRunResult, error) {
	if r == nil || r.runtime == nil {
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
	actorID, err := uuid.Parse(req.ActorID)
	if err != nil {
		actorID = digitalHumanID
	}
	criteria := ""
	if len(req.SuccessCriteria) > 0 {
		criteria = "\nSuccess criteria:\n- " + strings.Join(req.SuccessCriteria, "\n- ")
	}
	query := fmt.Sprintf("Execute the confirmed task %q.\nGoal: %s%s\nWhen finished, provide the final result and any generated files.", req.Name, req.Goal, criteria)
	event := invocation.Event{ID: req.ActionRunID, Type: invocation.EventTypeSchedule, Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: organizationID, WorkspaceID: workspaceID}, Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}, OccurredAt: time.Now().UTC(), Context: map[string]any{"task_id": req.TaskID, "task_run_id": req.TaskRunID, "action_run_id": req.ActionRunID}, ReplyTo: req.ReplyTo}
	run, _, err := r.runtime.SubmitInvocation(ctx, digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}, event, query, nil)
	if err != nil {
		return nil, err
	}
	return &automationaction.DigitalHumanRunResult{RunID: run.ID.String()}, nil
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
