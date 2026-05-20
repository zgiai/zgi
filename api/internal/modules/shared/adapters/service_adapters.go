package adapters

import (
	"context"

	"github.com/zgiai/ginext/internal/dto"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	auth_service "github.com/zgiai/ginext/internal/modules/user/auth/service"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/ginext/internal/modules/workspace/service"
	helper "github.com/zgiai/ginext/internal/util"
	"gorm.io/gorm"
)

// Login implements shared.AccountService interface
func (a *AccountServiceAdapter) Login(ctx context.Context, req *dto.LoginReq) (*auth_model.TokenPair, error, dto.LoginResponse, helper.ErrorResponse) {
	// Call the concrete service method
	tokenPair, err, loginResp, errResp := a.accountService.Login(ctx, req)
	// Type assert the interface{} returns to proper types
	var sharedLoginResp dto.LoginResponse
	var sharedErrResp helper.ErrorResponse
	sharedLoginResp = loginResp
	sharedErrResp = errResp
	return tokenPair, err, sharedLoginResp, sharedErrResp
}

// LoginRefactored implements shared.AccountService interface
func (a *AccountServiceAdapter) LoginRefactored(ctx context.Context, req *dto.LoginReq) *dto.LoginResult {
	serviceResult := a.accountService.LoginRefactored(ctx, req)

	switch serviceResult.GetResponseType() {
	case auth_service.ResponseTypeSuccess:
		return &dto.LoginResult{
			Success:    true,
			ResultType: dto.LoginResultTypeSuccess,
			Data: &dto.LoginResponse{
				AccessToken:  serviceResult.TokenPair.AccessToken,
				RefreshToken: serviceResult.TokenPair.RefreshToken,
				Account:      serviceResult.Account,
			},
		}
	case auth_service.ResponseTypeBusinessError:
		message := "登录失败"
		errorCode := ""
		if serviceResult.Error != nil {
			message = serviceResult.Error.Message
			errorCode = serviceResult.Error.Code
		}
		return &dto.LoginResult{
			Success:    false,
			Message:    message,
			ErrorCode:  errorCode,
			ResultType: dto.LoginResultTypeBusinessError,
		}
	case auth_service.ResponseTypeSpecialFail:
		return &dto.LoginResult{
			Success:     false,
			ResultType:  dto.LoginResultTypeSpecialFail,
			SpecialData: serviceResult.GetSpecialData(),
			SpecialCode: serviceResult.GetSpecialCode(),
		}
	default:
		return &dto.LoginResult{
			Success:    false,
			Message:    "登录失败",
			ResultType: dto.LoginResultTypeBusinessError,
		}
	}
}

// GetAccountByID implements shared.AccountService interface
func (a *AccountServiceAdapter) GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error) {
	return a.accountService.GetAccountByID(ctx, id)
}

// GetAccountByEmail implements shared.AccountService interface
func (a *AccountServiceAdapter) GetAccountByEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return a.accountService.GetAccountByEmail(ctx, email)
}

// Activate implements shared.AccountService interface
func (a *AccountServiceAdapter) Activate(ctx context.Context, workspaceID, email, token, name, password, lang, timezone string) (interface{}, error) {
	return a.accountService.Activate(ctx, workspaceID, email, token, name, password, lang, timezone)
}

// SendResetPasswordEmail implements shared.AccountService interface
func (a *AccountServiceAdapter) SendResetPasswordEmail(ctx context.Context, account *auth_model.Account, email string, language string) (string, error) {
	return a.accountService.SendResetPasswordEmail(ctx, account, email, language)
}

// CreateAccount implements shared.AccountService interface
func (a *AccountServiceAdapter) CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (*auth_model.Account, error) {
	return a.accountService.CreateAccount(ctx, req)
}

// RegisterEx implements shared.AccountService interface
func (a *AccountServiceAdapter) RegisterEx(ctx context.Context, email string, name string, password *string, openID *string, provider *string, language *string, status *auth_model.AccountStatus, isSetup *bool, createWorkspaceRequired *bool) (*auth_model.Account, error) {
	return a.accountService.RegisterEx(ctx, email, name, password, openID, provider, language, status, isSetup, createWorkspaceRequired)
}

// All other required methods with placeholder implementations

