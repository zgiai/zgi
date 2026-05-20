package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/dto"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
)

type AccountService interface {
	SendResetPasswordEmail(ctx context.Context, account *auth_model.Account, email string, language string) (string, error)
	SendDirectAddMemberEmail(ctx context.Context, account *auth_model.Account, groupID, groupName, departmentName, language string) error
	CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (*auth_model.Account, error)
	GetAccountExtensionByID(ctx context.Context, id string) (auth_model.JSONMap, error)
	GetAccountByEmail(ctx context.Context, email string) (*auth_model.Account, error)
	UpdateAccount(ctx context.Context, id string, req *dto.UpdateAccountRequest) error
	DeleteAccount(ctx context.Context, id string) error
	DeleteCurrentAccount(ctx context.Context, id, password string) error
	ChangePassword(ctx context.Context, id string, oldPassword, newPassword string) error
	ResetPassword(ctx context.Context, resetToken, newPassword string) error
	VerifyAccount(ctx context.Context, token string) error
	Logout(ctx context.Context, accessToken, refreshToken string) error
	RefreshToken(ctx context.Context, refreshToken string) (*dto.TokenResponse, error)
	GetAccountProfile(ctx context.Context, accountID string) (*dto.AccountProfileResponse, error)
	UpdateAccountProfile(ctx context.Context, accountID string, req *dto.UpdateProfileRequest) error
	ActivateCheck(ctx context.Context, workspaceID, email, token string) (map[string]interface{}, bool)
	Activate(ctx context.Context, workspaceID, email, token, name, password, lang, timezone string) (interface{}, error)
	CheckRegisterValidity(ctx context.Context, email, code, token string) (bool, error)
	ValidateResetPasswordToken(token, email, code string) (bool, string, error)
	ResetPasswordWithAutoRegister(token, newPassword string) error
	IsAllowRegister() bool
	AddForgotPasswordErrorRateLimit(email string)
	ResetForgotPasswordErrorRateLimit(email string)
	IsForgotPasswordErrorRateLimit(email string) bool
	ExistsByEmail(ctx context.Context, email string) bool
	LoadUser(ctx context.Context, userID string) (*auth_model.Account, error)
	GetAccountJWTToken(ctx context.Context, account *auth_model.Account) (string, error)
	Authenticate(ctx context.Context, email, password string, inviteToken string) (*auth_model.Account, error)
	UpdateAccountPassword(ctx context.Context, account *auth_model.Account, password, newPassword string) error
	CreateAccountEx(ctx context.Context, account *auth_model.Account, mobile string, gender *auth_model.GenderEnum) (auth_model.JSONMap, error)

	CreateAccountAndTenant(ctx context.Context, email, name, interfaceLanguage string, password *string) (*auth_model.Account, error)
	GenerateAccountDeletionVerificationCode(ctx context.Context, account *auth_model.Account) (string, string, error)
	SendAccountDeletionVerificationEmail(ctx context.Context, account *auth_model.Account, code string) error
	VerifyAccountDeletionCode(ctx context.Context, token, code string) (bool, error)
	LinkAccountIntegrate(ctx context.Context, provider auth_model.AccountIntegrateProvider, openID string, account *auth_model.Account) error
	CloseAccount(ctx context.Context, account *auth_model.Account) error
	UpdateLoginInfo(ctx context.Context, account *auth_model.Account, ipAddress string) error
	LoadLoggedInAccount(ctx context.Context, accountID string) (*auth_model.Account, error)
	RevokeResetPasswordToken(ctx context.Context, token string) error
	GetResetPasswordData(ctx context.Context, token string) (map[string]interface{}, error)
	SendEmailCodeLoginEmail(ctx context.Context, account *auth_model.Account, email, language string) (string, error)
	GetEmailCodeLoginData(ctx context.Context, token string) (map[string]interface{}, error)
	RevokeEmailCodeLoginToken(ctx context.Context, token string) error
	GetUserThroughEmail(ctx context.Context, email string) (*auth_model.Account, error)
	AddLoginErrorRateLimit(ctx context.Context, email string) error
	IsLoginErrorRateLimit(ctx context.Context, email string) (bool, error)
	ResetLoginErrorRateLimit(ctx context.Context, email string) error
	IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error)
	GetAccountsWithExtensions(ctx context.Context, args map[string]interface{}, currentAccount *auth_model.Account) (*dto.PaginationResult, error)
	GetAccountsWithExtensionsByEmail(ctx context.Context, email string) (*auth_model.Account, error)
	GetAccountsWithExtensionsByID(ctx context.Context, id string) (*auth_model.Account, error)
	GetAccountsNotInWorkspace(ctx context.Context, tenantID string, search *string, page, perPage int) (*dto.PaginationResult, error)
	LoginCommon(account *auth_model.Account, ipAddress string) (*auth_model.TokenPair, error)
	Login(ctx context.Context, req *dto.LoginReq) (*auth_model.TokenPair, error, dto.LoginResponse, helper.ErrorResponse)

	GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error)
	GetAccountsByIDs(ctx context.Context, ids []string) (map[string]*auth_model.Account, error)
	GetGroupRole(ctx context.Context, accountID string) (string, error)
	GetOrganizationRoleByWorkspaceID(ctx context.Context, accountID string, tenantID string) (string, error)
	IsGroupOwner(ctx context.Context, accountID string, tenantID string) (bool, error)
	UpsertOrganizationRole(ctx context.Context, tenantID string, accountID string, role string) error

	// CheckOrganizationpAdminByWorkspace is a legacy compatibility API.
	// Prefer IsOrganizationAdminOrOwner when the organization ID is already known.
	CheckOrganizationpAdminByWorkspace(ctx context.Context, accountID, tenantID string) (bool, error)
	CheckTenantAdmin(ctx context.Context, accountID, tenantID string) (bool, error)
	GetCurrentWorkspace(ctx context.Context, accountID string) (*model.Workspace, error)

	EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error)

	UpdateAccountBasicInfo(ctx context.Context, account *auth_model.Account, name, email, status *string) error
	UpdateAccountExtension(ctx context.Context, account *auth_model.Account, mobile *string, gender *string) error

	UpdateAccountEx(ctx context.Context, account *auth_model.Account, req *dto.UpdateAccountExRequest) error

	LoginRefactored(ctx context.Context, req *dto.LoginReq) *dto.LoginResult

	RegisterEx(ctx context.Context, email string, name string, password *string, openID *string, provider *string, language *string, status *auth_model.AccountStatus, isSetup *bool, createWorkspaceRequired *bool) (*auth_model.Account, error)
	JoinByInvitation(ctx context.Context, id string, code string) (*model.Workspace, error)
	InviteMemberEx(ctx context.Context, tenantID, inviterID, email string, role model.WorkspaceMemberRole, language string, name, mobile, gender, position string, sendEmail bool) (string, error)
	InviteMember(ctx context.Context, tenantID, inviterID, email string, role model.WorkspaceMemberRole, language string) (string, error)

	IsEditor(ctx context.Context, accountID string) (bool, error)

	GetAccountContext(ctx context.Context, accountID string) (*auth_model.AccountContext, error)
	UpdateAccountContext(ctx context.Context, accountID string, organizationID, workspaceID *string) (*auth_model.AccountContext, error)

	IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error)
	IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error)
}
