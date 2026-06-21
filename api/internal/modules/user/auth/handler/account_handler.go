package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/config"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"

	"github.com/gin-gonic/gin"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"

	"strconv"
	"strings"

	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// Error definitions
var ErrAccountNotFoundAndAllowRegister = errors.New("account not found but allow register")

// Local DTO types removed - using shared_dto.UpdateProfileRequest

// Response types
type ResponseType int

const (
	ResponseTypeSuccess ResponseType = iota
	ResponseTypeBusinessError
	ResponseTypeSpecialFail
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type LoginError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LoginResult struct {
	Success bool `json:"success"`

	Data        interface{}  `json:"data,omitempty"`
	Error       interface{}  `json:"error,omitempty"`
	Type        ResponseType `json:"-"`
	TokenPair   *TokenPair   `json:"-"`
	Account     interface{}  `json:"-"`
	LoginError  *LoginError  `json:"-"`
	SpecialData interface{}  `json:"-"`
	SpecialCode string       `json:"-"`
}

type accountContextMode string

const (
	accountContextModeNone         accountContextMode = "none"
	accountContextModeOrganization accountContextMode = "organization"
	accountContextModeWorkspace    accountContextMode = "workspace"
)

type updateAccountContextRequest struct {
	Mode                  string  `json:"mode"`
	CurrentGroupID        *string `json:"current_group_id"`
	CurrentTeamID         *string `json:"current_team_id"`
	CurrentOrganizationID *string `json:"current_organization_id"`
	CurrentWorkspaceID    *string `json:"current_workspace_id"`
}

type accountContextResponse struct {
	AccountID             string             `json:"account_id"`
	Mode                  accountContextMode `json:"mode"`
	CurrentOrganizationID *string            `json:"current_organization_id"`
	CurrentWorkspaceID    *string            `json:"current_workspace_id"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// Methods for LoginResult
func (r *LoginResult) GetResponseType() ResponseType {
	return r.Type
}

func (r *LoginResult) GetError() *LoginError {
	return r.LoginError
}

func (r *LoginResult) GetSpecialData() interface{} {
	return r.SpecialData
}

func (r *LoginResult) GetSpecialCode() string {
	return r.SpecialCode
}

func (r *LoginResult) GetTokenPair() *TokenPair {
	return r.TokenPair
}

type AccountHandler struct {
	accountService interfaces.AccountService
	tenantService  interfaces.WorkspaceManagementService
}

func NewAccountHandler(accountService interfaces.AccountService, tenantService interfaces.WorkspaceManagementService) *AccountHandler {
	return &AccountHandler{
		accountService: accountService,
		tenantService:  tenantService,
	}
}

func (h *AccountHandler) GetProfile(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	profile, err := h.accountService.GetAccountProfile(c.Request.Context(), accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Fail(c, response.ErrGetUserInfoFailed)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, profile)
}

func (h *AccountHandler) GetAccountContext(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	ctxModel, err := h.accountService.GetAccountContext(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, newAccountContextResponse(ctxModel))
}

func (h *AccountHandler) GetAccountCapabilities(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	capabilities, err := h.accountService.GetAccountCapabilities(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, capabilities)
}

func (h *AccountHandler) UpdateAccountContext(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req updateAccountContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Compatibility logic: use new fields if provided, otherwise fallback to old fields
	organizationID := req.CurrentOrganizationID
	if organizationID == nil {
		organizationID = req.CurrentGroupID
	}

	workspaceID := req.CurrentWorkspaceID
	if workspaceID == nil {
		workspaceID = req.CurrentTeamID
	}

	switch accountContextMode(strings.TrimSpace(req.Mode)) {
	case "":
	case accountContextModeOrganization:
		if organizationID == nil || strings.TrimSpace(*organizationID) == "" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		emptyWorkspaceID := ""
		workspaceID = &emptyWorkspaceID
	case accountContextModeWorkspace:
		if workspaceID == nil || strings.TrimSpace(*workspaceID) == "" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	case accountContextModeNone:
		emptyOrganizationID := ""
		emptyWorkspaceID := ""
		organizationID = &emptyOrganizationID
		workspaceID = &emptyWorkspaceID
	default:
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	ctxModel, err := h.accountService.UpdateAccountContext(c.Request.Context(), accountID, organizationID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, newAccountContextResponse(ctxModel))
}

func newAccountContextResponse(ctxModel *auth_model.AccountContext) accountContextResponse {
	if ctxModel == nil {
		return accountContextResponse{Mode: accountContextModeNone}
	}

	return accountContextResponse{
		AccountID:             ctxModel.AccountID,
		Mode:                  accountContextModeFromModel(ctxModel),
		CurrentOrganizationID: ctxModel.CurrentOrganizationID,
		CurrentWorkspaceID:    ctxModel.CurrentWorkspaceID,
		CreatedAt:             ctxModel.CreatedAt,
		UpdatedAt:             ctxModel.UpdatedAt,
	}
}

func accountContextModeFromModel(ctxModel *auth_model.AccountContext) accountContextMode {
	if ctxModel == nil {
		return accountContextModeNone
	}
	if ctxModel.CurrentWorkspaceID != nil && strings.TrimSpace(*ctxModel.CurrentWorkspaceID) != "" {
		return accountContextModeWorkspace
	}
	if ctxModel.CurrentOrganizationID != nil && strings.TrimSpace(*ctxModel.CurrentOrganizationID) != "" {
		return accountContextModeOrganization
	}
	return accountContextModeNone
}

func (h *AccountHandler) GetProfileEx(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	profile, err := h.accountService.GetAccountProfile(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, profile)
}

func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req shared_dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Use the service method to update profile
	err := h.accountService.UpdateAccountProfile(c.Request.Context(), accountID, &req)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

func (h *AccountHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	err := h.accountService.ResetPassword(c.Request.Context(), req.Token, req.NewPassword)
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

func (h *AccountHandler) ChangePassword(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	err := h.accountService.ChangePassword(c.Request.Context(), accountID, req.OldPassword, req.NewPassword)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

func (h *AccountHandler) Logout(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req struct {
		AccessToken  string `json:"access_token" binding:"required"`
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	err := h.accountService.Logout(c.Request.Context(), req.AccessToken, req.RefreshToken)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

func (h *AccountHandler) RegisterRoutes(router *gin.RouterGroup) {

	// Route group requiring JWT authentication
	accountAuth := router.Group("/account", middleware.JWT())
	{
		accountAuth.POST("/logout", h.Logout)
		accountAuth.GET("/context", h.GetAccountContext)
		accountAuth.PUT("/context", h.UpdateAccountContext)
		accountAuth.GET("/capabilities", h.GetAccountCapabilities)
		accountAuth.GET("/profile", h.GetProfile)
		accountAuth.PUT("/profile", h.UpdateProfile)
		accountAuth.POST("/change-password", h.ChangePassword)
		accountAuth.POST("/reset-password", h.ResetPassword)
		accountAuth.GET("/list", h.GetAccountExList)     // GET account-ex/list
		accountAuth.GET("/email", h.GetAccountExByEmail) // GET account-ex/email
		accountAuth.GET("/id", h.GetAccountExByID)       // GET account-ex/id
		accountAuth.PUT("/id", h.UpdateAccountExByID)    // PUT account-ex/id
		accountAuth.POST("/id", h.UpdateAccountExByID)
		accountAuth.DELETE("/", h.DeleteAccount)
		accountAuth.GET("/integrations", h.GetIntegrations)
		accountAuth.POST("/integrations", h.CreateIntegration)
		accountAuth.DELETE("/integrations/:id", h.DeleteIntegration)
		accountAuth.POST("/interface-language", h.UpdateInterfaceLanguage) // Interface language update
	}

	// Account extension related routes - add by priority list
	accountEx := router.Group("/account-ex", middleware.JWT()) // Add JWT middleware at route group level
	{
		accountEx.GET("/profile", h.GetProfileEx)      // GET profile-ex
		accountEx.GET("/list", h.GetAccountExList)     // GET account-ex/list
		accountEx.GET("/email", h.GetAccountExByEmail) // GET account-ex/email
		accountEx.GET("/id", h.GetAccountExByID)       // GET account-ex/id
		accountEx.PUT("/id", h.UpdateAccountExByID)    // PUT account-ex/id
		accountEx.POST("/id", h.UpdateAccountExByID)
	}
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	err := h.accountService.DeleteCurrentAccount(c.Request.Context(), accountID, req.Password)
	if err != nil {
		if errors.Is(err, auth_service.ErrCurrentPasswordMismatch) {
			response.Fail(c, response.ErrPasswordMismatch)
			return
		}
		response.Fail(c, response.ErrAccountDeleteFailed)
		return
	}

	response.Success(c, nil)
}

func (h *AccountHandler) GetIntegrations(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	integrations := []interface{}{}

	response.Success(c, gin.H{
		"integrations": integrations,
	})
}

func (h *AccountHandler) CreateIntegration(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req struct {
		Provider string `json:"provider" binding:"required"`
		OpenID   string `json:"open_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	response.Success(c, gin.H{
		"message": "Integration created successfully",
	})
}

