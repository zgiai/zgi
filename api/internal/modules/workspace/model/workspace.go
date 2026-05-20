package model

import (
	"encoding/json"
	"time"
)

// WorkspaceStatus Workspace status enumeration
type WorkspaceStatus string

const (
	WorkspaceStatusNormal   WorkspaceStatus = "normal"
	WorkspaceStatusArchived WorkspaceStatus = "archived"
)

// Workspace model
type Workspace struct {
	ID               string          `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name             string          `gorm:"type:varchar(255);not null" json:"name"`
	EncryptPublicKey *string         `gorm:"type:text" json:"-"`
	Plan             string          `gorm:"type:varchar(255);not null;default:'basic'" json:"plan"`
	Status           WorkspaceStatus `gorm:"type:varchar(16);not null;default:'normal'" json:"status"`
	OrganizationID   *string         `gorm:"type:uuid;index" json:"organization_id"`
	DepartmentID     *string         `gorm:"type:uuid" json:"department_id"`
	ApiKeyID         *string         `gorm:"type:uuid" json:"api_key_id"`
	CustomConfig     *string         `gorm:"type:text" json:"-"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// TableName specifies table name
func (Workspace) TableName() string {
	return "workspaces"
}

// IsNormal Check if Workspace is normal
func (t *Workspace) IsNormal() bool {
	return t.Status == WorkspaceStatusNormal
}

// IsArchived Check if Workspace is archived
func (t *Workspace) IsArchived() bool {
	return t.Status == WorkspaceStatusArchived
}

// GetCustomConfigDict Get custom configuration dictionary
func (t *Workspace) GetCustomConfigDict() map[string]interface{} {
	if t.CustomConfig == nil || *t.CustomConfig == "" {
		return make(map[string]interface{})
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(*t.CustomConfig), &config); err != nil {
		return make(map[string]interface{})
	}

	return config
}

// WorkspaceMemberRole Workspace account role enumeration
type WorkspaceMemberRole string

const (
	WorkspaceRoleOwner  WorkspaceMemberRole = "owner"
	WorkspaceRoleAdmin  WorkspaceMemberRole = "admin"
	WorkspaceRoleEditor WorkspaceMemberRole = "editor"
	WorkspaceRoleMember WorkspaceMemberRole = "member"
	WorkspaceRoleViewer WorkspaceMemberRole = "viewer"
	WorkspaceRoleNormal WorkspaceMemberRole = "normal"
)

// IsValidRole Check if the role is valid
func (r WorkspaceMemberRole) IsValidRole() bool {
	validRoles := []WorkspaceMemberRole{
		WorkspaceRoleOwner,
		WorkspaceRoleAdmin,
		WorkspaceRoleEditor,
		WorkspaceRoleNormal,
	}

	for _, validRole := range validRoles {
		if r == validRole {
			return true
		}
	}
	return false
}

// IsPrivilegedRole Check if it's a privileged role
func (r WorkspaceMemberRole) IsPrivilegedRole() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin
}

// IsAdminRole Check if it's an admin role
func (r WorkspaceMemberRole) IsAdminRole() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin
}

// IsEditingRole Check if it's an editing role
func (r WorkspaceMemberRole) IsEditingRole() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin || r == WorkspaceRoleEditor
}

// IsDatasetEditRole Check if it's a dataset editing role
func (r WorkspaceMemberRole) IsDatasetEditRole() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin || r == WorkspaceRoleEditor
}

func (r WorkspaceMemberRole) IsNonOwnerRole() bool {
	return r == WorkspaceRoleAdmin || r == WorkspaceRoleEditor || r == WorkspaceRoleNormal
}

// WorkspaceMember Workspace account association
type WorkspaceMember struct {
	ID          string              `gorm:"type:varchar(255);primaryKey" json:"id"`
	WorkspaceID string              `gorm:"type:varchar(255);not null;index" json:"workspace_id"`
	AccountID   string              `gorm:"type:varchar(255);not null;index" json:"account_id"`
	Role        WorkspaceMemberRole `gorm:"type:varchar(16);not null" json:"role"`
	RoleID      *string             `gorm:"type:uuid" json:"role_id,omitempty"`
	Current     bool                `gorm:"not null;default:false" json:"current"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	InvitedBy   *string             `gorm:"column:invited_by;type:uuid" json:"invited_by"`

	// Extensions JSON field for additional data (e.g., position)
	// Currently used to store:
	// - position: string
	Extensions map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"extensions"`
}

// TableName specifies table name
func (WorkspaceMember) TableName() string {
	return "workspace_members"
}

// WorkspaceInfo represents a single Workspace info
type WorkspaceInfo struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Plan           string                 `json:"plan"`
	Status         string                 `json:"status"`
	CreatedAt      int64                  `json:"created_at"`
	Role           *string                `json:"role"`
	InTrial        *bool                  `json:"in_trial"`
	TrialEndReason *string                `json:"trial_end_reason"`
	CustomConfig   map[string]interface{} `json:"custom_config"`
	Current        bool                   `json:"current"`
	AdminsCount    int                    `json:"admins_count"`
	MembersCount   int                    `json:"members_count"`
	DatasetsCount  int                    `json:"datasets_count"`
	AgentsCount    int                    `json:"agents_count"`
}

// CustomConfigRequest represents the request to update custom config
type CustomConfigRequest struct {
	RemoveWebappBrand *bool   `json:"remove_webapp_brand"`
	ReplaceWebappLogo *string `json:"replace_webapp_logo"`
}

// WebappLogoUploadResponse represents the response for webapp logo upload
type WebappLogoUploadResponse struct {
	ID string `json:"id"`
}

// WorkspaceCreateRequest represents the request to create a workspace
type WorkspaceCreateRequest struct {
	Name string `json:"name" binding:"required"`
}

// WorkspaceCreateResponse represents the response for creating workspace
type WorkspaceCreateResponse struct {
	Message string `json:"message"`
}

// WorkspaceUpdateRequest represents the request to update workspace
type WorkspaceUpdateRequest struct {
	Name   *string          `json:"name,omitempty"`
	Status *WorkspaceStatus `json:"status,omitempty"`
}

// WorkspaceUpdateResponse represents the response for updating workspace
type WorkspaceUpdateResponse struct {
	Result string `json:"result"`
	Tenant struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"tenant"`
}

// ErrorResponse represents error response with code and message
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
