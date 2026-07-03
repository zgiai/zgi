package dto

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

// OrganizationMemberResponse represents a member in an enterprise group
type OrganizationMemberResponse struct {
	AccountID string                 `json:"account_id"`
	Email     string                 `json:"email"`
	Username  string                 `json:"username"`
	Name      *string                `json:"name"`
	Role      model.OrganizationRole `json:"role"`
	JoinedAt  time.Time              `json:"joined_at"`
}

// OrganizationWithRoleResponse represents an enterprise group with user role
type OrganizationWithRoleResponse struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	ShortName        *string                  `json:"short_name"`
	Status           model.OrganizationStatus `json:"status"`
	CreatedAt        int64                    `json:"created_at"`
	OrganizationRole model.OrganizationRole   `json:"organization_role"`
}

// OrganizationPaginationResponse represents a paginated list of enterprise groups
type OrganizationPaginationResponse struct {
	Data    []*OrganizationWithRoleResponse `json:"data"`
	Page    int                             `json:"page"`
	Limit   int                             `json:"limit"`
	Total   int64                           `json:"total"`
	HasMore bool                            `json:"has_more"`
}

// WorkspacePaginationResponse represents a paginated list of tenants
type WorkspacePaginationResponse struct {
	Data    []*model.Workspace `json:"data"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	Total   int64              `json:"total"`
	HasMore bool               `json:"has_more"`
}

type OrganizationWorkspaceResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	CreatedAt      int64   `json:"created_at"`
	LeaderID       *string `json:"leader_id,omitempty"`
	LeaderName     *string `json:"leader_name,omitempty"`
	DepartmentID   *string `json:"department_id,omitempty"`
	DepartmentName *string `json:"department_name,omitempty"`
	APIKeyID       *string `json:"api_key_id,omitempty"`
	APIKeyName     *string `json:"api_key_name,omitempty"`
	MemberCount    int64   `json:"member_count"`
}

type OrganizationWorkspacePaginationResponse struct {
	Data    []*OrganizationWorkspaceResponse `json:"data"`
	Page    int                              `json:"page"`
	Limit   int                              `json:"limit"`
	Total   int64                            `json:"total"`
	HasMore bool                             `json:"has_more"`
}

// AccountSystemRole represents system role information
type AccountSystemRole struct {
	RoleType string `json:"role_type"`
}

// OrganizationMemberWithExtensionResponse represents a group member with extension info
type OrganizationMemberWithExtensionResponse struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	AccountName      string                 `json:"account_name"`
	MemberName       *string                `json:"member_name"`
	Avatar           string                 `json:"avatar"`
	AvatarURL        string                 `json:"avatar_url"`
	Email            string                 `json:"email"`
	LastLoginAt      *int64                 `json:"last_login_at"`
	LastActiveAt     *int64                 `json:"last_active_at"`
	CreatedAt        int64                  `json:"created_at"`
	Status           string                 `json:"status"`
	OrganizationRole model.OrganizationRole `json:"organization_role"`
	AccountRole      *AccountSystemRole     `json:"account_role"`
	Extension        map[string]interface{} `json:"extension"`
	DepartmentID     *string                `json:"department_id,omitempty"`
	DepartmentName   *string                `json:"department_name,omitempty"`
}

// MarshalJSON implements custom JSON marshaling to generate avatar URLs
func (g *OrganizationMemberWithExtensionResponse) MarshalJSON() ([]byte, error) {
	// Generate avatar URL if needed
	var avatarUrl string

	if g.Avatar != "" {
		// Check if Avatar already starts with http/https
		if strings.HasPrefix(strings.ToLower(g.Avatar), "http://") || strings.HasPrefix(strings.ToLower(g.Avatar), "https://") {
			// Avatar is already a full URL, use it directly
			avatarUrl = g.Avatar
		} else {
			// Avatar is a file ID, generate signed preview URL
			signedURL, err := util.GetSignedFileURL(g.Avatar)
			if err == nil {
				avatarUrl = signedURL
			} else {
				// Fallback: use simple URL without signature
				if config.GlobalConfig != nil && config.GlobalConfig.App.FilesURL != "" {
					consoleAPIURL := config.GlobalConfig.Console.APIURL
					avatarUrl = fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, g.Avatar)
				}
			}
		}
	}

	// Create alias to avoid infinite recursion
	type Alias OrganizationMemberWithExtensionResponse
	return json.Marshal(&struct {
		*Alias
		AvatarURL string `json:"avatar_url,omitempty"`
	}{
		Alias:     (*Alias)(g),
		AvatarURL: avatarUrl,
	})
}

// OrganizationMemberPaginationResponse represents a paginated list of organization members
type OrganizationMemberPaginationResponse struct {
	Data    []*OrganizationMemberWithExtensionResponse `json:"data"`
	Page    int                                        `json:"page"`
	Limit   int                                        `json:"limit"`
	Total   int64                                      `json:"total"`
	HasMore bool                                       `json:"has_more"`
}

type OrganizationJoinRequestItem struct {
	ID             string                              `json:"id"`
	OrganizationID string                              `json:"organization_id"`
	InviteLinkID   *string                             `json:"invite_link_id,omitempty"`
	AccountID      string                              `json:"account_id"`
	AccountName    string                              `json:"account_name"`
	MemberName     *string                             `json:"member_name,omitempty"`
	AccountEmail   string                              `json:"account_email"`
	Avatar         string                              `json:"avatar"`
	AvatarURL      string                              `json:"avatar_url"`
	DepartmentID   *string                             `json:"department_id,omitempty"`
	DepartmentName *string                             `json:"department_name,omitempty"`
	WorkspaceID    *string                             `json:"workspace_id,omitempty"`
	Status         model.OrganizationJoinRequestStatus `json:"status"`
	Reason         *string                             `json:"reason,omitempty"`
	ReviewerID     *string                             `json:"reviewer_id,omitempty"`
	CreatedAt      int64                               `json:"created_at"`
	ReviewedAt     *int64                              `json:"reviewed_at,omitempty"`
}

func (e *OrganizationJoinRequestItem) MarshalJSON() ([]byte, error) {
	var avatarUrl string

	if e.Avatar != "" {
		if strings.HasPrefix(strings.ToLower(e.Avatar), "http://") || strings.HasPrefix(strings.ToLower(e.Avatar), "https://") {
			avatarUrl = e.Avatar
		} else {
			signedURL, err := util.GetSignedFileURL(e.Avatar)
			if err == nil {
				avatarUrl = signedURL
			} else {
				if config.GlobalConfig != nil && config.GlobalConfig.App.FilesURL != "" {
					consoleAPIURL := config.GlobalConfig.Console.APIURL
					avatarUrl = fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, e.Avatar)
				}
			}
		}
	}

	type Alias OrganizationJoinRequestItem
	return json.Marshal(&struct {
		*Alias
		AvatarURL string `json:"avatar_url,omitempty"`
	}{
		Alias:     (*Alias)(e),
		AvatarURL: avatarUrl,
	})
}

type OrganizationJoinRequestPaginationResponse struct {
	Data    []*OrganizationJoinRequestItem `json:"data"`
	Page    int                            `json:"page"`
	Limit   int                            `json:"limit"`
	Total   int64                          `json:"total"`
	HasMore bool                           `json:"has_more"`
}

// WorkspaceRoleResponse represents tenant role information
type WorkspaceRoleResponse struct {
	WorkspaceID   string   `json:"workspace_id"`
	WorkspaceName string   `json:"workspace_name"`
	Role          string   `json:"role"`
	Position      string   `json:"position"`
	Permissions   []string `json:"permissions"`
}

// CurrentOrganizationResponse represents current enterprise information
type CurrentOrganizationResponse struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	ShortName        *string                  `json:"short_name"`
	Status           model.OrganizationStatus `json:"status"`
	CreatedAt        int64                    `json:"created_at"`
	OrganizationRole model.OrganizationRole   `json:"organization_role"`
}

// CurrentOrganizationDetailResponse represents detailed current enterprise information
type CurrentOrganizationDetailResponse struct {
	EnterpriseGroup *CurrentOrganizationResponse `json:"enterprise_group"`
	ShadowTenant    *model.Workspace             `json:"shadow_tenant"`
}

// DatasetSimple represents simplified dataset information
type DatasetSimple struct {
	ID                     string                 `json:"id"`
	Name                   string                 `json:"name"`
	Description            *string                `json:"description"`
	Provider               string                 `json:"provider"`
	Permission             string                 `json:"permission"`
	DataSourceType         string                 `json:"data_source_type"`
	IndexingTechnique      string                 `json:"indexing_technique"`
	AppCount               int                    `json:"app_count"`
	DocumentCount          int                    `json:"document_count"`
	WordCount              int                    `json:"word_count"`
	CreatedBy              string                 `json:"created_by"`
	CreatedAt              int64                  `json:"created_at"`
	UpdatedBy              *string                `json:"updated_by"`
	UpdatedAt              *int64                 `json:"updated_at"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	EmbeddingAvailable     bool                   `json:"embedding_available"`
	Tags                   []interface{}          `json:"tags"`
	DocForm                string                 `json:"doc_form"`
	Icon                   *string                `json:"icon"`
	IconBackground         *string                `json:"icon_background"`
	IconURL                *string                `json:"icon_url"`
	Owner                  *string                `json:"owner"`
	OwnerAccount           map[string]interface{} `json:"owner_account"`
	Tenant                 map[string]interface{} `json:"tenant"`
}