func (h *AccountHandler) DeleteIntegration(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	integrationID := c.Param("id")
	if integrationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	response.Success(c, gin.H{
		"message": "Integration deleted successfully",
	})
}

type ForgotPasswordSendEmailRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Language string `json:"language"`
}

type ForgotPasswordCheckRequest struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code" binding:"required"`
	Token string `json:"token" binding:"required"`
}

type ForgotPasswordResetRequest struct {
	Token           string `json:"token" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
	PasswordConfirm string `json:"password_confirm" binding:"required,min=8"`
}

func (h *AccountHandler) ForgotPasswordSendEmail(c *gin.Context) {
	var req ForgotPasswordSendEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrEmailFormat)
		return
	}

	ip := c.ClientIP()
	isLimit, err := h.accountService.IsEmailSendIPLimit(c.Request.Context(), ip)
	if err != nil || isLimit {
		response.Fail(c, response.ErrRateLimitExceeded)
		return
	}

	if !h.accountService.ExistsByEmail(c.Request.Context(), req.Email) {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	token, err := h.accountService.SendResetPasswordEmail(context.Background(), nil, req.Email, req.Language)
	if err != nil {
		if err == ErrAccountNotFoundAndAllowRegister {
			response.Fail(c, response.ErrUserNotFound)
			return
		}
		if isResetPasswordEmailRateLimitError(err) {
			response.Fail(c, response.ErrRateLimitExceeded)
			return
		}
		response.Fail(c, response.ErrEmailSendFailed)
		return
	}

	response.Success(c, gin.H{"result": "success", "data": token})
}

func (h *AccountHandler) ForgotPasswordCheck(c *gin.Context) {
	var req ForgotPasswordCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if h.accountService.IsForgotPasswordErrorRateLimit(req.Email) {
		response.Fail(c, response.ErrRateLimitExceeded)
		return
	}

	isValid, email, err := h.accountService.ValidateResetPasswordToken(req.Token, req.Email, req.Code)
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	response.Success(c, gin.H{"is_valid": isValid, "email": email})
}

func (h *AccountHandler) ForgotPasswordReset(c *gin.Context) {
	var req ForgotPasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.NewPassword != req.PasswordConfirm {
		response.Fail(c, response.ErrPasswordMismatch)
		return
	}

	err := h.accountService.ResetPasswordWithAutoRegister(req.Token, req.NewPassword)
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *AccountHandler) ActivateCheck(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	email := c.Query("email")
	token := c.Query("token")
	// Call service layer validation
	data, isValid := h.accountService.ActivateCheck(c.Request.Context(), workspaceID, email, token)
	response.Success(c, gin.H{
		"is_valid": isValid,
		"data":     data,
	})
}

func (h *AccountHandler) Activate(c *gin.Context) {
	var req struct {
		WorkspaceID       string `json:"workspace_id"`
		Email             string `json:"email"`
		Token             string `json:"token" binding:"required"`
		Name              string `json:"name" binding:"required"`
		Password          string `json:"password" binding:"required"`
		InterfaceLanguage string `json:"interface_language" binding:"required"`
		Timezone          string `json:"timezone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	// Call service layer activation
	result, err := h.accountService.Activate(c.Request.Context(), req.WorkspaceID, req.Email, req.Token, req.Name, req.Password, req.InterfaceLanguage, req.Timezone)
	if err != nil {
		response.Fail(c, response.ErrAccountActivateFailed)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) RegisterValidity(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Code  string `json:"code" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	isValid, err := h.accountService.CheckRegisterValidity(c.Request.Context(), req.Email, req.Code, req.Token)
	if err != nil {
		response.Fail(c, response.ErrAccountCheckFailed)
		return
	}
	response.Success(c, gin.H{"is_valid": isValid, "email": req.Email})
}

func (h *AccountHandler) GetAccountExList(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Parse query parameters
	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Get current user
	currentAccount, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrUserNotFound)
		return
	}

	args := map[string]interface{}{
		"page":  page,
		"limit": limit,
	}

	result, err := h.accountService.GetAccountsWithExtensions(c.Request.Context(), args, currentAccount)
	if err != nil {
		response.Fail(c, response.ErrAccountListFailed)
		return
	}

	response.Success(c, result)
}

func (h *AccountHandler) GetAccountExByEmail(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		response.Fail(c, response.ErrEmailFormat)
		return
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get target account
	account, err := h.accountService.GetAccountsWithExtensionsByEmail(c.Request.Context(), email)
	if err != nil {
		// If account doesn't exist, return empty object {}
		c.JSON(200, gin.H{})
		return
	}

	response.Success(c, account)
}

func (h *AccountHandler) GetAccountExByID(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get target account
	account, err := h.accountService.GetAccountsWithExtensionsByID(c.Request.Context(), id)
	if err != nil {
		// If account doesn't exist, return empty object instead of error
		response.Success(c, map[string]interface{}{})
		return
	}

	// Get current tenant association
	currentTenantJoin, err := h.tenantService.GetCurrentWorkspace(c.Request.Context(), currentAccountID)
	if err != nil || currentTenantJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// Get group role
	// TODO: Fix TenantAccountJoin reference - temporarily use string conversion
	// groupRole, err := h.accountService.GetGroupRoleByTenantID(c.Request.Context(), account.ID, currentTenantJoin.TenantID)
	groupRole := "member" // temporary default value
	err = nil
	if err != nil {
		groupRole = "normal" // If retrieval fails, use default role
	}
	account.GroupRole = groupRole

	response.Success(c, account)
}

func (h *AccountHandler) UpdateAccountExByID(c *gin.Context) {
	// Get id from query parameter, if not provided, use current user's ID
	id := c.Query("id")
	if id == "" {
		// Use current user's ID if not specified
		id = c.GetString("account_id")
		if id == "" {
			response.Fail(c, response.ErrUnauthorized)
			return
		}
	}

	var req struct {
		Name      *string `json:"name"`
		Avatar    *string `json:"avatar"`
		Status    *string `json:"status"`
		Mobile    *string `json:"mobile"`
		Wechat    *string `json:"wechat"`
		Address   *string `json:"address"`
		GroupRole *string `json:"group_role"`
		Gender    *string `json:"gender"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get target account
	account, err := h.accountService.GetAccountsWithExtensionsByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	// Get current user
	currentUserID := c.GetString("account_id")
	_, err = h.accountService.GetAccountByID(c.Request.Context(), currentUserID)
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Update basic account information
	if req.Name != nil || req.Status != nil {
		updateReq := &shared_dto.UpdateAccountRequest{}
		if req.Name != nil {
			updateReq.Name = *req.Name
		}
		if req.Status != nil {
			// Validate status value
			validStatuses := []auth_model.AccountStatus{auth_model.AccountStatusActive, auth_model.AccountStatusBanned, auth_model.AccountStatusClosed}
			isValidStatus := false
			for _, status := range validStatuses {
				if string(status) == *req.Status {
					isValidStatus = true
					updateReq.Status = auth_model.AccountStatus(*req.Status)
					break
				}
			}
			if !isValidStatus {
				response.Fail(c, response.ErrInvalidStatus)
				return
			}
		}
		err = h.accountService.UpdateAccount(c.Request.Context(), id, updateReq)
		if err != nil {
			response.Fail(c, response.ErrAccountUpdate)
			return
		}
	}

	// Update extension information
	if req.Mobile != nil || req.Gender != nil || req.Wechat != nil || req.Address != nil {
		updateExReq := &shared_dto.UpdateAccountExRequest{}
		if req.Mobile != nil {
			updateExReq.Mobile = *req.Mobile
		}
		if req.Gender != nil {
			// Convert string to GenderEnum
			genderEnum := auth_model.GenderEnum(*req.Gender)
			updateExReq.Gender = &genderEnum
		}
		if req.Wechat != nil {
			updateExReq.Wechat = *req.Wechat
		}
		if req.Address != nil {
			updateExReq.Address = *req.Address
		}
		err = h.accountService.UpdateAccountEx(c.Request.Context(), account, updateExReq)
		if err != nil {
			response.Fail(c, response.ErrAccountUpdate)
			return
		}
	}

	// Update enterprise group role
	if req.GroupRole != nil {
		// Get current tenant association
		currentTenantJoin, err := h.tenantService.GetCurrentWorkspace(c.Request.Context(), currentUserID)
		if err != nil || currentTenantJoin == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		isGroupOwner := true
		if !isGroupOwner {
			response.Fail(c, response.ErrGroupOwnerRequired)
			return
		}

		// TODO: Fix TenantAccountJoin reference - temporarily skip
		// err = h.accountService.UpsertOrganizationRole(c.Request.Context(), currentTenantJoin.TenantID, id, *req.GroupRole)
		err = nil // temporary skip
		if err != nil {
			response.Fail(c, response.ErrAccountUpdate)
			return
		}
	}

	// Return updated account information
	updatedAccount, err := h.accountService.GetAccountsWithExtensionsByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, response.ErrAccountGetFailed)
		return
	}

	response.Success(c, updatedAccount)
}

