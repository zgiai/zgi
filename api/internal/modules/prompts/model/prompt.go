package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type PromptSource string

const (
	PromptSourceOfficial  PromptSource = "official"
	PromptSourceWorkspace PromptSource = "workspace"
	PromptSourcePersonal  PromptSource = "personal"
)

type PromptType string

const (
	PromptTypeText PromptType = "text"
	PromptTypeChat PromptType = "chat"
)

type Prompt struct {
	ID             string          `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID *string         `gorm:"type:uuid;index:idx_app_prompts_org_workspace_source,priority:1" json:"organization_id,omitempty"`
	WorkspaceID    *string         `gorm:"type:uuid;index:idx_app_prompts_org_workspace_source,priority:2" json:"workspace_id,omitempty"`
	OwnerAccountID *string         `gorm:"type:uuid;index" json:"owner_account_id,omitempty"`
	Source         PromptSource    `gorm:"type:varchar(32);not null;index:idx_app_prompts_org_workspace_source,priority:3" json:"source"`
	Name           string          `gorm:"type:varchar(255);not null" json:"name"`
	Slug           string          `gorm:"type:varchar(255);not null;index" json:"slug"`
	Description    *string         `gorm:"type:text" json:"description,omitempty"`
	Locale         string          `gorm:"type:varchar(32);not null;default:'zh-Hans';index" json:"locale"`
	Category       *string         `gorm:"type:varchar(128);index" json:"category,omitempty"`
	Tags           []string        `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"tags"`
	LatestVersion  int             `gorm:"not null;default:1" json:"latest_version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Versions       []PromptVersion `gorm:"foreignKey:PromptID;references:ID" json:"versions,omitempty"`
}

func (Prompt) TableName() string {
	return "app_prompts"
}

func (p *Prompt) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	return nil
}

type PromptVersion struct {
	ID            string         `gorm:"type:uuid;primaryKey" json:"id"`
	PromptID      string         `gorm:"type:uuid;not null;index:idx_app_prompt_versions_prompt_version,priority:1" json:"prompt_id"`
	Version       int            `gorm:"not null;index:idx_app_prompt_versions_prompt_version,priority:2" json:"version"`
	PromptType    PromptType     `gorm:"type:varchar(16);not null" json:"prompt_type"`
	Content       datatypes.JSON `gorm:"type:jsonb;not null" json:"content"`
	Config        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"config"`
	Labels        []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"labels"`
	CommitMessage *string        `gorm:"type:text" json:"commit_message,omitempty"`
	CreatedBy     *string        `gorm:"type:uuid" json:"created_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (PromptVersion) TableName() string {
	return "app_prompt_versions"
}

func (p *PromptVersion) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	return nil
}

type PromptOptimizationRun struct {
	ID                     string     `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         string     `gorm:"type:uuid;not null;index:idx_prompt_opt_runs_org_workspace_created,priority:1" json:"organization_id"`
	WorkspaceID            *string    `gorm:"type:uuid;index:idx_prompt_opt_runs_org_workspace_created,priority:2" json:"workspace_id,omitempty"`
	PromptID               *string    `gorm:"type:uuid;index:idx_prompt_opt_runs_account_prompt_created,priority:2" json:"prompt_id,omitempty"`
	AccountID              string     `gorm:"type:uuid;not null;index:idx_prompt_opt_runs_account_prompt_created,priority:1" json:"account_id"`
	Goal                   string     `gorm:"type:varchar(32);not null;default:'general'" json:"goal"`
	Provider               *string    `gorm:"type:varchar(128)" json:"provider,omitempty"`
	Model                  *string    `gorm:"type:varchar(255)" json:"model,omitempty"`
	PreserveVariables      bool       `gorm:"not null;default:true" json:"preserve_variables"`
	DetectedVariables      []string   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"detected_variables"`
	RawPrompt              string     `gorm:"type:text;not null" json:"raw_prompt"`
	SafeOutput             string     `gorm:"type:text;not null" json:"safe_output"`
	BalancedOutput         string     `gorm:"type:text;not null" json:"balanced_output"`
	AdvancedOutput         string     `gorm:"type:text;not null" json:"advanced_output"`
	AdoptedVariant         *string    `gorm:"type:varchar(16)" json:"adopted_variant,omitempty"`
	AdoptedPromptVersionID *string    `gorm:"type:uuid" json:"adopted_prompt_version_id,omitempty"`
	AdoptedAt              *time.Time `json:"adopted_at,omitempty"`
	CreatedAt              time.Time  `gorm:"index:idx_prompt_opt_runs_account_prompt_created,priority:3;index:idx_prompt_opt_runs_org_workspace_created,priority:3" json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (PromptOptimizationRun) TableName() string {
	return "app_prompt_optimization_runs"
}

func (r *PromptOptimizationRun) BeforeCreate(tx *gorm.DB) error {
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
