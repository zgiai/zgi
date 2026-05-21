package announcement

import "time"

type Announcement struct {
	ID              string    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID           string    `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID   string    `gorm:"type:varchar(255);not null;index;uniqueIndex:idx_workflow_announcements_run_node" json:"workflow_run_id"`
	NodeID          string    `gorm:"type:varchar(255);not null;index;uniqueIndex:idx_workflow_announcements_run_node" json:"node_id"`
	NodeTitle       string    `gorm:"type:varchar(255)" json:"node_title"`
	Content         string    `gorm:"type:text;not null" json:"content"`
	RenderedContent string    `gorm:"type:text;not null" json:"rendered_content"`
	AccessToken     string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"access_token"`
	ExpirationTime  time.Time `gorm:"not null;index" json:"expiration_time"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Announcement) TableName() string {
	return "workflow_announcements"
}