func (h *AccountHandler) UpdateInterfaceLanguage(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req struct {
		InterfaceLanguage string `json:"interface_language" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Use the service method to update profile
	updateReq := &shared_dto.UpdateProfileRequest{
		Language: &req.InterfaceLanguage,
	}
	err := h.accountService.UpdateAccountProfile(c.Request.Context(), accountID, updateReq)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Get updated profile
	profile, err := h.accountService.GetAccountProfile(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, profile)
}

type ActivateHandler struct {
	accountService interfaces.AccountService
	//registerService *RegisterService
}

func NewActivateHandler(accountService interfaces.AccountService) *ActivateHandler {
	return &ActivateHandler{
		accountService: accountService,
		//registerService: registerService,
	}
}

func (h *ActivateHandler) Check(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	email := c.Query("email")
	token := c.Query("token")

	result, isValid := h.accountService.ActivateCheck(context.Background(), workspaceID, email, token)
	if !isValid {
		response.Success(c, gin.H{
			"is_valid": false,
		})
		return
	}

	response.Success(c, result)
}

func (h *ActivateHandler) Activate(c *gin.Context) {
	var req struct {
		WorkspaceID       string `json:"workspace_id" binding:"omitempty"`
		Email             string `json:"email" binding:"omitempty,email"`
		Token             string `json:"token" binding:"required"`
		Name              string `json:"name" binding:"required,max=30"`
		InterfaceLanguage string `json:"interface_language" binding:"required"`
		Timezone          string `json:"timezone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Activate account
	account, err := h.accountService.Activate(context.Background(), req.WorkspaceID, req.Email, req.Token, req.Name, "", req.InterfaceLanguage, req.Timezone)
	if err != nil {
		// Check for specific activation error
		if err.Error() == "Auth Token is invalid or account already activated, please check again." {
			response.Fail(c, response.ErrTokenInvalid)
			return
		}
		response.Fail(c, response.ErrAccountActivateFailed)
		return
	}

	// Convert to Account type
	activatedAccount, ok := account.(*auth_model.Account)
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Generate login token
	// Get client IP address
	clientIP := c.ClientIP()

	// Call AccountService.LoginCommon method to generate login token
	tokenPair, err := h.accountService.LoginCommon(activatedAccount, clientIP)
	if err != nil {
		response.Fail(c, response.ErrTokenGenerateFailed)
		return
	}

	getStringValue := func(ptr *string) string {
		if ptr != nil {
			return *ptr
		}
		return ""
	}

	// Build complete login response
	loginResponse := gin.H{
		"result": "success",
		"data": gin.H{
			"access_token":  tokenPair.AccessToken,
			"refresh_token": tokenPair.RefreshToken,
			"account": gin.H{
				"id":                 activatedAccount.ID,
				"name":               activatedAccount.Name,
				"email":              activatedAccount.Email,
				"avatar":             getStringValue(activatedAccount.Avatar),
				"interface_language": getStringValue(activatedAccount.InterfaceLanguage),
				"timezone":           getStringValue(activatedAccount.Timezone),
				"status":             string(activatedAccount.Status),
			},
		},
	}

	response.Success(c, loginResponse)
}

func (h *ActivateHandler) RegisterRoutes(v1 *gin.RouterGroup) {
	activate := v1.Group("/activate")
	{
		activate.GET("/check", h.Check)
		activate.POST("", h.Activate)
	}
}

type ForgotPasswordHandler struct {
	accountService interfaces.AccountService
}

func NewForgotPasswordHandler(accountService interfaces.AccountService) *ForgotPasswordHandler {
	return &ForgotPasswordHandler{
		accountService: accountService,
	}
}

func (h *ForgotPasswordHandler) SendEmail(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Language string `json:"language"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Check IP rate limiting
	ipAddress := c.ClientIP()
	if limit, err := h.accountService.IsEmailSendIPLimit(context.Background(), ipAddress); err != nil || limit {
		response.Fail(c, response.ErrRateLimitExceeded)
		return
	}

	// Determine language
	language := req.Language
	if language == "" {
		language = "en-US" // default language
	}

	if !h.accountService.ExistsByEmail(c.Request.Context(), req.Email) {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	// Send password reset email
	token, err := h.accountService.SendResetPasswordEmail(context.Background(), nil, req.Email, language)
	if err != nil {
		if err.Error() == "Account not found" {
			response.Success(c, gin.H{
				"result": "fail",
				"data":   token,
				"code":   "account_not_found",
			})
			return
		}
		if isResetPasswordEmailRateLimitError(err) {
			response.Fail(c, response.ErrRateLimitExceeded)
			return
		}
		response.Fail(c, response.ErrEmailSendFailed)
		return
	}

	response.Success(c, gin.H{
		"result": "success",
		"data":   token,
	})
}

func (h *ForgotPasswordHandler) Check(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Check frequency limit
	if h.accountService.IsForgotPasswordErrorRateLimit(req.Email) {
		response.Fail(c, response.ErrRateLimitExceeded)
		return
	}

	// Validate token and verification code
	isValid, email, err := h.accountService.ValidateResetPasswordToken(req.Token, req.Email, req.Code)
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	response.Success(c, gin.H{
		"is_valid": isValid,
		"email":    email,
	})
}

func (h *ForgotPasswordHandler) Reset(c *gin.Context) {
	var req struct {
		Token           string `json:"token" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=6"`
		PasswordConfirm string `json:"password_confirm" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, fmt.Sprintf("参数校验失败: %s", err.Error()))
		return
	}

	// Password confirmation
	if req.NewPassword != req.PasswordConfirm {
		response.Fail(c, response.ErrPasswordMismatch)
		return
	}

	// Reset password
	err := h.accountService.ResetPassword(context.Background(), req.Token, req.NewPassword)
	if err != nil {
		if err.Error() == "Account is frozen" {
			response.Fail(c, response.ErrAccountFrozen)
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{
		"result": "success",
	})
}

func (h *ForgotPasswordHandler) RegisterRoutes(v1 *gin.RouterGroup) {
	forgotPassword := v1.Group("/forgot-password")
	{
		forgotPassword.POST("", h.SendEmail)
		forgotPassword.POST("/validity", h.Check)
		forgotPassword.POST("/resets", h.Reset)
	}
}

//func determineLanguage(reqLanguage string) string {
//	if reqLanguage == "zh-Hans" {
//		return "zh-Hans"
//	}
//	return "en-US"
//}

type AuthHandler struct {
	accountService interfaces.AccountService
	featureService interfaces.FeatureService
	tokenManager   *helper.TokenManager
	ssoService     ssoService
	casdoorClient  casdoorOIDCClient
}

func NewAuthHandler(accountService interfaces.AccountService, featureService interfaces.FeatureService, tokenManager *helper.TokenManager) *AuthHandler {
	var ssoSvc ssoService
	if svc, ok := accountService.(ssoService); ok {
		ssoSvc = svc
	}

	var casdoorClient casdoorOIDCClient
	if client, err := auth_service.NewCasdoorOIDCClientFromEnv(); err == nil {
		casdoorClient = client
	}

	return &AuthHandler{
		accountService: accountService,
		featureService: featureService,
		tokenManager:   tokenManager,
		ssoService:     ssoSvc,
		casdoorClient:  casdoorClient,
	}
}

type LoginRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	RememberMe  bool   `json:"remember_me"`
	InviteToken string `json:"invite_token"`
	Language    string `json:"language" default:"en-US"`
}

type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required,min=6"`
	Name           string `json:"name" binding:"required,min=2"`
	InvitationCode string `json:"invitation_code,omitempty"`
	Language       string `json:"language,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
}

//type LoginResponse struct {
//	AccessToken  string                  `json:"access_token"`
//	RefreshToken string                  `json:"refresh_token"`
//	Account      *AccountProfileResponse `json:"account"`
//}

// AccountProfileResponse
//type AccountProfileResponse struct {
//	ID                string `json:"id"`
//	Name              string `json:"name"`
//	Email             string `json:"email"`
//	Avatar            string `json:"avatar"`
//	AvatarURL         string `json:"avatar_url"`
//	IsPasswordSet     bool   `json:"is_password_set"`
//	InterfaceLanguage string `json:"interface_language"`
//	InterfaceTheme    string `json:"interface_theme"`
//	Timezone          string `json:"timezone"`
//	LastLoginAt       *int64 `json:"last_login_at"`
//	LastLoginIP       string `json:"last_login_ip"`
//	CreatedAt         *int64 `json:"created_at"`
//	Status            string `json:"status"`
//	Mobile            string `json:"mobile,omitempty"`
//	Wechat            string `json:"wechat,omitempty"`
//	EnterpriseLicense bool   `json:"enterprise_license"`
//}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	loginReq := &shared_dto.LoginReq{
		Email:       req.Email,
		Password:    req.Password,
		RememberMe:  req.RememberMe,
		InviteToken: req.InviteToken,
		Language:    req.Language,
		LastLoginIp: c.ClientIP(),
	}

	sharedLoginResult := h.accountService.LoginRefactored(c.Request.Context(), loginReq)

	loginResult := &LoginResult{
		Success: sharedLoginResult.Success,
	}

	switch sharedLoginResult.ResultType {
	case shared_dto.LoginResultTypeSuccess:
		loginResult.Type = ResponseTypeSuccess
		loginResult.TokenPair = &TokenPair{
			AccessToken:  sharedLoginResult.Data.AccessToken,
			RefreshToken: sharedLoginResult.Data.RefreshToken,
		}
		loginResult.Account = sharedLoginResult.Data.Account
	case shared_dto.LoginResultTypeSpecialFail:
		loginResult.Type = ResponseTypeSpecialFail
		loginResult.SpecialData = sharedLoginResult.SpecialData
		loginResult.SpecialCode = sharedLoginResult.SpecialCode
	case shared_dto.LoginResultTypeBusinessError:
		loginResult.Type = ResponseTypeBusinessError
		loginResult.LoginError = &LoginError{
			Code:    sharedLoginResult.ErrorCode,
			Message: sharedLoginResult.Message,
		}
	default:
		if sharedLoginResult.Success && sharedLoginResult.Data != nil {
			loginResult.Type = ResponseTypeSuccess
			loginResult.TokenPair = &TokenPair{
				AccessToken:  sharedLoginResult.Data.AccessToken,
				RefreshToken: sharedLoginResult.Data.RefreshToken,
			}
			loginResult.Account = sharedLoginResult.Data.Account
			break
		}
		loginResult.Type = ResponseTypeBusinessError
		loginResult.LoginError = &LoginError{
			Code:    helper.UnknownError.ErrorCode,
			Message: helper.UnknownError.Description,
		}
	}

	h.respondLoginResult(c, loginResult)
}

func (h *AuthHandler) respondLoginResult(c *gin.Context, result *LoginResult) {
	switch result.GetResponseType() {
	case ResponseTypeSuccess:
		h.respondSuccess(c, result)
	case ResponseTypeBusinessError:
		h.respondBusinessError(c, result)
	case ResponseTypeSpecialFail:
		h.respondSpecialFail(c, result)
	}
}

func (h *AuthHandler) respondSuccess(c *gin.Context, result *LoginResult) {
	tokenPair := result.GetTokenPair()
	if tokenPair == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	loginData := gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"account":       result.Account,
	}
	response.Success(c, gin.H{
		"result": "success",
		"data":   loginData,
	})
}

func (h *AuthHandler) respondBusinessError(c *gin.Context, result *LoginResult) {
	err := result.GetError()
	if errorCode, ok := standardLoginErrorCode(err); ok {
		errorCode.Message = err.Message
		response.Fail(c, errorCode)
		return
	}

	// Convert string code to int, fallback to business error if conversion fails
	code := 200001 // default business error code
	if intCode, parseErr := strconv.Atoi(err.Code); parseErr == nil {
		code = intCode
	}

	errorCode := response.ErrorCode{
		Code:        code,
		Message:     err.Message,
		UserVisible: true,
	}
	response.Fail(c, errorCode)
}

func standardLoginErrorCode(err *LoginError) (response.ErrorCode, bool) {
	if err == nil {
		return response.ErrorCode{}, false
	}

	if err.Code != "" {
		errorCode := auth_service.GetStandardErrorCode(helper.ErrorResponse{ErrorCode: err.Code})
		if errorCode.Code != response.ErrSystemError.Code {
			return errorCode, true
		}
		if err.Code == helper.UnknownError.ErrorCode {
			return response.ErrSystemError, true
		}
	}

	switch strings.TrimSpace(err.Message) {
	case helper.EmailPasswordLoginLimitError.Description, helper.EmailPasswordLoginLimitError.EnDescription:
		return response.ErrLoginErrorRateLimit, true
	case helper.EmailOrPasswordMismatchError.Description, helper.EmailOrPasswordMismatchError.EnDescription:
		return response.ErrEmailPasswordMismatch, true
	}

	return response.ErrorCode{}, false
}

func (h *AuthHandler) respondSpecialFail(c *gin.Context, result *LoginResult) {
	specialData := result.GetSpecialData()
	specialCode := result.GetSpecialCode()

	if specialCode == "account_not_found" {
		response.SpecialFail(c, gin.H{
			"result": "fail",
			"data":   specialData,
			"code":   specialCode,
		})
	} else {

		response.SpecialFail(c, gin.H{
			"result": "fail",
			"data":   specialData,
		})
	}
}

func (h *AuthHandler) CheckEmailRegistered(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	exists := h.accountService.ExistsByEmail(c.Request.Context(), req.Email)

	response.Success(c, gin.H{
		"email":         req.Email,
		"is_registered": exists,
	})
}

func (h *AuthHandler) RegisterSendEmail(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Language string `json:"language"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	_, err := h.featureService.GetSystemFeatures(c)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if !h.featureService.IsPublicDeployment() {
		response.Fail(c, response.ErrRegisterNotAllowed)
		return
	}

	ipAddress := c.ClientIP()
	if limit, err := h.accountService.IsEmailSendIPLimit(context.Background(), ipAddress); err != nil || limit {
		response.Fail(c, response.ErrRateLimitExceeded)
		return
	}

	language := "en-US"
	if req.Language == "zh-Hans" {
		language = "zh-Hans"
	}

	if h.accountService.ExistsByEmail(c.Request.Context(), req.Email) {
		response.Fail(c, response.ErrUserExists)
		return
	}

	token, err := h.accountService.SendResetPasswordEmail(context.Background(), nil, req.Email, language)
	if err != nil {
		if isResetPasswordEmailRateLimitError(err) {
			response.Fail(c, response.ErrRateLimitExceeded)
			return
		}
		response.Fail(c, response.ErrEmailSendFailed)
		return
	}

	response.Success(c, gin.H{
		"result": "success",
		"data":   token,
	})
}

func (h *AuthHandler) RegisterCheck(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Code  string `json:"code" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get token data
	tokenData, err := h.accountService.GetResetPasswordData(context.Background(), req.Token)
	if err != nil || tokenData == nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	// Validate email
	tokenEmail, _ := tokenData["email"].(string)
	if req.Email != tokenEmail {
		response.Fail(c, response.ErrEmailFormat)
		return
	}

	// Validate verification code
	tokenCode, _ := tokenData["code"].(string)
	masterCode := config.Current().Auth.MasterVerificationCode
	if req.Code != tokenCode && (masterCode == "" || req.Code != masterCode) {
		response.Fail(c, response.ErrInvalidCode)
		return
	}

	response.Success(c, gin.H{
		"is_valid": true,
		"email":    tokenEmail,
	})
}

func (h *AuthHandler) RegisterFinish(c *gin.Context) {
	var req struct {
		Token           string `json:"token" binding:"required"`
		Name            string `json:"name" binding:"required"`
		Password        string `json:"password" binding:"required"`
		PasswordConfirm string `json:"password_confirm" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Info("RegisterFinish: 参数校验失败, token=%s, err=%v", req.Token, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if strings.TrimSpace(req.Password) != strings.TrimSpace(req.PasswordConfirm) {
		logger.Info("RegisterFinish: 两次密码不一致, token=%s, email=%s", req.Token, req.Name)
		response.Fail(c, response.ErrPasswordMismatch)
		return
	}

	registerData, err := h.accountService.GetResetPasswordData(context.Background(), req.Token)
	if err != nil || registerData == nil {
		logger.Info("RegisterFinish: token 校验失败, token=%s, err=%v", req.Token, err)
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	email, _ := registerData["email"].(string)
	if email == "" {
		logger.Info("RegisterFinish: token 中 email 为空, token=%s", req.Token)
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	if h.accountService.ExistsByEmail(c.Request.Context(), email) {
		logger.Info("RegisterFinish: 邮箱已注册, token=%s, email=%s", req.Token, email)
		response.Fail(c, response.ErrUserExists)
		return
	}

	name := req.Name
	if strings.TrimSpace(name) == "" {
		name = strings.Split(email, "@")[0]
	}

	language := "en-US"
	createWorkspace := true
	_, err = h.accountService.RegisterEx(context.Background(), email, name, &req.Password, nil, nil, &language, nil, nil, &createWorkspace)
	if err != nil {
		if strings.Contains(err.Error(), "frozen") || strings.Contains(err.Error(), "freeze") {
			logger.Info("RegisterFinish: 账号被冻结, token=%s, email=%s, err=%v", req.Token, email, err)
			response.Fail(c, response.ErrAccountFrozen)
			return
		}
		logger.Info("RegisterFinish: 注册服务报错, token=%s, email=%s, err=%v", req.Token, email, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	loginReq := &shared_dto.LoginReq{
		Email:       email,
		Password:    req.Password,
		LastLoginIp: c.ClientIP(),
	}

	_, err, loginResp, _ := h.accountService.Login(c.Request.Context(), loginReq)
	if err != nil {
		logger.Info("RegisterFinish: 自动登录失败, token=%s, email=%s, err=%v", req.Token, email, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	h.accountService.RevokeResetPasswordToken(context.Background(), req.Token)

	response.Success(c, gin.H{
		"result": "success",
		"data": gin.H{
			"access_token":  loginResp.AccessToken,
			"refresh_token": loginResp.RefreshToken,
			"account":       loginResp.Account,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	// Assume accessToken is obtained from header
	accessToken := c.GetHeader("Authorization")
	if accessToken == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	err := h.tokenManager.RevokeToken(accessToken, "access")
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "Logged out successfully"})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate and refresh token
	tokenData, err := h.accountService.RefreshToken(context.Background(), req.RefreshToken)
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	response.Success(c, gin.H{
		"access_token":  tokenData.AccessToken,
		"refresh_token": tokenData.RefreshToken,
	})
}

func isResetPasswordEmailRateLimitError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Too many password reset emails")
}

func (h *AuthHandler) RegisterAuthRoutes(v1 *gin.RouterGroup) {
	v1.POST("/login", h.Login)
	v1.POST("/logout", h.Logout)
	v1.POST("/refresh-token", h.RefreshToken)
	v1.POST("/email/check", h.CheckEmailRegistered)
	v1.GET("/sso/casdoor/start", h.StartCasdoorSSO)
	v1.GET("/sso/casdoor/callback", h.HandleCasdoorCallback)
	v1.POST("/sso/casdoor/consume-ticket", h.ConsumeSSOLoginTicket)

	// Three registration-related interfaces
	v1.POST("/register", h.RegisterSendEmail)      // Corresponds to RegisterSendEmailApi
	v1.POST("/register/validity", h.RegisterCheck) // Corresponds to RegisterCheckApi
	v1.POST("/register/finish", h.RegisterFinish)  // Corresponds to RegisterFinishApi
}