// DatasetPaginationResponse represents a paginated list of datasets
type DatasetPaginationResponse struct {
	Page    int              `json:"page"`
	PerPage int              `json:"limit"`
	Total   int64            `json:"total"`
	HasMore bool             `json:"has_more"`
	Data    []*DatasetSimple `json:"data"`
}

// ========== Request DTOs ==========

// CreateOrganizationRequest represents the request to create a new enterprise group
type CreateOrganizationRequest struct {
	Name      string `json:"name" binding:"required"`
	CreatedBy string `json:"created_by" binding:"required"`
}

// CreateOrganizationWithWorkspaceRequest represents the request to create an enterprise group with tenant
type CreateOrganizationWithWorkspaceRequest struct {
	Name      string  `json:"name" binding:"required"`
	ShortName *string `json:"short_name,omitempty"`
	CreatedBy string  `json:"created_by" binding:"required"`
}

// UpdateOrganizationRequest represents the request to update an enterprise group
type UpdateOrganizationRequest struct {
	Name      string  `json:"name" binding:"required"`
	ShortName *string `json:"short_name,omitempty"`
}

// AddWorkspaceToOrganizationRequest represents the request to add a tenant to enterprise
type AddWorkspaceToOrganizationRequest struct {
	OrganizationID string  `json:"organization_id" binding:"required"`
	WorkspaceID    string  `json:"workspace_id" binding:"required"`
	DepartmentID   *string `json:"department_id,omitempty"`
	APIKeyID       *string `json:"api_key_id,omitempty"`
}