// GetAccountExtensionByID implements shared.AccountService interface
func (a *AccountServiceAdapter) GetAccountExtensionByID(ctx context.Context, id string) (auth_model.JSONMap, error) {
	return a.accountService.GetAccountExtensionByID(ctx, id)
}

func (a *AccountServiceAdapter) UpdateAccount(ctx context.Context, id string, req *dto.UpdateAccountRequest) error {
	return a.accountService.UpdateAccount(ctx, id, req)
}

func (a *AccountServiceAdapter) DeleteAccount(ctx context.Context, id string) error {
	return a.accountService.DeleteAccount(ctx, id)
}

func (a *AccountServiceAdapter) DeleteCurrentAccount(ctx context.Context, id, password string) error {
	return a.accountService.DeleteCurrentAccount(ctx, id, password)
}

func (a *AccountServiceAdapter) ChangePassword(ctx context.Context, id string, oldPassword, newPassword string) error {
	return a.accountService.ChangePassword(ctx, id, oldPassword, newPassword)
}

func (a *AccountServiceAdapter) ResetPassword(ctx context.Context, resetToken, newPassword string) error {
	return a.accountService.ResetPassword(ctx, resetToken, newPassword)
}

func (a *AccountServiceAdapter) VerifyAccount(ctx context.Context, token string) error {
	return a.accountService.VerifyAccount(ctx, token)
}

func (a *AccountServiceAdapter) Logout(ctx context.Context, accessToken, refreshToken string) error {
	return a.accountService.Logout(ctx, accessToken, refreshToken)
}

func (a *AccountServiceAdapter) GetAccountProfile(ctx context.Context, accountID string) (*dto.AccountProfileResponse, error) {
	return a.accountService.GetAccountProfile(ctx, accountID)
}

func (a *AccountServiceAdapter) UpdateAccountProfile(ctx context.Context, accountID string, req *dto.UpdateProfileRequest) error {
	return a.accountService.UpdateAccountProfile(ctx, accountID, req)
}

func (a *AccountServiceAdapter) ActivateCheck(ctx context.Context, workspaceID, email, token string) (map[string]interface{}, bool) {
	return a.accountService.ActivateCheck(ctx, workspaceID, email, token)
}

func (a *AccountServiceAdapter) CheckRegisterValidity(ctx context.Context, email, code, token string) (bool, error) {
	return a.accountService.CheckRegisterValidity(ctx, email, code, token)
}

func (a *AccountServiceAdapter) ValidateResetPasswordToken(token, email, code string) (bool, string, error) {
	return a.accountService.ValidateResetPasswordToken(token, email, code)
}

func (a *AccountServiceAdapter) ResetPasswordWithAutoRegister(token, newPassword string) error {
	return a.accountService.ResetPasswordWithAutoRegister(token, newPassword)
}

func (a *AccountServiceAdapter) IsAllowRegister() bool {
	return a.accountService.IsAllowRegister()
}

func (a *AccountServiceAdapter) AddForgotPasswordErrorRateLimit(email string) {
	a.accountService.AddForgotPasswordErrorRateLimit(email)
}

func (a *AccountServiceAdapter) ResetForgotPasswordErrorRateLimit(email string) {
	a.accountService.ResetForgotPasswordErrorRateLimit(email)
}

func (a *AccountServiceAdapter) IsForgotPasswordErrorRateLimit(email string) bool {
	return a.accountService.IsForgotPasswordErrorRateLimit(email)
}

func (a *AccountServiceAdapter) LoadUser(ctx context.Context, userID string) (*auth_model.Account, error) {
	return a.accountService.LoadUser(ctx, userID)
}

func (a *AccountServiceAdapter) GetAccountJWTToken(ctx context.Context, account *auth_model.Account) (string, error) {
	return a.accountService.GetAccountJWTToken(ctx, account)
}

func (a *AccountServiceAdapter) Authenticate(ctx context.Context, email, password string, inviteToken string) (*auth_model.Account, error) {
	return a.accountService.Authenticate(ctx, email, password, inviteToken)
}

func (a *AccountServiceAdapter) UpdateAccountPassword(ctx context.Context, account *auth_model.Account, password, newPassword string) error {
	return a.accountService.UpdateAccountPassword(ctx, account, password, newPassword)
}

