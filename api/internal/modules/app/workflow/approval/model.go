package approval

import "time"

type Form struct {
	ID                     string     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID               string     `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID                  string     `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID          string     `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	NodeID                 string     `gorm:"type:varchar(255);not null;index" json:"node_id"`
	NodeTitle              string     `gorm:"type:varchar(255)" json:"node_title"`
	FormDefinition         string     `gorm:"type:text;not null" json:"form_definition"`
	RenderedContent        string     `gorm:"type:text;not null" json:"rendered_content"`
	Status                 string     `gorm:"type:varchar(32);not null;default:'waiting';index" json:"status"`
	ExpirationTime         time.Time  `gorm:"not null;index" json:"expiration_time"`
	SelectedActionID       *string    `gorm:"type:varchar(200)" json:"selected_action_id"`
	SubmittedData          *string    `gorm:"type:text" json:"submitted_data"`
	SubmittedAt            *time.Time `json:"submitted_at"`
	SubmissionUserID       *string    `gorm:"type:uuid" json:"submission_user_id"`
	SubmissionEndUserID    *string    `gorm:"type:uuid" json:"submission_end_user_id"`
	CompletedByRecipientID *string    `gorm:"type:uuid" json:"completed_by_recipient_id"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (Form) TableName() string {
	return "workflow_approval_forms"
}

type Delivery struct {
	ID                 string     `gorm:"type:uuid;primaryKey" json:"id"`
	FormID             string     `gorm:"type:uuid;not null;index" json:"form_id"`
	DeliveryMethodType string     `gorm:"type:varchar(32);not null" json:"delivery_method_type"`
	DeliveryConfigID   *string    `gorm:"type:uuid" json:"delivery_config_id"`
	ChannelPayload     string     `gorm:"type:text;not null" json:"channel_payload"`
	LastError          *string    `gorm:"type:text" json:"last_error"`
	SentAt             *time.Time `json:"sent_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (Delivery) TableName() string {
	return "workflow_approval_deliveries"
}

type Recipient struct {
	ID               string    `gorm:"type:uuid;primaryKey" json:"id"`
	FormID           string    `gorm:"type:uuid;not null;index" json:"form_id"`
	DeliveryID       string    `gorm:"type:uuid;not null;index" json:"delivery_id"`
	RecipientType    string    `gorm:"type:varchar(64);not null" json:"recipient_type"`
	RecipientPayload string    `gorm:"type:text;not null" json:"recipient_payload"`
	AccessToken      string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"access_token"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Recipient) TableName() string {
	return "workflow_approval_recipients"
}

type RunEvent struct {
	ID            string    `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID      string    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	AppID         string    `gorm:"type:uuid;not null;index" json:"app_id"`
	WorkflowRunID string    `gorm:"type:varchar(255);not null;index" json:"workflow_run_id"`
	Sequence      int       `gorm:"not null;index" json:"sequence"`
	EventType     string    `gorm:"type:varchar(100);not null" json:"event_type"`
	EventData     string    `gorm:"type:text;not null" json:"event_data"`
	CreatedAt     time.Time `json:"created_at"`
}

func (RunEvent) TableName() string {
	return "workflow_run_events"
}