// AddOrganizationMemberRequest represents the request to add a member to enterprise group
type AddOrganizationMemberRequest struct {
	OrganizationID string                 `json:"organization_id" binding:"required"`
	AccountID      string                 `json:"account_id" binding:"required"`
	Role           model.OrganizationRole `json:"role" binding:"required"`
	Name           *string                `json:"name"`
}

type InviteCurrentOrganizationMemberRequest struct {
	OrganizationID    string
	OperatorAccountID string
	WorkspaceID       string
	Email             string
	Name              string
	Password          string
	DepartmentID      *string
}

type InviteCurrentOrganizationMemberResponse struct {
	AccountID       string                 `json:"account_id"`
	Email           string                 `json:"email"`
	Name            string                 `json:"name"`
	OrganizationID  string                 `json:"organization_id"`
	Role            model.OrganizationRole `json:"role"`
	CreatedAccount  bool                   `json:"created_account"`
	AlreadyMember   bool                   `json:"already_member"`
	PasswordApplied bool                   `json:"password_applied"`
	Department      *MemberDepartmentInfo  `json:"department,omitempty"`
	Workspace       *MemberWorkspaceInfo   `json:"workspace,omitempty"`
}

type DirectAddOrganizationMemberRequest struct {
	OrganizationID    string
	OperatorAccountID string
	WorkspaceID       string
	Email             string
	Name              string
	DepartmentID      *string
}

