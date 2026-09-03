package container

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/feishu"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// File authorization belongs here, outside the provider leaf. Public model
// arguments can name a platform file ID, never its storage key or source URL.
type connectorResourceFiles struct{ container *ServiceContainer }

func (a connectorResourceFiles) Read(ctx context.Context, scope providers.ResourceScope, fileID string) (feishu.ResourceFile, error) {
	if _, err := uuid.Parse(fileID); err != nil || a.container == nil || scope.OwnerAccountID == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.Target.Validate() != nil {
		return feishu.ResourceFile{}, providers.Invalid()
	}
	files := a.container.GetFileService()
	file, err := files.GetFileByID(ctx, fileID)
	if err != nil || file == nil || file.IsArchived || file.Size <= 0 || file.Size > feishu.MaxResourceBytes {
		return feishu.ResourceFile{}, providers.Invalid()
	}
	org := strings.TrimSpace(file.OrganizationID)
	if org == "" {
		org = strings.TrimSpace(file.TenantID)
	}
	if org != scope.OrganizationID.String() {
		return feishu.ResourceFile{}, providers.Invalid()
	}
	workspace := ""
	if file.WorkspaceID != nil {
		workspace = *file.WorkspaceID
	} else if file.TeamTenantID != nil {
		workspace = *file.TeamTenantID
	}
	if file.IsTemporary || workspace == "" {
		if file.CreatedBy != scope.OwnerAccountID.String() {
			return feishu.ResourceFile{}, providers.Invalid()
		}
	} else {
		allowed, err := a.container.GetOrganizationService().CheckWorkspacePermission(ctx, org, workspace, scope.OwnerAccountID.String(), workspacemodel.WorkspacePermissionFilePreview)
		if err != nil || !allowed {
			return feishu.ResourceFile{}, providers.Invalid()
		}
	}
	content, err := files.DownloadFile(ctx, fileID)
	if err != nil || int64(len(content)) != file.Size || feishu.ResourceMIME(content) == "" {
		clear(content)
		return feishu.ResourceFile{}, providers.Invalid()
	}
	return feishu.ResourceFile{Name: file.Name, MIME: file.MimeType, Content: content}, nil
}
func (a connectorResourceFiles) Save(ctx context.Context, scope providers.ResourceScope, file feishu.ResourceFile) (string, error) {
	if a.container == nil || scope.OwnerAccountID == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.Target.Validate() != nil || feishu.ResourceMIME(file.Content) == "" {
		return "", providers.Invalid()
	}
	// Private temporary artifacts follow the existing file lifecycle and TTL;
	// they are not published to a workspace or returned as public storage URLs.
	saved, err := a.container.GetFileService().UploadFile(ctx, file.Name, file.Content, file.MIME, scope.OwnerAccountID.String(), scope.OrganizationID.String(), filemodel.CreatedByRoleAccount, nil, nil, true, false)
	if err != nil || saved == nil {
		return "", providers.Invalid()
	}
	return saved.ID, nil
}
