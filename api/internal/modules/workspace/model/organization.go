package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationStatus enterprise group status enum
type OrganizationStatus string

const (
	OrganizationStatusActive   OrganizationStatus = "active"
	OrganizationStatusInactive OrganizationStatus = "inactive"
	OrganizationStatusArchived OrganizationStatus = "archived"
	OrganizationStatusDeleted  OrganizationStatus = "deleted"
)

// OrganizationRole represents a role in an organization
type OrganizationRole string

const (
	OrganizationRoleOwner  OrganizationRole = "owner"
	OrganizationRoleAdmin  OrganizationRole = "admin"
	OrganizationRoleNormal OrganizationRole = "normal"
)

// Organization enterprise group model
type Organization struct {
	ID        string             `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name      string             `gorm:"type:varchar(255);not null" json:"name"`
	ShortName *string            `gorm:"type:varchar(255)" json:"short_name"`
	Status    OrganizationStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`

	// Relationships - commented out for modular architecture
	// TenantJoins  []EnterpriseGroupTenantJoin  `gorm:"foreignKey:GroupID" json:"-"`
	// Members      []OrganizationMember         `gorm:"foreignKey:GroupID" json:"-"`
}

// TableName sets the table name for Organization
func (Organization) TableName() string {
	return "organizations"
}

// IsActive checks if enterprise group is active
func (org *Organization) IsActive() bool {
	return org.Status == OrganizationStatusActive
}

