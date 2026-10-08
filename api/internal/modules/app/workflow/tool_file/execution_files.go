package tool_file

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

var ErrExecutionFileAccess = errors.New("execution file is unavailable in this scope")

// ExecutionFileAccess is supplied by an authenticated conversation adapter.
// Publication and execution admission deliberately are not read permissions:
// retained results remain readable after a new publication or compute disable.
type ExecutionFileAccess struct {
	OrganizationID  string
	SubjectID       string
	WorkspaceID     string
	ApplicationType string
	ApplicationID   string
	Entry           string
	SubjectKind     string
	ConversationID  string
}

func (access ExecutionFileAccess) valid() bool {
	identity := execution.Identity{Version: execution.IdentityVersion,
		WorkspaceID: access.WorkspaceID, ApplicationType: access.ApplicationType,
		ApplicationID: access.ApplicationID, Entry: access.Entry, SubjectKind: access.SubjectKind,
		ConversationID: access.ConversationID, PublicationRef: "file-access"}
	return validFileUUID(access.OrganizationID) && validFileUUID(access.SubjectID) &&
		validFileUUID(access.WorkspaceID) && validFileUUID(access.ApplicationID) &&
		validFileUUID(access.ConversationID) && identity.Validate() == nil
}

func (access ExecutionFileAccess) owns(file *ToolFile) bool {
	if file == nil || file.ExecutionIdentity == nil || file.ExecutionMessageID == nil ||
		!validFileUUID(*file.ExecutionMessageID) || file.ConversationID == nil || !access.valid() {
		return false
	}
	i := file.ExecutionIdentity
	return i.Validate() == nil && file.DeletedAt == nil &&
		file.TenantID == access.OrganizationID && file.UserID == access.SubjectID &&
		*file.ConversationID == access.ConversationID && i.ConversationID == access.ConversationID &&
		i.WorkspaceID == access.WorkspaceID && i.ApplicationType == access.ApplicationType &&
		i.ApplicationID == access.ApplicationID && i.Entry == access.Entry && i.SubjectKind == access.SubjectKind
}

// GetExecutionFileBinary checks immutable ownership before touching storage.
// The caller must also validate current application and conversation access.
// No public presigned storage URL is returned, even for remote storage backends.
func (tm *ToolFileManager) GetExecutionFileBinary(ctx context.Context, access ExecutionFileAccess, id string) ([]byte, *ToolFile, error) {
	if !access.valid() || !validFileUUID(id) {
		return nil, nil, ErrExecutionFileAccess
	}
	file, err := tm.GetToolFileByID(ctx, id)
	if err != nil || !access.owns(file) {
		return nil, nil, ErrExecutionFileAccess
	}
	data, err := tm.storage.Load(file.FileKey)
	if err != nil {
		return nil, nil, fmt.Errorf("load execution artifact: %w", err)
	}
	return data, file, nil
}

func validateExecutionFileOwner(params CreateFileByRawParams) (*execution.Identity, *string, error) {
	if params.ExecutionIdentity == nil && params.ExecutionMessageID == nil {
		return nil, nil, nil // Historical ToolFile contract.
	}
	if params.ExecutionIdentity == nil || params.ExecutionMessageID == nil || params.ConversationID == nil {
		return nil, nil, ErrExecutionFileAccess
	}
	i := *params.ExecutionIdentity
	messageID := *params.ExecutionMessageID
	access := ExecutionFileAccess{OrganizationID: params.TenantID, SubjectID: params.UserID,
		WorkspaceID: i.WorkspaceID, ApplicationType: i.ApplicationType, ApplicationID: i.ApplicationID,
		Entry: i.Entry, SubjectKind: i.SubjectKind, ConversationID: *params.ConversationID}
	if !access.valid() || i.Validate() != nil || i.ConversationID != *params.ConversationID || !validFileUUID(messageID) {
		return nil, nil, ErrExecutionFileAccess
	}
	return &i, &messageID, nil
}

func validFileUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
