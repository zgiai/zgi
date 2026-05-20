package interfaces

import (
	"context"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type RegisterService interface {
	ActivateCheck(ctx context.Context, workspaceID, email, token string) (map[string]interface{}, bool)

	Activate(ctx context.Context, workspaceID, email, token, name, password, lang, timezone string) (interface{}, error)

	CheckRegisterValidity(ctx context.Context, email, code, token string) (bool, error)

	RegisterFinish(ctx context.Context, email, name, password string) (*auth_model.Account, error)

	InviteMember(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string) (string, error)
	InviteMemberEx(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string, name, mobile, gender, position string, sendEmail bool) (string, error)

	JoinByInvitation(ctx context.Context, accountID, code string) (*workspace_model.Workspace, error)

	Register(ctx context.Context, email string, password string, name string) (interface{}, error)

	GetInvitationIfTokenValid(workspace_id *string, email, token string) map[string]interface{}

	ValidateInvitationCode(ctx context.Context, code string) (bool, error)

	GetInvitationData(ctx context.Context, token string) (map[string]interface{}, error)
}