func (a *AccountServiceAdapter) CreateAccountEx(ctx context.Context, account *auth_model.Account, mobile string, gender *auth_model.GenderEnum) (auth_model.JSONMap, error) {
	return a.accountService.CreateAccountEx(ctx, account, mobile, gender)
}

func (a *AccountServiceAdapter) CreateAccountAndTenant(ctx context.Context, email, name, interfaceLanguage string, password *string) (*auth_model.Account, error) {
	return a.accountService.CreateAccountAndTenant(ctx, email, name, interfaceLanguage, password)
}

func (a *AccountServiceAdapter) GenerateAccountDeletionVerificationCode(ctx context.Context, account *auth_model.Account) (string, string, error) {
	return a.accountService.GenerateAccountDeletionVerificationCode(ctx, account)
}

func (a *AccountServiceAdapter) SendAccountDeletionVerificationEmail(ctx context.Context, account *auth_model.Account, code string) error {
	return a.accountService.SendAccountDeletionVerificationEmail(ctx, account, code)
}

func (a *AccountServiceAdapter) VerifyAccountDeletionCode(ctx context.Context, token, code string) (bool, error) {
	return a.accountService.VerifyAccountDeletionCode(ctx, token, code)
}

func (a *AccountServiceAdapter) LinkAccountIntegrate(ctx context.Context, provider auth_model.AccountIntegrateProvider, openID string, account *auth_model.Account) error {
	return a.accountService.LinkAccountIntegrate(ctx, provider, openID, account)
}

func (a *AccountServiceAdapter) CloseAccount(ctx context.Context, account *auth_model.Account) error {
	return a.accountService.CloseAccount(ctx, account)
}

func (a *AccountServiceAdapter) ExistsByEmail(ctx context.Context, email string) bool {
	return a.accountService.ExistsByEmail(ctx, email)
}

func (a *AccountServiceAdapter) UpdateLoginInfo(ctx context.Context, account *auth_model.Account, ipAddress string) error {
	return a.accountService.UpdateLoginInfo(account, ipAddress)
}

func (a *AccountServiceAdapter) LoadLoggedInAccount(ctx context.Context, accountID string) (*auth_model.Account, error) {
	return a.accountService.LoadLoggedInAccount(ctx, accountID)
}

func (a *AccountServiceAdapter) RevokeResetPasswordToken(ctx context.Context, token string) error {
	return a.accountService.RevokeResetPasswordToken(ctx, token)
}

func (a *AccountServiceAdapter) SendEmailCodeLoginEmail(ctx context.Context, account *auth_model.Account, email, language string) (string, error) {
	return a.accountService.SendEmailCodeLoginEmail(ctx, account, email, language)
}

func (a *AccountServiceAdapter) GetEmailCodeLoginData(ctx context.Context, token string) (map[string]interface{}, error) {
	return a.accountService.GetEmailCodeLoginData(ctx, token)
}

func (a *AccountServiceAdapter) RevokeEmailCodeLoginToken(ctx context.Context, token string) error {
	return a.accountService.RevokeEmailCodeLoginToken(ctx, token)
}

func (a *AccountServiceAdapter) GetUserThroughEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return a.accountService.GetUserThroughEmail(ctx, email)
}

func (a *AccountServiceAdapter) IssueSSOState(ctx context.Context) (string, error) {
	return a.accountService.IssueSSOState(ctx)
}

func (a *AccountServiceAdapter) ConsumeSSOState(ctx context.Context, state string) error {
	return a.accountService.ConsumeSSOState(ctx, state)
}

func (a *AccountServiceAdapter) ResolveOrCreateSSOAccount(ctx context.Context, identity *dto.SSOIdentity) (*auth_model.Account, error) {
	return a.accountService.ResolveOrCreateSSOAccount(ctx, identity)
}

func (a *AccountServiceAdapter) IssueSSOLoginTicket(ctx context.Context, account *auth_model.Account, sso *dto.SSOProviderToken) (string, error) {
	return a.accountService.IssueSSOLoginTicket(ctx, account, sso)
}

func (a *AccountServiceAdapter) ConsumeSSOLoginTicket(ctx context.Context, ticket, ipAddress string) (*dto.LoginResponse, error) {
	return a.accountService.ConsumeSSOLoginTicket(ctx, ticket, ipAddress)
}