// BeforeCreate hook to set ID and timestamps
func (org *Organization) BeforeCreate(tx *gorm.DB) error {
	// Generate UUID if ID is empty
	if org.ID == "" {
		org.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	if org.CreatedAt.IsZero() {
		org.CreatedAt = now
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = now
	}

	return nil
}

const (
	WorkspaceBuiltinRoleOwnerID  = "00000000-0000-0000-0000-000000000001"
	WorkspaceBuiltinRoleAdminID  = "00000000-0000-0000-0000-000000000002"
	WorkspaceBuiltinRoleMemberID = "00000000-0000-0000-0000-000000000003"
	WorkspaceBuiltinRoleViewerID = "00000000-0000-0000-0000-000000000004"
)

type WorkspacePermissionCode string

const (
	WorkspacePermissionWorkspaceView            WorkspacePermissionCode = "workspace.view"
	WorkspacePermissionWorkspaceManage          WorkspacePermissionCode = "workspace.manage"
	WorkspacePermissionWorkspaceBillingAudit    WorkspacePermissionCode = "workspace.billing_audit"
	WorkspacePermissionWorkspaceTransferArchive WorkspacePermissionCode = "workspace.transfer_archive"

	WorkspacePermissionAgentView   WorkspacePermissionCode = "agent.view"
	WorkspacePermissionAgentManage WorkspacePermissionCode = "agent.manage"
	WorkspacePermissionAgentLock   WorkspacePermissionCode = "agent.lock"

	WorkspacePermissionKnowledgeBaseView          WorkspacePermissionCode = "knowledge_base.view"
	WorkspacePermissionKnowledgeBaseManage        WorkspacePermissionCode = "knowledge_base.manage"
	WorkspacePermissionKnowledgeBaseRetrievalTest WorkspacePermissionCode = "knowledge_base.retrieval_test"
	WorkspacePermissionKnowledgeBaseFolderManage  WorkspacePermissionCode = "knowledge_base.folder_manage"
	WorkspacePermissionKnowledgeBaseLock          WorkspacePermissionCode = "knowledge_base.lock"

	WorkspacePermissionDatabaseView     WorkspacePermissionCode = "database.view"
	WorkspacePermissionDatabaseManage   WorkspacePermissionCode = "database.manage"
	WorkspacePermissionDatabaseDataEdit WorkspacePermissionCode = "database.data_edit"
	WorkspacePermissionDatabaseAIQuery  WorkspacePermissionCode = "database.ai_query"
	WorkspacePermissionDatabaseLock     WorkspacePermissionCode = "database.lock"

	WorkspacePermissionFileView         WorkspacePermissionCode = "file.view"
	WorkspacePermissionFileManage       WorkspacePermissionCode = "file.manage"
	WorkspacePermissionFileUploadCreate WorkspacePermissionCode = "file.upload_create"
	WorkspacePermissionFileDownload     WorkspacePermissionCode = "file.download"
	WorkspacePermissionFileMoveCreate   WorkspacePermissionCode = "file.move_create"
)

func IsBuiltinRole(roleID string) bool {
	return roleID == WorkspaceBuiltinRoleOwnerID ||
		roleID == WorkspaceBuiltinRoleAdminID ||
		roleID == WorkspaceBuiltinRoleMemberID ||
		roleID == WorkspaceBuiltinRoleViewerID
}

func AllWorkspacePermissionCodes() []WorkspacePermissionCode {
	return []WorkspacePermissionCode{
		WorkspacePermissionWorkspaceView,
		WorkspacePermissionWorkspaceManage,
		WorkspacePermissionWorkspaceBillingAudit,
		WorkspacePermissionWorkspaceTransferArchive,

		WorkspacePermissionAgentView,
		WorkspacePermissionAgentManage,
		WorkspacePermissionAgentLock,
		WorkspacePermissionKnowledgeBaseView,
		WorkspacePermissionKnowledgeBaseManage,
		WorkspacePermissionKnowledgeBaseRetrievalTest,
		WorkspacePermissionKnowledgeBaseFolderManage,
		WorkspacePermissionKnowledgeBaseLock,
		WorkspacePermissionDatabaseView,
		WorkspacePermissionDatabaseManage,
		WorkspacePermissionDatabaseDataEdit,
		WorkspacePermissionDatabaseAIQuery,
		WorkspacePermissionDatabaseLock,
		WorkspacePermissionFileView,
		WorkspacePermissionFileManage,
		WorkspacePermissionFileUploadCreate,
		WorkspacePermissionFileDownload,
		WorkspacePermissionFileMoveCreate,
	}
}

func GetBuiltinGroupRolePermissionsByID(roleID string) []WorkspacePermissionCode {
	switch roleID {
	case WorkspaceBuiltinRoleOwnerID:
		return AllWorkspacePermissionCodes()
	case WorkspaceBuiltinRoleAdminID:
		return []WorkspacePermissionCode{
			WorkspacePermissionWorkspaceView,
			WorkspacePermissionWorkspaceManage,
			WorkspacePermissionWorkspaceBillingAudit,

			WorkspacePermissionAgentView,
			WorkspacePermissionAgentManage,
			WorkspacePermissionAgentLock,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionKnowledgeBaseManage,
			WorkspacePermissionKnowledgeBaseRetrievalTest,
			WorkspacePermissionKnowledgeBaseFolderManage,
			WorkspacePermissionKnowledgeBaseLock,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionDatabaseManage,
			WorkspacePermissionDatabaseDataEdit,
			WorkspacePermissionDatabaseAIQuery,
			WorkspacePermissionDatabaseLock,
			WorkspacePermissionFileView,
			WorkspacePermissionFileManage,
			WorkspacePermissionFileUploadCreate,
			WorkspacePermissionFileDownload,
			WorkspacePermissionFileMoveCreate,
		}
	case WorkspaceBuiltinRoleMemberID:
		return []WorkspacePermissionCode{
			WorkspacePermissionWorkspaceView,

			WorkspacePermissionAgentView,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionKnowledgeBaseRetrievalTest,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionDatabaseAIQuery,
			WorkspacePermissionFileView,
			WorkspacePermissionFileUploadCreate,
			WorkspacePermissionFileDownload,
		}
	case WorkspaceBuiltinRoleViewerID:
		return []WorkspacePermissionCode{
			WorkspacePermissionWorkspaceView,

			WorkspacePermissionAgentView,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionFileView,
		}
	default:
		return nil
	}
}

// WorkspaceCustomRoleStatus workspace custom role status enum
type WorkspaceCustomRoleStatus string

const (
	WorkspaceCustomRoleStatusActive   WorkspaceCustomRoleStatus = "active"
	WorkspaceCustomRoleStatusInactive WorkspaceCustomRoleStatus = "inactive"
	WorkspaceCustomRoleStatusArchived WorkspaceCustomRoleStatus = "archived"
	WorkspaceCustomRoleStatusDeleted  WorkspaceCustomRoleStatus = "deleted"
)

type WorkspaceCustomRole struct {
	ID             string                    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID string                    `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`
	Name           string                    `gorm:"type:varchar(255);not null" json:"name"`
	Description    *string                   `gorm:"type:text" json:"description,omitempty"`
	Status         WorkspaceCustomRoleStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	Permissions    []string                  `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"permissions"`
	CreatedBy      string                    `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

func (WorkspaceCustomRole) TableName() string {
	return "roles"
}

func (r *WorkspaceCustomRole) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	return nil
}

func (r *WorkspaceCustomRole) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = time.Now()
	return nil
}

// WorkspaceCustomRolePermission removed as permissions are now in roles table

// OrganizationMemberStatus enterprise group account status enum
type OrganizationMemberStatus string

const (
	OrganizationMemberStatusActive   OrganizationMemberStatus = "active"
	OrganizationMemberStatusInactive OrganizationMemberStatus = "inactive"
)

// OrganizationMember enterprise group account association
type OrganizationMember struct {
	OrganizationID string                   `gorm:"type:varchar(255);not null;primaryKey;index" json:"organization_id"`
	AccountID      string                   `gorm:"type:varchar(255);not null;primaryKey;index" json:"account_id"`
	Role           OrganizationRole         `gorm:"type:varchar(16);not null" json:"role"`
	Name           *string                  `gorm:"type:varchar(255)" json:"name"`
	Status         OrganizationMemberStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`

	// Relationships - commented out for modular architecture
	// Group   Organization `gorm:"foreignKey:GroupID" json:"-"`
	// Account Account      `gorm:"foreignKey:AccountID" json:"-"`
}

// TableName specifies table name
func (OrganizationMember) TableName() string {
	return "members"
}

// IsAdmin checks if it's an admin role
func (om *OrganizationMember) IsAdmin() bool {
	return om.Role == OrganizationRoleAdmin
}

// BeforeCreate hook to set timestamps
func (om *OrganizationMember) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if om.CreatedAt.IsZero() {
		om.CreatedAt = now
	}
	if om.UpdatedAt.IsZero() {
		om.UpdatedAt = now
	}
	return nil
}