type DirectAddOrganizationMemberResponse struct {
	AccountID      string                `json:"account_id"`
	Email          string                `json:"email"`
	Name           string                `json:"name"`
	OrganizationID string                `json:"organization_id"`
	Department     *MemberDepartmentInfo `json:"department,omitempty"`
	Workspace      *MemberWorkspaceInfo  `json:"workspace,omitempty"`
	CreatedAccount bool                  `json:"created_account"`
	AlreadyMember  bool                  `json:"already_member"`
}

type MemberDepartmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MemberWorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ResetCurrentOrganizationMemberPasswordRequest struct {
	OrganizationID    string
	OperatorAccountID string
	Email             string
	Password          string
}

type ResetCurrentOrganizationMemberPasswordResponse struct {
	AccountID     string `json:"account_id"`
	Email         string `json:"email"`
	PasswordReset bool   `json:"password_reset"`
}

// UpdateOrganizationMemberRoleRequest represents the request to update member role
type UpdateOrganizationMemberRoleRequest struct {
	OrganizationID string                 `json:"organization_id" binding:"required"`
	AccountID      string                 `json:"account_id" binding:"required"`
	Role           model.OrganizationRole `json:"role" binding:"required"`
}

type UpdateCurrentOrganizationMemberRoleRequest struct {
	Role model.OrganizationRole `json:"role" binding:"required"`
}

// UpdateOrganizationMemberRequest represents the request to update member info
type UpdateOrganizationMemberRequest struct {
	OrganizationID string                  `json:"organization_id"` // Add alias for consistency
	AccountID      string                  `json:"account_id"`
	MemberID       string                  `json:"member_id"` // Add alias
	Name           *string                 `json:"name,omitempty"`
	Role           *model.OrganizationRole `json:"role,omitempty"`
}

// GroupPermissionDefinition represents a single permission definition item
type GroupPermissionDefinition struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Default     bool   `json:"default"`
}

// GroupPermissionDefinitionResponse represents permission definition list
type GroupPermissionDefinitionResponse struct {
	Items []GroupPermissionDefinition `json:"items"`
}

// WorkspaceRoleSummary represents a role summary in list
type WorkspaceRoleSummary struct {
	ID              string                          `json:"id"`
	Name            string                          `json:"name"`
	Description     *string                         `json:"description,omitempty"`
	DescriptionI18n *LocalizedString                `json:"description_i18n,omitempty"`
	Builtin         bool                            `json:"builtin"`
	Editable        bool                            `json:"editable"`
	Status          model.WorkspaceCustomRoleStatus `json:"status"`
	Permissions     []string                        `json:"permissions"`
	MemberCount     int64                           `json:"member_count"`
}

// WorkspaceRoleListResponse represents role list response
type WorkspaceRoleListResponse struct {
	Roles []WorkspaceRoleSummary `json:"roles"`
}

type OrganizationRoleMemberItem struct {
	AccountID  string                      `json:"account_id"`
	Name       string                      `json:"name"`
	Email      string                      `json:"email"`
	Avatar     string                      `json:"avatar"`
	AvatarURL  string                      `json:"avatar_url"`
	Workspaces []MemberWorkspacePermission `json:"workspaces"`
}

type OrganizationRoleMembersResponse struct {
	RoleID  string                       `json:"role_id"`
	Items   []OrganizationRoleMemberItem `json:"items"`
	Page    int                          `json:"page"`
	Limit   int                          `json:"limit"`
	Total   int64                        `json:"total"`
	HasMore bool                         `json:"has_more"`
}