func (a *AccountServiceAdapter) AddLoginErrorRateLimit(ctx context.Context, email string) error {
	return a.accountService.AddLoginErrorRateLimit(ctx, email)
}

func (a *AccountServiceAdapter) IsLoginErrorRateLimit(ctx context.Context, email string) (bool, error) {
	return a.accountService.IsLoginErrorRateLimit(ctx, email)
}

func (a *AccountServiceAdapter) ResetLoginErrorRateLimit(ctx context.Context, email string) error {
	return a.accountService.ResetLoginErrorRateLimit(ctx, email)
}

func (a *AccountServiceAdapter) GetAccountsWithExtensions(ctx context.Context, args map[string]interface{}, currentAccount *auth_model.Account) (*dto.PaginationResult, error) {
	result, err := a.accountService.GetAccountsWithExtensions(ctx, args, currentAccount)
	if err != nil {
		return nil, err
	}
	return &dto.PaginationResult{
		Items:      result.Items,
		Total:      result.Total,
		Page:       result.Page,
		PerPage:    result.PerPage,
		TotalPages: result.TotalPages,
	}, nil
}

func (a *AccountServiceAdapter) GetAccountsNotInWorkspace(ctx context.Context, tenantID string, search *string, page, perPage int) (*dto.PaginationResult, error) {
	result, err := a.accountService.GetAccountsNotInTenant(ctx, tenantID, search, page, perPage)
	if err != nil {
		return nil, err
	}
	// Convert type from auth_service.PaginationResult to dto.PaginationResult
	return &dto.PaginationResult{
		Items:      result.Items,
		Total:      result.Total,
		Page:       result.Page,
		PerPage:    result.PerPage,
		TotalPages: result.TotalPages,
	}, nil
}

func (a *AccountServiceAdapter) GetAccountsWithExtensionsByEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return a.accountService.GetAccountsWithExtensionsByEmail(ctx, email)
}

func (a *AccountServiceAdapter) GetAccountsWithExtensionsByID(ctx context.Context, id string) (*auth_model.Account, error) {
	return a.accountService.GetAccountsWithExtensionsByID(ctx, id)
}

func (a *AccountServiceAdapter) GetAccountsByIDs(ctx context.Context, ids []string) (map[string]*auth_model.Account, error) {
	return a.accountService.GetAccountsByIDs(ctx, ids)
}

func (a *AccountServiceAdapter) GetGroupRole(ctx context.Context, accountID string) (string, error) {
	return a.accountService.GetGroupRole(ctx, accountID)
}

func (a *AccountServiceAdapter) GetOrganizationRoleByWorkspaceID(ctx context.Context, accountID string, tenantID string) (string, error) {
	return a.accountService.GetGroupRoleByTenantID(ctx, accountID, tenantID)
}

func (a *AccountServiceAdapter) IsGroupOwner(ctx context.Context, accountID string, tenantID string) (bool, error) {
	return a.accountService.IsGroupOwner(ctx, accountID, tenantID)
}

func (a *AccountServiceAdapter) UpsertOrganizationRole(ctx context.Context, tenantID string, accountID string, role string) error {
	return a.accountService.UpsertOrganizationRole(ctx, tenantID, accountID, role)
}

// CheckOrganizationpAdminByWorkspace keeps legacy naming for compatibility.
// Prefer IsOrganizationAdminOrOwner when caller already has organization scope ID.
func (a *AccountServiceAdapter) CheckOrganizationpAdminByWorkspace(ctx context.Context, accountID string, tenantID string) (bool, error) {
	return a.accountService.CheckGroupAdminByWorkspace(ctx, accountID, tenantID)
}

func (a *AccountServiceAdapter) CheckTenantAdmin(ctx context.Context, accountID string, tenantID string) (bool, error) {
	return a.accountService.CheckTenantAdmin(ctx, accountID, tenantID)
}

func (a *AccountServiceAdapter) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error) {
	return a.accountService.GetCurrentWorkspace(ctx, accountID)
}

func (a *AccountServiceAdapter) EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error) {
	return a.accountService.EnsureCurrentOrganizationID(ctx, accountID)
}

func (a *AccountServiceAdapter) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	return a.accountService.IsOrganizationMember(ctx, organizationID, accountID)
}

type AccountServiceAdapter struct {
	accountService  *auth_service.AccountService
	registerService interfaces.RegisterService
}

