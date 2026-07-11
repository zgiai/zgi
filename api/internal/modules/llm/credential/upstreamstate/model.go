package upstreamstate

import (
	"time"

	"github.com/google/uuid"
)

type BalanceCapability string
type Availability string
type ObservationSource string
type CheckStatus string
type GuardReason string

const (
	BalanceCapabilityUnknown          BalanceCapability = "unknown"
	BalanceCapabilitySupported        BalanceCapability = "supported"
	BalanceCapabilityUnsupported      BalanceCapability = "unsupported"
	BalanceCapabilityPermissionDenied BalanceCapability = "permission_denied"

	AvailabilityUnknown    Availability = "unknown"
	AvailabilityAvailable  Availability = "available"
	AvailabilityExhausted  Availability = "exhausted"
	AvailabilityInvalidKey Availability = "invalid_key"

	ObservationSourceBalanceAPI    ObservationSource = "balance_api"
	ObservationSourceProviderError ObservationSource = "provider_error"

	CheckStatusUnknown     CheckStatus = "unknown"
	CheckStatusSuccess     CheckStatus = "success"
	CheckStatusFailed      CheckStatus = "failed"
	CheckStatusUnsupported CheckStatus = "unsupported"

	GuardReasonBalanceExhausted   GuardReason = "balance_exhausted"
	GuardReasonQuotaExhausted     GuardReason = "quota_exhausted"
	GuardReasonBillingUnavailable GuardReason = "billing_unavailable"
	GuardReasonAuthInvalid        GuardReason = "auth_invalid"
)

type BalanceAmount struct {
	Currency  string `json:"currency"`
	Remaining string `json:"remaining"`
}

type BalanceSnapshot struct {
	Scope       string          `json:"scope"`
	Items       []BalanceAmount `json:"items"`
	Spendable   *bool           `json:"spendable,omitempty"`
	IsUnlimited bool            `json:"is_unlimited,omitempty"`
}

type WarningThreshold struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

type State struct {
	CredentialID   uuid.UUID `gorm:"column:credential_id;type:uuid;primaryKey" json:"credential_id"`
	OrganizationID uuid.UUID `gorm:"column:organization_id;type:uuid;not null;index" json:"organization_id"`
	Generation     int64     `gorm:"not null;default:1" json:"generation"`

	BalanceCapability BalanceCapability  `gorm:"type:varchar(32);not null;default:'unknown'" json:"balance_capability"`
	BalanceSnapshot   *BalanceSnapshot   `gorm:"type:jsonb;serializer:json" json:"balance_snapshot,omitempty"`
	BalanceObservedAt *time.Time         `gorm:"type:timestamptz" json:"balance_observed_at,omitempty"`
	WarningThresholds []WarningThreshold `gorm:"type:jsonb;serializer:json;default:'[]'" json:"warning_thresholds"`

	Availability           Availability      `gorm:"type:varchar(32);not null;default:'unknown'" json:"availability"`
	ObservationSource      ObservationSource `gorm:"type:varchar(32)" json:"observation_source,omitempty"`
	AvailabilityObservedAt *time.Time        `gorm:"type:timestamptz" json:"availability_observed_at,omitempty"`

	LastCheckAt         *time.Time  `gorm:"type:timestamptz" json:"last_check_at,omitempty"`
	LastCheckStatus     CheckStatus `gorm:"type:varchar(32);not null;default:'unknown'" json:"last_check_status"`
	LastCheckErrorKind  string      `gorm:"type:varchar(64)" json:"last_check_error_kind,omitempty"`
	NextCheckAt         *time.Time  `gorm:"type:timestamptz;index" json:"next_check_at,omitempty"`
	CheckLeaseUntil     *time.Time  `gorm:"type:timestamptz" json:"-"`
	ConsecutiveFailures int         `gorm:"not null;default:0" json:"consecutive_failures"`

	BlockReason            GuardReason `gorm:"type:varchar(32)" json:"block_reason,omitempty"`
	CooldownUntil          *time.Time  `gorm:"type:timestamptz" json:"cooldown_until,omitempty"`
	GuardStrikes           int         `gorm:"not null;default:0" json:"guard_strikes"`
	HalfOpenLeaseUntil     *time.Time  `gorm:"type:timestamptz" json:"-"`
	ManualRetryRequestedAt *time.Time  `gorm:"type:timestamptz" json:"manual_retry_requested_at,omitempty"`
	ProviderErrorCode      string      `gorm:"type:varchar(128)" json:"provider_error_code,omitempty"`
	ProviderErrorStatus    int         `gorm:"not null;default:0" json:"provider_error_status,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (State) TableName() string {
	return "llm_credential_upstream_states"
}