// CreateWorkspaceRoleRequest represents request to create custom role
type CreateWorkspaceRoleRequest struct {
	OrganizationID string   `json:"organization_id"`
	Name           string   `json:"name" binding:"required"`
	Description    *string  `json:"description,omitempty"`
	Permissions    []string `json:"permissions"`
	CreatedBy      string   `json:"-"`
}

// UpdateWorkspaceRoleRequest represents request to update role basic info
type UpdateWorkspaceRoleRequest struct {
	OrganizationID string  `json:"organization_id"`
	RoleID         string  `json:"-"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
}

// UpdateWorkspaceRolePermissionsRequest represents request to update role permissions
type UpdateWorkspaceRolePermissionsRequest struct {
	OrganizationID string   `json:"organization_id"`
	RoleID         string   `json:"role_id" binding:"required"`
	Permissions    []string `json:"permissions"`
	OperatorID     string   `json:"-"`
}

// OrganizationRoleDetailResponse represents role detail
type OrganizationRoleDetailResponse struct {
	ID              string                          `json:"id"`
	OrganizationID  string                          `json:"organization_id"`
	Name            string                          `json:"name"`
	Description     *string                         `json:"description,omitempty"`
	DescriptionI18n *LocalizedString                `json:"description_i18n,omitempty"`
	Builtin         bool                            `json:"builtin"`
	Editable        bool                            `json:"editable"`
	Status          model.WorkspaceCustomRoleStatus `json:"status"`
	Permissions     []string                        `json:"permissions"`
}

// Deprecated: use OrganizationRoleMemberItem.
type GroupRoleMemberItem = OrganizationRoleMemberItem

// Deprecated: use OrganizationRoleMembersResponse.
type GroupRoleMembersResponse = OrganizationRoleMembersResponse

// Deprecated: use OrganizationRoleDetailResponse.
type GroupRoleDetailResponse = OrganizationRoleDetailResponse

type MemberWorkspacePermission struct {
	WorkspaceID   string   `json:"workspace_id"`
	WorkspaceName string   `json:"workspace_name"`
	Role          string   `json:"role"`
	RoleID        *string  `json:"role_id,omitempty"`
	RoleName      string   `json:"role_name"`
	Permissions   []string `json:"permissions"`
}

// MemberPermissionsResponse represents member effective permissions
type MemberPermissionsResponse struct {
	OrganizationID string                      `json:"organization_id"`
	AccountID      string                      `json:"account_id"`
	Role           string                      `json:"role"`
	Workspaces     []MemberWorkspacePermission `json:"workspaces"`
}

type WorkspaceMemberPermissionsResponse struct {
	OrganizationID    string   `json:"organization_id"`
	WorkspaceID       string   `json:"workspace_id"`
	WorkspaceName     string   `json:"workspace_name"`
	AccountID         string   `json:"account_id"`
	OrganizationRole  string   `json:"organization_role"`
	WorkspaceRole     string   `json:"workspace_role"`
	WorkspaceRoleID   *string  `json:"workspace_role_id"`
	WorkspaceRoleName string   `json:"workspace_role_name"`
	Permissions       []string `json:"permissions"`
}

// UpdateOrganizationMemberStatusRequest represents the request to update member status
type UpdateOrganizationMemberStatusRequest struct {
	OrganizationID string                         `json:"organization_id" binding:"required"`
	AccountID      string                         `json:"account_id" binding:"required"`
	Status         model.OrganizationMemberStatus `json:"status" binding:"required,oneof=active inactive"`
}

// GetOrganizationDatasetsPaginatedRequest represents the request to get group datasets
type GetOrganizationDatasetsPaginatedRequest struct {
	OrganizationID string  `json:"organization_id" binding:"required"`
	Page           int     `json:"page"`
	PerPage        int     `json:"per_page"`
	Search         *string `json:"search,omitempty"`
	UserID         string  `json:"user_id" binding:"required"`
}