func NewAccountServiceAdapter(accountService *auth_service.AccountService, registerService interfaces.RegisterService) interfaces.AccountService {
	return &AccountServiceAdapter{
		accountService:  accountService,
		registerService: registerService,
	}
}

// InviteMember implements shared.AccountService interface
func (a *AccountServiceAdapter) InviteMember(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string) (string, error) {
	return a.registerService.InviteMember(ctx, tenantID, inviterID, email, role, language)
}

// InviteMemberEx implements shared.AccountService interface
func (a *AccountServiceAdapter) InviteMemberEx(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string, name, mobile, gender, position string, sendEmail bool) (string, error) {
	return a.registerService.InviteMemberEx(ctx, tenantID, inviterID, email, role, language, name, mobile, gender, position, sendEmail)
}

// JoinByInvitation implements shared.AccountService interface
func (a *AccountServiceAdapter) JoinByInvitation(ctx context.Context, id string, code string) (*workspace_model.Workspace, error) {
	return a.registerService.JoinByInvitation(ctx, id, code)
}

// TenantServiceAdapter adapts concrete TenantRepositoryInterface to shared.TenantService interface
type TenantServiceAdapter struct {
	tenantService *workspace_service.WorkspaceManagementServiceImpl
}

// NewTenantServiceAdapter creates a new TenantServiceAdapter
func NewTenantServiceAdapter(tenantService *workspace_service.WorkspaceManagementServiceImpl) interfaces.WorkspaceManagementService {
	return &TenantServiceAdapter{
		tenantService: tenantService,
	}
}

func (t *TenantServiceAdapter) GetWorkspaceByID(ctx context.Context, id string) (*workspace_model.Workspace, error) {
	return t.tenantService.GetWorkspaceByID(ctx, id)
}

func (t *TenantServiceAdapter) CreateWorkspace(ctx context.Context, name string, isFromDashboard bool) (*workspace_model.Workspace, error) {
	return t.tenantService.CreateWorkspace(ctx, name, isFromDashboard)
}

func (t *TenantServiceAdapter) UpdateWorkspace(ctx context.Context, id string, req *interfaces.UpdateWorkspaceRequest) error {
	return t.tenantService.UpdateWorkspace(ctx, id, req)
}

func (t *TenantServiceAdapter) DeleteWorkspace(ctx context.Context, id string) error {
	return t.tenantService.DeleteWorkspace(ctx, id)
}

func (t *TenantServiceAdapter) DeleteWorkspaceWithMembers(ctx context.Context, id string) error {
	return t.tenantService.DeleteWorkspaceWithMembers(ctx, id)
}

func (t *TenantServiceAdapter) AddMember(ctx context.Context, req *interfaces.AddMemberRequest) error {
	return t.tenantService.AddMember(ctx, req)
}

func (t *TenantServiceAdapter) RemoveMember(ctx context.Context, tenantID, accountID string) error {
	return t.tenantService.RemoveMember(ctx, tenantID, accountID)
}

func (t *TenantServiceAdapter) RemoveMemberFromWorkspace(ctx context.Context, tenant *workspace_model.Workspace, member *auth_model.Account, operator *auth_model.Account) error {
	return t.tenantService.RemoveMemberFromWorkspace(ctx, tenant, member, operator)
}

func (t *TenantServiceAdapter) UpdateMemberRole(ctx context.Context, req *interfaces.UpdateMemberRoleRequest) error {
	return t.tenantService.UpdateMemberRole(ctx, req)
}

func (t *TenantServiceAdapter) UpdateMemberRoleWithPermissionCheck(ctx context.Context, tenant *workspace_model.Workspace, member *auth_model.Account, newRole string, operator *auth_model.Account) error {
	return t.tenantService.UpdateMemberRoleWithPermissionCheck(ctx, tenant, member, newRole, operator)
}

func (t *TenantServiceAdapter) UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx context.Context, tenant *workspace_model.Workspace, member *auth_model.Account, newRole string, roleID *string, operator *auth_model.Account) error {
	return t.tenantService.UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx, tenant, member, newRole, roleID, operator)
}

func (t *TenantServiceAdapter) UpdateMemberCustomRoleWithPermissionCheck(ctx context.Context, tenant *workspace_model.Workspace, member *auth_model.Account, roleID string, operator *auth_model.Account) error {
	return t.tenantService.UpdateMemberCustomRoleWithPermissionCheck(ctx, tenant, member, roleID, operator)
}

