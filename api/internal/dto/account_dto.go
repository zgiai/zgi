package dto

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/config"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"github.com/zgiai/ginext/internal/util"
)

type UpdateAccountExRequest struct {
	Name     string                 `json:"name,omitempty"`
	Email    string                 `json:"email,omitempty"`
	Password string                 `json:"password,omitempty"`
	Role     string                 `json:"role,omitempty"`
	Status   string                 `json:"status,omitempty"`
	Tenant   *TenantInfo            `json:"tenant,omitempty"`
	Mobile   string                 `json:"mobile,omitempty"`
	Gender   *auth_model.GenderEnum `json:"gender,omitempty"`
	Wechat   string                 `json:"wechat,omitempty"`
	Address  string                 `json:"address,omitempty"`
}

type CreateAccountRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Name     string `json:"name" binding:"required"`
	Language string `json:"language"`
	Timezone string `json:"timezone"`
	IsSetup  bool
}

type UpdateAccountRequest struct {
	Name   string                   `json:"name"`
	Avatar string                   `json:"avatar"`
	Status auth_model.AccountStatus `json:"status"`
}

type UpdateProfileRequest struct {
	Name     *string `json:"name,omitempty"`
	Avatar   *string `json:"avatar,omitempty"`
	Language *string `json:"language,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
	Mobile   *string `json:"mobile,omitempty"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type AccountProfileResponse struct {
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	Email                 string             `json:"email"`
	Avatar                string             `json:"avatar"`
	InterfaceLanguage     string             `json:"language"`
	Timezone              string             `json:"timezone"`
	Status                string             `json:"status"`
	GroupRole             string             `json:"group_role"`
	OrganizationRole      string             `json:"organization_role"`
	IsSuperAdmin          *bool              `json:"is_super_admin,omitempty"`
	Extension             auth_model.JSONMap `json:"extension"`
	CurrentOrganizationID *string            `json:"current_organization_id,omitempty"`
	CurrentWorkspaceID    *string            `json:"current_workspace_id"`
}

// MarshalJSON implements custom JSON marshaling to generate avatar URLs
func (a *AccountProfileResponse) MarshalJSON() ([]byte, error) {
	// Generate avatar URL if needed
	var avatarUrl string

	if a.Avatar != "" {
		// Check if Avatar already starts with http/https
		if strings.HasPrefix(strings.ToLower(a.Avatar), "http://") || strings.HasPrefix(strings.ToLower(a.Avatar), "https://") {
			// Avatar is already a full URL, use it directly
			avatarUrl = a.Avatar
		} else {
			// Avatar is a file ID, generate signed preview URL
			signedURL, err := util.GetSignedFileURL(a.Avatar)
			if err == nil {
				avatarUrl = signedURL
			} else {
				// Fallback: use simple URL without signature
				if config.GlobalConfig != nil && config.GlobalConfig.App.FilesURL != "" {
					consoleAPIURL := config.GlobalConfig.Console.APIURL
					avatarUrl = fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, a.Avatar)
				}
			}
		}
	}

	// Create alias to avoid infinite recursion
	type Alias AccountProfileResponse
	return json.Marshal(&struct {
		*Alias
		AvatarUrl string `json:"avatar_url,omitempty"`
	}{
		Alias:     (*Alias)(a),
		AvatarUrl: avatarUrl,
	})
}

type LoginReq struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required"`
	RememberMe  bool   `json:"remember_me"`
	InviteToken string `json:"invite_token,omitempty"`
	Language    string `json:"language"`
	LastLoginIp string `json:"last_login_ip"`
	Name        string `json:"name"`
}

type LoginResponse struct {
	AccessToken  string                  `json:"access_token"`
	RefreshToken string                  `json:"refresh_token"`
	Account      *AccountProfileResponse `json:"account"`
	SSO          *SSOProviderToken       `json:"sso,omitempty"`
}

type LoginResultType string

const (
	LoginResultTypeSuccess       LoginResultType = "success"
	LoginResultTypeBusinessError LoginResultType = "business_error"
	LoginResultTypeSpecialFail   LoginResultType = "special_fail"
)

type LoginResult struct {
	Success     bool            `json:"success"`
	Message     string          `json:"message,omitempty"`
	ErrorCode   string          `json:"-"`
	ResultType  LoginResultType `json:"-"`
	SpecialData interface{}     `json:"-"`
	SpecialCode string          `json:"-"`
	Data        *LoginResponse  `json:"data,omitempty"`
}
