package model

import (
	"time"
)

// DatasetFolder represents a folder that can contain datasets
type DatasetFolder struct {
	ID             string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null;index:dataset_folder_organization_idx"`
	WorkspaceID    string    `json:"workspace_id" gorm:"type:uuid;index:dataset_folder_workspace_idx"`
	Name           string    `json:"name" gorm:"type:varchar(255);not null"`
	Description    *string   `json:"description" gorm:"type:text"`
	ParentID       *string   `json:"parent_id" gorm:"type:uuid;index:dataset_folder_parent_idx"`
	CreatedBy      string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	UpdatedBy      *string   `json:"updated_by" gorm:"type:uuid"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	IconType       *string   `gorm:"type:varchar(255)" json:"icon_type"`
	Icon           *string   `json:"icon" gorm:"type:varchar(255)"`
	IconBackground *string   `json:"icon_background" gorm:"type:varchar(255)"`
	Position       int       `json:"position" gorm:"type:integer;not null;default:0"`
	Permission     string    `json:"permission" gorm:"type:varchar(255);not null;default:'only_me'"`
}

// TableName specifies table name
func (DatasetFolder) TableName() string {
	return "dataset_folders"
}

// DatasetFolderJoins represents the many-to-many relationship between datasets and folders
type DatasetFolderJoins struct {
	ID        string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DatasetID string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_folder_assoc_dataset_idx"`
	FolderID  string    `json:"folder_id" gorm:"type:uuid;not null;index:dataset_folder_assoc_folder_idx"`
	CreatedBy string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

// TableName specifies table name
func (DatasetFolderJoins) TableName() string {
	return "dataset_folder_joins"
}