func (t *TenantServiceAdapter) LeaveWorkspace(ctx context.Context, workspaceID, accountID string) error {
	return t.tenantService.LeaveWorkspace(ctx, workspaceID, accountID)
}

func (t *TenantServiceAdapter) TransferOwner(ctx context.Context, tenantID string, currentOwnerID string, newOwnerID string) error {
	return t.tenantService.TransferOwner(ctx, tenantID, currentOwnerID, newOwnerID)
}

func (t *TenantServiceAdapter) GetWorkspaceMembers(ctx context.Context, tenantID string) ([]*interfaces.AccountWithRole, error) {
	workspaceMembers, err := t.tenantService.GetWorkspaceMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Convert workspace_service.AccountWithRole to interfaces.AccountWithRole
	var interfaceMembers []*interfaces.AccountWithRole
	for _, member := range workspaceMembers {
		interfaceMembers = append(interfaceMembers, &interfaces.AccountWithRole{
			ID:           member.ID,
			Name:         member.Name,
			Avatar:       member.Avatar,
			AvatarURL:    member.AvatarURL,
			Email:        member.Email,
			LastLoginAt:  member.LastLoginAt,
			LastActiveAt: member.LastActiveAt,
			CreatedAt:    member.CreatedAt,
			Role:         member.Role,
			RoleID:       member.RoleID,
			Status:       member.Status,
		})
	}
	return interfaceMembers, nil
}

func (t *TenantServiceAdapter) GetWorkspaceMembersPaginated(ctx context.Context, tenantID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error) {
	return t.tenantService.GetWorkspaceMembersPaginated(ctx, tenantID, page, limit, keyword, roleFilter)
}

