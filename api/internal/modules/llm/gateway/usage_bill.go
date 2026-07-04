package gateway

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	usageBillStatusSuccess = "success"
	usageBillStatusFailed  = "failed"
	usageBillStatusPartial = "partial"
)

type UsageBill struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey"`
	AttemptID         string           `gorm:"column:attempt_id;size:120;not null;uniqueIndex"`
	RequestID         string           `gorm:"column:request_id;size:100;not null;index"`
	OrganizationID    string           `gorm:"column:organization_id;type:uuid;not null;index:idx_usage_bills_org_created,priority:1;index:idx_usage_bills_org_model_created,priority:1;index:idx_usage_bills_org_app_type_created,priority:1;index:idx_usage_bills_org_app_created,priority:1;index:idx_usage_bills_org_source_created,priority:1;index:idx_usage_bills_org_lane_created,priority:1"`
	AppID             *uuid.UUID       `gorm:"column:app_id;type:uuid;index:idx_usage_bills_org_app_created,priority:2"`
	AppType           *string          `gorm:"column:app_type;type:varchar(50);index:idx_usage_bills_org_app_type_created,priority:2"`
	WorkspaceID       *string          `gorm:"column:workspace_id;type:varchar(255)"`
	APIKeyID          string           `gorm:"column:api_key_id;type:uuid"`
	QuotaSubjectType  *string          `gorm:"column:quota_subject_type;type:varchar(20)"`
	QuotaSubjectID    *string          `gorm:"column:quota_subject_id;type:varchar(255)"`
	ModelID           uuid.UUID        `gorm:"column:model_id;type:uuid;not null"`
	ModelName         string           `gorm:"column:model_name;type:varchar(100);not null;index:idx_usage_bills_org_model_created,priority:2"`
	ProviderID        uuid.UUID        `gorm:"column:provider_id;type:uuid;not null"`
	ProviderName      string           `gorm:"column:provider_name;type:varchar(100);not null"`
	RouteID           *uuid.UUID       `gorm:"column:route_id;type:uuid"`
	ChannelID         *uuid.UUID       `gorm:"column:channel_id;type:uuid"`
	BillingLane       UsageBillingLane `gorm:"column:billing_lane;type:varchar(20);not null;default:'private';index:idx_usage_bills_org_lane_created,priority:2"`
	RemoteDeductionID *string          `gorm:"column:remote_deduction_id;type:varchar(120);index"`
	UseSystemProvider bool             `gorm:"column:use_system_provider;not null;default:false;index:idx_usage_bills_org_source_created,priority:2"`
	Status            string           `gorm:"column:status;type:varchar(20);not null;index"`
	PromptTokens      int64            `gorm:"column:prompt_tokens;not null;default:0"`
	CompletionTokens  int64            `gorm:"column:completion_tokens;not null;default:0"`
	TotalTokens       int64            `gorm:"column:total_tokens;not null;default:0"`
	OfficialPoints    int64            `gorm:"column:official_points;not null;default:0"`
	PrivatePoints     int64            `gorm:"column:private_points;not null;default:0"`
	TotalPoints       int64            `gorm:"column:total_points;not null;default:0"`
	PricingSource     PricingSource    `gorm:"column:pricing_source;type:varchar(50);not null;default:''"`
	UsageSource       UsageSource      `gorm:"column:usage_source;type:varchar(50);not null;default:''"`
	PricingSnapshot   datatypes.JSON   `gorm:"column:pricing_snapshot;type:jsonb;not null;default:'{}'"`
	ResponseTimeMS    int64            `gorm:"column:response_time_ms;not null;default:0"`
	ErrorCode         *string          `gorm:"column:error_code;type:varchar(100)"`
	ErrorMessage      *string          `gorm:"column:error_message;type:text"`
	RequestCreatedAt  time.Time        `gorm:"column:request_created_at;not null;index:idx_usage_bills_org_created,priority:2;index:idx_usage_bills_org_model_created,priority:3;index:idx_usage_bills_org_app_type_created,priority:3;index:idx_usage_bills_org_app_created,priority:3;index:idx_usage_bills_org_source_created,priority:3;index:idx_usage_bills_org_lane_created,priority:3"`
	SettledAt         time.Time        `gorm:"column:settled_at;not null"`
}

func (UsageBill) TableName() string {
	return "llm_usage_bills"
}

func (b *UsageBill) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	lane, err := normalizeUsageBillingLane(b.BillingLane, b.UseSystemProvider)
	if err != nil {
		return err
	}
	b.BillingLane = lane
	b.UseSystemProvider = usageBillingLaneUsesSystemProvider(lane)
	return nil
}
