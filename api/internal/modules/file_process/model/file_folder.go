package model

import (
	"time"
)

// FileFolder represents a folder that can contain files
type FileFolder struct {
	ID             string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null;index:file_folder_organization_idx"`
	WorkspaceID    *string   `json:"workspace_id" gorm:"type:uuid;index:file_folder_workspace_idx"`
	Name           string    `json:"name" gorm:"type:varchar(255);not null"`
	Description    *string   `json:"description" gorm:"type:text"`
	ParentID       *string   `json:"parent_id" gorm:"type:uuid;index:file_folder_parent_idx"`
	CreatedBy      string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	UpdatedBy      *string   `json:"updated_by" gorm:"type:uuid"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	IconType       *string   `gorm:"type:varchar(255)" json:"icon_type"`
	Icon           *string   `json:"icon" gorm:"type:varchar(255)"`
	IconBackground *string   `json:"icon_background" gorm:"type:varchar(255)"`
	Position       int       `json:"position" gorm:"type:integer;not null;default:0"`
	Permission     string    `json:"permission" gorm:"type:varchar(255);not null;default:'only_me'"` // File folder permission, see FileFolderPermissionType constants
}

// TableName specifies table name
func (FileFolder) TableName() string {
	return "file_folders"
}

// FileFolderPermissionType represents the permission levels for file folders
type FileFolderPermissionType string

const (
	// FileFolderPermissionAllTeam allows all team members to access the folder
	FileFolderPermissionAllTeam FileFolderPermissionType = "all_team"

	// FileFolderPermissionOnlyMe restricts access to only the folder creator
	FileFolderPermissionOnlyMe FileFolderPermissionType = "only_me"

	// FileFolderPermissionPartialTeam allows access to specified team members or departments
	FileFolderPermissionPartialTeam FileFolderPermissionType = "partial_team"
)

// IsValidFileFolderPermission checks if a given string is a valid file folder permission
func IsValidFileFolderPermission(permission string) bool {
	switch FileFolderPermissionType(permission) {
	case FileFolderPermissionAllTeam, FileFolderPermissionOnlyMe, FileFolderPermissionPartialTeam:
		return true
	default:
		return false
	}
}

// FileFolderPermission represents the permission relationship between file folders and tenants
// This model is used when FileFolder.Permission is set to "partial_team"
type FileFolderPermission struct {
	ID          string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	FolderID    string    `json:"folder_id" gorm:"type:uuid;not null;index:file_folder_permission_folder_idx"`
	WorkspaceID string    `json:"workspace_id" gorm:"type:uuid;not null;index:file_folder_permission_tenant_idx"`
	CreatedBy   string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

// TableName specifies table name
func (FileFolderPermission) TableName() string {
	return "file_folder_permissions"
}

// FileFolderJoins represents the many-to-many relationship between files and folders
type FileFolderJoins struct {
	ID        string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	FileID    string    `json:"file_id" gorm:"type:uuid;not null;index:file_folder_assoc_file_idx"`
	FolderID  string    `json:"folder_id" gorm:"type:uuid;not null;index:file_folder_assoc_folder_idx"`
	CreatedBy string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

// TableName specifies table name
func (FileFolderJoins) TableName() string {
	return "file_folder_joins"
}