func (t *TenantServiceAdapter) GetWorkspaceMembersWithExtensions(ctx context.Context, tenantID string) ([]*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	workspaceMembers, err := t.tenantService.GetWorkspaceMembersWithExtensions(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Convert workspace_service.TenantMemberWithExtensionResponse to interfaces.TenantMemberWithExtensionResponse
	var interfaceMembers []*interfaces.WorkspaceMemberWithExtensionResponse
	for _, member := range workspaceMembers {
		interfaceMembers = append(interfaceMembers, &interfaces.WorkspaceMemberWithExtensionResponse{
			Account:          member.Account,
			Role:             member.Role,
			JoinedAt:         member.JoinedAt,
			Position:         member.Position,
			Permissions:      member.Permissions,
			Extension:        member.Extension,
			OrganizationRole: member.OrganizationRole,
		})
	}
	return interfaceMembers, nil
}

func (t *TenantServiceAdapter) GetWorkspaceMemberWithExtensionsById(ctx context.Context, tenantID, memberID string) (*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	workspaceMember, err := t.tenantService.GetWorkspaceMemberWithExtensionsById(ctx, tenantID, memberID)
	if err != nil {
		return nil, err
	}

	// Convert workspace_service.TenantMemberWithExtensionResponse to interfaces.TenantMemberWithExtensionResponse
	return &interfaces.WorkspaceMemberWithExtensionResponse{
		Account:          workspaceMember.Account,
		Role:             workspaceMember.Role,
		JoinedAt:         workspaceMember.JoinedAt,
		Position:         workspaceMember.Position,
		Permissions:      workspaceMember.Permissions,
		Extension:        workspaceMember.Extension,
		OrganizationRole: workspaceMember.OrganizationRole,
	}, nil
}

func (t *TenantServiceAdapter) GetDatasetOperatorMembers(ctx context.Context, tenantID string) ([]*interfaces.AccountWithRole, error) {
	workspaceMembers, err := t.tenantService.GetDatasetOperatorMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Convert workspace_service.AccountWithRole to interfaces.AccountWithRole
	var interfaceMembers []*interfaces.AccountWithRole
	for _, member := range workspaceMembers {
		interfaceMembers = append(interfaceMembers, &interfaces.AccountWithRole{
			ID:           member.ID,
			Name:         member.Name,
			Avatar:       member.Avatar,
			AvatarURL:    member.AvatarURL,
			Email:        member.Email,
			LastLoginAt:  member.LastLoginAt,
			LastActiveAt: member.LastActiveAt,
			CreatedAt:    member.CreatedAt,
			Role:         member.Role,
			RoleID:       member.RoleID,
			Status:       member.Status,
		})
	}
	return interfaceMembers, nil
}

func (t *TenantServiceAdapter) CreateWorkspaceMember(ctx context.Context, tenantID string, accountID string, role string) error {
	return t.tenantService.CreateWorkspaceMember(ctx, tenantID, accountID, role)
}

func (t *TenantServiceAdapter) CheckPermission(ctx context.Context, tenantID, accountID string) bool {
	return t.tenantService.CheckPermission(ctx, tenantID, accountID)
}

func (t *TenantServiceAdapter) CheckMemberPermission(ctx context.Context, tenant *workspace_model.Workspace, operator *auth_model.Account, member *auth_model.Account, action string) error {
	return t.tenantService.CheckMemberPermission(ctx, tenant, operator, member, action)
}

func (t *TenantServiceAdapter) ChangeWorkspaceWithJoin(ctx context.Context, member *auth_model.Account, fromTenantID, toTenantID string, operator *auth_model.Account) error {
	return t.tenantService.ChangeWorkspaceWithJoin(ctx, member, fromTenantID, toTenantID, operator)
}

func (t *TenantServiceAdapter) UpdateMemberRoleExtensions(ctx context.Context, tenant *workspace_model.Workspace, member *auth_model.Account, newRole, newPosition *string, newPermissions []string, operator *auth_model.Account) error {
	return t.tenantService.UpdateMemberRoleExtensions(ctx, tenant, member, newRole, newPosition, newPermissions, operator)
}

func (t *TenantServiceAdapter) CreateMemberExtension(ctx context.Context, req *interfaces.CreateMemberExtensionRequest) error {
	return t.tenantService.CreateMemberExtension(ctx, req)
}

func (t *TenantServiceAdapter) UpdateMemberExtension(ctx context.Context, req *interfaces.UpdateMemberExtensionRequest) error {
	return t.tenantService.UpdateMemberExtension(ctx, req)
}

func (t *TenantServiceAdapter) GetAccountWorkspaces(ctx context.Context, accountID string) ([]*workspace_model.Workspace, error) {
	account := &auth_model.Account{ID: accountID}
	return t.tenantService.GetJoinWorkspaces(ctx, account)
}

func (t *TenantServiceAdapter) SwitchWorkspace(ctx context.Context, accountID, tenantID string) error {
	return t.tenantService.SwitchWorkspace(ctx, accountID, tenantID)
}

func (t *TenantServiceAdapter) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.WorkspaceMember, error) {
	return t.tenantService.GetCurrentWorkspace(ctx, accountID)
}

func (t *TenantServiceAdapter) GetCurrentOrganization(ctx context.Context, memberID string) (*workspace_model.OrganizationMember, error) {
	return t.tenantService.GetCurrentOrganization(ctx, memberID)
}

func (t *TenantServiceAdapter) GetUserWorkspaceMemberships(ctx context.Context, accountID string) ([]interfaces.WorkspaceMembership, error) {
	return t.tenantService.GetUserWorkspaceMemberships(ctx, accountID)
}

func (t *TenantServiceAdapter) GetUserRole(ctx context.Context, accountID, tenantID string) (*workspace_model.WorkspaceMemberRole, error) {
	return t.tenantService.GetUserRole(ctx, accountID, tenantID)
}

func (t *TenantServiceAdapter) GetJoinWorkspaces(ctx context.Context, account *auth_model.Account) ([]*workspace_model.Workspace, error) {
	return t.tenantService.GetJoinWorkspaces(ctx, account)
}

func (t *TenantServiceAdapter) GetAccountWorkspaceJoins(ctx context.Context, accountID string) ([]*workspace_model.WorkspaceMember, error) {
	return t.tenantService.GetAccountWorkspaceJoins(ctx, accountID)
}

func (t *TenantServiceAdapter) GetWorkspaceAccountJoins(ctx context.Context, tenantID string) ([]*workspace_model.WorkspaceMember, error) {
	return t.tenantService.GetWorkspaceAccountJoins(ctx, tenantID)
}

