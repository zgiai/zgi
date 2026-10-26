package digitalhumantask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// PauseSupersededSocialOperations stops prior social-operations schedules
// before a replacement plan is created. Historical tasks remain available.
func (s *Service) PauseSupersededSocialOperations(ctx context.Context, access digitalhuman.TaskAccessContext, versionID uuid.UUID, configurationRevision int64) error {
	if s == nil || versionID == uuid.Nil || configurationRevision < 1 || validateBoundary(access) != nil ||
		access.Principal.Type != invocation.PrincipalAccount || access.Owner != digitalhuman.SharedTaskOwner() ||
		access.Surface != digitalhuman.SurfaceConsole || access.CurrentTaskID != nil {
		return digitalhuman.ErrTaskPolicyRejected
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(access.Principal.ID))
	if err != nil || ownerID == uuid.Nil || ownerID.String() != access.Principal.ID {
		return digitalhuman.ErrTaskPolicyRejected
	}
	var tasks []automationmodel.AutomationTask
	err = s.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND subject_id = ? AND task_kind = ? AND created_by = ? AND source_ref = ? AND status = ?",
		access.OrganizationID, access.WorkspaceID, access.DigitalHumanID, automationmodel.AutomationTaskKindDigitalHuman,
		ownerID, "social_operations", automationmodel.AutomationTaskStatusActive,
	).Order("created_at, id").Find(&tasks).Error
	if err != nil {
		return normalizeError(err)
	}
	for _, task := range tasks {
		metadata, ok := strictSocialOperationsMetadata(task.SourceSnapshot)
		if !ok || metadata.Scope.OrganizationID != access.OrganizationID || metadata.Scope.WorkspaceID != access.WorkspaceID ||
			metadata.Scope.OwnerID != ownerID || metadata.Scope.EmployeeID != access.DigitalHumanID {
			return digitalhuman.ErrTaskPolicyRejected
		}
		if metadata.EmployeeVersionID == versionID && metadata.ConfigurationRevision == configurationRevision {
			continue
		}
		taskID, parseErr := uuid.Parse(task.ID)
		if parseErr != nil || taskID == uuid.Nil || taskID.String() != task.ID {
			return digitalhuman.ErrTaskPolicyRejected
		}
		operationID := uuid.NewSHA1(taskID, []byte(fmt.Sprintf("social_operations/pause_superseded/%s/%d", versionID, configurationRevision)))
		_, err = s.ApplyTaskAction(ctx, access, taskID, digitalhuman.TaskActionPause,
			digitalhuman.ApplyTaskActionRequest{ExpectedRevision: task.Revision}, digitalhuman.TaskMutationContext{
				ActorType: "account", ActorRef: ownerID.String(), OperationID: operationID, Source: "social_operations",
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func strictSocialOperationsMetadata(snapshot map[string]any) (digitalhuman.SocialOperationsTaskMetadata, bool) {
	raw, ok := snapshot["social_operations"]
	if !ok {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	var metadata digitalhuman.SocialOperationsTaskMetadata
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&metadata); err != nil {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	return metadata, true
}
