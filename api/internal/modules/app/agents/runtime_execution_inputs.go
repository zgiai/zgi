package agents

import (
	"context"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/gorm"
)

// resolveAgentExecutionInputs is the protected adapter for published entries.
// It is not wired into new entry factories until the approval gate is complete.
func (h *AgentsHandler) resolveAgentExecutionInputs(prepared *runtimeservice.PreparedChat, scope execution.Scope) (*executionruntime.ScopedAttachmentReader, error) {
	if prepared == nil || scope.Identity == nil {
		return nil, execution.ErrInvalidIdentity
	}
	access, ok := agentExecutionFileAccess(prepared.Scope, prepared.Caller, prepared.Conversation)
	identity := scope.Identity
	if !ok || access.OrganizationID != scope.OrganizationID || access.SubjectID != scope.SubjectID ||
		access.WorkspaceID != identity.WorkspaceID || access.ApplicationID != identity.ApplicationID ||
		access.ApplicationType != identity.ApplicationType || access.Entry != identity.Entry ||
		access.SubjectKind != identity.SubjectKind || access.ConversationID != identity.ConversationID {
		return nil, execution.ErrInvalidIdentity
	}
	owner := execution.UploadOwner{OrganizationID: access.OrganizationID, WorkspaceID: access.WorkspaceID,
		ApplicationType: access.ApplicationType, ApplicationID: access.ApplicationID, Entry: access.Entry,
		SubjectKind: access.SubjectKind, SubjectID: access.SubjectID}
	return executionruntime.NewScopedAttachmentReader(h.fileService, scope, prepared.RuntimeAttachmentRefs(), func(ctx context.Context, file *dto.UploadFile) error {
		return authorizeAgentExecutionUpload(ctx, h.db, owner, file)
	})
}

func authorizeAgentExecutionUpload(ctx context.Context, db *gorm.DB, owner execution.UploadOwner, file *dto.UploadFile) error {
	// Public execution accepts only uploads made for this exact application,
	// entry and end user. Generic managed files and creator-owned uploads do not
	// inherit permission from the API billing account. Temporary file metadata
	// can use a zero tenant or nil workspace; the immutable binding is decisive.
	if file == nil || !file.IsTemporary || file.CreatedByRole != dto.CreatedByRoleEndUser || file.CreatedBy != owner.SubjectID {
		return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("agent_execution.input_owner"))
	}
	return execution.AuthorizeUpload(ctx, db, file.ID, owner)
}