func (t *TenantServiceAdapter) GetWorkspacesByIDs(ctx context.Context, tenantIDs []string) ([]*workspace_model.Workspace, error) {
	return t.tenantService.GetWorkspacesByIDs(ctx, tenantIDs)
}

func (t *TenantServiceAdapter) GetAccessibleWorkspaceIDs(ctx context.Context, accountID string) ([]string, error) {
	return t.tenantService.GetAccessibleWorkspaceIDs(ctx, accountID)
}

func (t *TenantServiceAdapter) WithTx(tx *gorm.DB) interfaces.WorkspaceManagementService {
	if impl, ok := t.tenantService.WithTx(tx).(*workspace_service.WorkspaceManagementServiceImpl); ok {
		return NewTenantServiceAdapter(impl)
	}
	return t
}

// UpdateAccountBasicInfo implements shared.AccountService interface
func (a *AccountServiceAdapter) UpdateAccountBasicInfo(ctx context.Context, account *auth_model.Account, name, email, status *string) error {
	return a.accountService.UpdateAccountBasicInfo(ctx, account, name, email, status)
}

// UpdateAccountEx implements shared.AccountService interface
func (a *AccountServiceAdapter) UpdateAccountEx(ctx context.Context, account *auth_model.Account, req *dto.UpdateAccountExRequest) error {
	return a.accountService.UpdateAccountEx(ctx, account, req)
}

// UpdateAccountExtension implements shared.AccountService interface
func (a *AccountServiceAdapter) UpdateAccountExtension(ctx context.Context, account *auth_model.Account, mobile *string, gender *string) error {
	return a.accountService.UpdateAccountExtension(ctx, account, mobile, gender)
}

func (t *TenantServiceAdapter) GetByWorkspaceAndMember(ctx context.Context, tenantID, accountID string) (*workspace_model.WorkspaceMember, error) {
	return t.tenantService.GetByWorkspaceAndMember(ctx, tenantID, accountID)
}

// GetResetPasswordData implements shared.AccountService interface
func (a *AccountServiceAdapter) GetResetPasswordData(ctx context.Context, token string) (map[string]interface{}, error) {
	return a.accountService.GetResetPasswordData(ctx, token)
}

func (a *AccountServiceAdapter) GetAccountContext(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	return a.accountService.GetAccountContext(ctx, accountID)
}

func (a *AccountServiceAdapter) UpdateAccountContext(ctx context.Context, accountID string, organizationID, workspaceID *string) (*auth_model.AccountContext, error) {
	return a.accountService.UpdateAccountContext(ctx, accountID, organizationID, workspaceID)
}

func (a *AccountServiceAdapter) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	return a.accountService.IsOrganizationAdminOrOwner(ctx, organizationID, accountID)
}

// IsEmailSendIPLimit checks if the IP address has reached the email sending limit
func (a *AccountServiceAdapter) IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error) {
	return a.accountService.IsEmailSendIPLimit(ctx, ipAddress)
}

func (a *AccountServiceAdapter) SendDirectAddMemberEmail(ctx context.Context, account *auth_model.Account, groupID, groupName, departmentName, language string) error {
	return a.accountService.SendDirectAddMemberEmail(ctx, account, groupID, groupName, departmentName, language)
}

// RefreshToken implements shared.AccountService interface
func (a *AccountServiceAdapter) RefreshToken(ctx context.Context, refreshToken string) (*dto.TokenResponse, error) {
	return a.accountService.RefreshToken(ctx, refreshToken)
}

// LoginCommon implements shared.AccountService interface
func (a *AccountServiceAdapter) LoginCommon(account *auth_model.Account, ipAddress string) (*auth_model.TokenPair, error) {
	return a.accountService.LoginCommon(account, ipAddress)
}

func (a *AccountServiceAdapter) IsEditor(ctx context.Context, accountID string) (bool, error) {
	return a.accountService.IsEditor(ctx, accountID)
}

func (a *AccountServiceAdapter) SetRegisterService(registerService interfaces.RegisterService) {
	a.registerService = registerService
}

// GetWorkspaceIDsByOrganizationID implements shared.TenantService interface
func (t *TenantServiceAdapter) GetWorkspaceIDsByOrganizationID(ctx context.Context, organizationID string) ([]string, error) {
	return t.tenantService.GetWorkspaceIDsByOrganizationID(ctx, organizationID)
}