// OrganizationInviteLink organization invite link model
type OrganizationInviteLink struct {
	ID             string `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID string `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`

	DepartmentID *string `gorm:"type:uuid" json:"department_id,omitempty"`
	WorkspaceID  *string `gorm:"column:tenant_id;type:uuid" json:"workspace_id,omitempty"`

	Token string `gorm:"type:varchar(255);not null;uniqueIndex" json:"token"`

	Status string `gorm:"type:varchar(32);not null" json:"status"`

	RequireApproval         bool   `gorm:"not null;default:true" json:"require_approval"`
	DefaultOrganizationRole string `gorm:"column:default_group_role;type:varchar(32);not null;default:'normal'" json:"default_organization_role"`
	DefaultWorkspaceRole    string `gorm:"column:default_tenant_role;type:varchar(32);not null;default:'normal'" json:"default_workspace_role"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	CreatedBy string    `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies table name
func (OrganizationInviteLink) TableName() string {
	return "organization_invite_links"
}

// BeforeCreate hook to set ID and timestamps
func (e *OrganizationInviteLink) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	return nil
}

// OrganizationJoinRequestStatus join request status enum
type OrganizationJoinRequestStatus string

const (
	OrganizationJoinRequestStatusPending  OrganizationJoinRequestStatus = "pending"
	OrganizationJoinRequestStatusApproved OrganizationJoinRequestStatus = "approved"
	OrganizationJoinRequestStatusRejected OrganizationJoinRequestStatus = "rejected"
	OrganizationJoinRequestStatusExpired  OrganizationJoinRequestStatus = "expired"
)

// OrganizationJoinRequest organization join request model
type OrganizationJoinRequest struct {
	ID string `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`

	OrganizationID string  `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`
	InviteLinkID   *string `gorm:"type:uuid" json:"invite_link_id,omitempty"`
	AccountID      string  `gorm:"type:uuid;not null;index" json:"account_id"`

	DepartmentID *string `gorm:"type:uuid" json:"department_id,omitempty"`
	WorkspaceID  *string `gorm:"column:tenant_id;type:uuid" json:"workspace_id,omitempty"`

	DefaultOrganizationRole string `gorm:"column:default_group_role;type:varchar(32);not null" json:"default_organization_role"`
	DefaultWorkspaceRole    string `gorm:"column:default_tenant_role;type:varchar(32);not null" json:"default_workspace_role"`

	Name       *string                       `gorm:"type:varchar(255)" json:"name"`
	Status     OrganizationJoinRequestStatus `gorm:"type:varchar(32);not null" json:"status"`
	Reason     *string                       `gorm:"type:text" json:"reason,omitempty"`
	ReviewerID *string                       `gorm:"type:uuid" json:"reviewer_id,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

// TableName specifies table name
func (OrganizationJoinRequest) TableName() string {
	return "organization_join_requests"
}

// BeforeCreate hook to set ID and timestamps
func (e *OrganizationJoinRequest) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}
