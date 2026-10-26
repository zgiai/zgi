package digitalhuman

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

// artifactApplicationType names DigitalHumans in the artifact center.
const artifactApplicationType = "digital_human"

// ArtifactsEnabled reports whether DigitalHumans keep artifacts (ADR 0009).
// It gates the model tools and the capture of report files; reading
// existing artifacts is always allowed.
func ArtifactsEnabled() bool {
	return envFlagDefaultOn("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED")
}

// ToolFileArtifactBlobs stores artifact file versions as persistent,
// organization-owned tool files. They carry no execution identity: an
// artifact owns its copy, independent of the run that produced it.
type ToolFileArtifactBlobs struct{}

// Store saves one version's bytes.
func (ToolFileArtifactBlobs) Store(ctx context.Context, organizationID uuid.UUID, name, mimeType string, data []byte) (string, error) {
	manager := toolfile.GlobalToolFileManager
	if manager == nil {
		return "", toolfile.ErrExecutionFileAccess
	}
	file, err := manager.CreateFileByRaw(ctx, toolfile.CreateFileByRawParams{UserID: organizationID.String(), TenantID: organizationID.String(),
		Filename: &name, FileData: data, MimeType: mimeType, Lifecycle: toolfile.ToolFileLifecyclePersistent})
	if err != nil {
		return "", err
	}
	return file.ID, nil
}

// Read loads one version's bytes from its organization.
func (ToolFileArtifactBlobs) Read(ctx context.Context, organizationID uuid.UUID, fileID string) ([]byte, error) {
	manager := toolfile.GlobalToolFileManager
	if manager == nil {
		return nil, toolfile.ErrExecutionFileAccess
	}
	file, err := manager.GetToolFileByID(ctx, fileID)
	if err != nil || file == nil || file.TenantID != organizationID.String() || protectedToolFile(file) || file.DeletedAt != nil {
		return nil, toolfile.ErrExecutionFileAccess
	}
	data, _, err := manager.GetFileBinary(ctx, fileID)
	return data, err
}

var _ artifacts.BlobStore = ToolFileArtifactBlobs{}

// artifactScope is a DigitalHuman's place in the artifact center.
func artifactScope(organizationID, workspaceID, digitalHumanID uuid.UUID) artifacts.Scope {
	return artifacts.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ApplicationType: artifactApplicationType, ApplicationID: digitalHumanID}
}

// runArtifactOwner says whose artifacts a run publishes and reads, and how
// widely they are shared by default:
//   - a person's run (console, WebApp, API, or their own task) works for that
//     account or end user, privately;
//   - a task center task works for the account that created it, for the team;
//   - a channel conversation, or a task it requested, works for the
//     workspace, for the team.
//
// Members' runs read the team's artifacts as well; end users' never do.
func runArtifactOwner(run *Run, event map[string]any) (owner artifacts.Owner, visibility artifacts.Visibility, member bool, ok bool) {
	if run == nil {
		return artifacts.Owner{}, "", false, false
	}
	if run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil {
		switch run.UserSubjectKind {
		case "account":
			return artifacts.Owner{Kind: artifacts.OwnerAccount, ID: *run.UserSubjectID}, artifacts.VisibilityOwner, true, true
		case "api_end_user":
			return artifacts.Owner{Kind: artifacts.OwnerAPIEndUser, ID: *run.UserSubjectID}, artifacts.VisibilityOwner, false, true
		}
		return artifacts.Owner{}, "", false, false
	}
	switch run.Surface {
	case SurfaceTask:
		switch kind, _ := event["task_owner_kind"].(string); kind {
		case string(TaskOwnerDigitalHuman):
			// Scheduled runs carry no interactive actor, so the frozen principal
			// (the account that created and owns the task) keeps the deliverables.
			// This matches the background-authorization check that freezes the
			// task creator as the run's owner.
			ownerID := run.ActorID
			if (ownerID == nil || *ownerID == uuid.Nil) && run.PrincipalID != nil && *run.PrincipalID != uuid.Nil {
				ownerID = run.PrincipalID
			}
			if ownerID != nil && *ownerID != uuid.Nil {
				return artifacts.Owner{Kind: artifacts.OwnerAccount, ID: *ownerID}, artifacts.VisibilityTeam, true, true
			}
		case string(TaskOwnerConversation):
			return artifacts.Owner{Kind: artifacts.OwnerWorkspace, ID: run.WorkspaceID}, artifacts.VisibilityTeam, true, true
		}
	case SurfaceChannel:
		return artifacts.Owner{Kind: artifacts.OwnerWorkspace, ID: run.WorkspaceID}, artifacts.VisibilityTeam, true, true
	}
	return artifacts.Owner{}, "", false, false
}

// artifactKindFor picks an artifact kind for a file by its type.
func artifactKindFor(mimeType, name string) artifacts.Kind {
	mimeType = strings.ToLower(mimeType)
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return artifacts.KindImage
	case mimeType == "text/csv" || strings.Contains(mimeType, "spreadsheet") || strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".xlsx"):
		return artifacts.KindTable
	default:
		return artifacts.KindFile
	}
}
