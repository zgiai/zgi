package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type runtimeAttachmentPermissionCheck struct{ allowed bool }

func (p *runtimeAttachmentPermissionCheck) CheckWorkspacePermission(context.Context, string, string, string, workspacemodel.WorkspacePermissionCode) (bool, error) {
	return p.allowed, nil
}

func TestRuntimeAttachmentAuthorizationRechecksWorkspacePermission(t *testing.T) {
	org, workspace, user := uuid.New(), uuid.NewString(), uuid.New()
	scope := Scope{OrganizationID: org, AccountID: user}
	file := &dto.UploadFile{ID: uuid.NewString(), OrganizationID: org.String(), WorkspaceID: &workspace, CreatedBy: uuid.NewString()}
	perms := &runtimeAttachmentPermissionCheck{allowed: true}
	if err := AuthorizeAttachment(t.Context(), scope, perms, file); err != nil {
		t.Fatal(err)
	}
	perms.allowed = false
	if err := AuthorizeAttachment(t.Context(), scope, perms, file); err == nil {
		t.Fatal("revoked workspace permission was cached")
	}
	if err := AuthorizeAttachment(t.Context(), scope, nil, file); err == nil {
		t.Fatal("missing permission service accepted")
	}
	// Preserve the existing external upload identity without rewriting billing.
	endUser := uuid.New()
	scope.ExternalUserID = &endUser
	file.IsTemporary, file.CreatedBy = true, endUser.String()
	if err := AuthorizeAttachment(t.Context(), scope, nil, file); err != nil {
		t.Fatal(err)
	}
	file.CreatedBy = user.String()
	if err := AuthorizeAttachment(t.Context(), scope, nil, file); err == nil {
		t.Fatal("credential owner substituted for external user")
	}
}

func TestRuntimeAttachmentRefsExposeOnlyAuthorizedMetadata(t *testing.T) {
	prepared := &PreparedChat{parts: &chatRequestParts{Attachments: &attachmentBundle{Files: []attachmentFile{{
		ID: "file-1", Name: "report.csv", Size: 12, Content: "private file content",
	}}}}}
	refs := prepared.RuntimeAttachmentRefs()
	if len(refs) != 1 || refs[0].ID != "file-1" || refs[0].Name != "report.csv" || refs[0].Size != 12 {
		t.Fatalf("attachment refs = %#v", refs)
	}
	refs[0].Name = "changed"
	if prepared.parts.Attachments.Files[0].Name != "report.csv" {
		t.Fatal("caller mutated prepared attachment metadata")
	}
	if (&PreparedChat{}).RuntimeAttachmentRefs() != nil {
		t.Fatal("empty prepared chat returned attachment refs")
	}
}
