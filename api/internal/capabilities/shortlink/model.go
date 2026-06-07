package shortlink

import "time"

const (
	TargetKindApprovalForm         = "approval_form"
	TargetKindWorkflowAnnouncement = "workflow_announcement"
)

type ShortLink struct {
	ID          string     `gorm:"type:uuid;primaryKey" json:"id"`
	ShortToken  string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_system_short_links_token" json:"short_token"`
	TargetKind  string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_system_short_links_target" json:"target_kind"`
	TargetToken string     `gorm:"type:varchar(128);not null;uniqueIndex:idx_system_short_links_target" json:"target_token"`
	TargetPath  string     `gorm:"type:varchar(512);not null" json:"target_path"`
	ExpiresAt   *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (ShortLink) TableName() string {
	return "system_short_links"
}

type CreateOrGetRequest struct {
	TargetKind  string
	TargetToken string
	TargetPath  string
	ExpiresAt   *time.Time
}
