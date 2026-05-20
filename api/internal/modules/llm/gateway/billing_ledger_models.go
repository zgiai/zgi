package gateway

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	quotaSubjectTypeAPIKey       = "key"
	quotaSubjectTypeWorkspace    = "workspace"
	quotaSubjectTypeOrganization = "organization"

	billingAttemptLaneRemote            = "remote"
	billingAttemptLaneLocal             = "local"
	billingAttemptStatusInit            = "INIT"
	billingAttemptStatusPre             = "PREDEDUCTED"
	billingAttemptStatusSettlePending   = "SETTLE_PENDING"
	billingAttemptStatusSettled         = "SETTLED"
	billingAttemptStatusRolledBack      = "ROLLED_BACK"
	billingAttemptStatusPartial         = "PARTIAL_SETTLED"
	billingAttemptStatusPredeductFailed = "PREDEDUCT_FAILED"
	billingAttemptStatusDeadLetter      = "DEAD_LETTER"

	billingEntryTypeSubject = "subject"
	billingEntryTypeFund    = "fund"

	billingLedgerTypeAPIKeyQuota   = "key_quota"
	billingLedgerTypeOrgFunds      = "org_funds"
	billingLedgerTypeChannelWallet = "channel_wallet"

	billingEntryStatusPending = "PENDING"
	billingEntryStatusSettled = "SETTLED"
	billingEntryStatusRolled  = "ROLLED_BACK"
	billingEntryStatusFailed  = "FAILED"

	channelWalletStatusActive = "ACTIVE"
	channelWalletStatusDebt   = "DEBT"

	channelWalletTxTypePreDeduct        = "prededuct"
	channelWalletTxTypeSettleAdjustment = "settle_adjustment"
	channelWalletTxTypeRefund           = "refund"
	channelWalletTxTypeRollback         = "rollback"

	billingPhasePreDeduct = "prededuct"
	billingPhaseSettle    = "settle"
)

type BillingAttempt struct {
	AttemptID         string     `gorm:"column:attempt_id;primaryKey;size:120"`
	RequestID         string     `gorm:"column:request_id;size:100;not null;index"`
	OrganizationID    uuid.UUID  `gorm:"column:organization_id;type:uuid;not null;index"`
	Lane              string     `gorm:"column:lane;size:20;not null"`
	RouteID           *uuid.UUID `gorm:"column:route_id;type:uuid"`
	ProviderID        *uuid.UUID `gorm:"column:provider_id;type:uuid"`
	ModelID           *uuid.UUID `gorm:"column:model_id;type:uuid"`
	QuotaSubjectType  string     `gorm:"column:quota_subject_type;size:20;not null"`
	QuotaSubjectID    string     `gorm:"column:quota_subject_id;size:64;not null"`
	Status            string     `gorm:"column:status;size:30;not null;index"`
	InvocationResult  *string    `gorm:"column:invocation_result;size:20"`
	ErrorCode         *string    `gorm:"column:error_code;size:100"`
	ErrorMessage      *string    `gorm:"column:error_message;type:text"`
	ReconcileAttempts int        `gorm:"column:reconcile_attempts;not null;default:0"`
	NextReconcileAt   *time.Time `gorm:"column:next_reconcile_at"`
	LastReconcileAt   *time.Time `gorm:"column:last_reconcile_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
}

func (BillingAttempt) TableName() string {
	return "billing_attempts"
}

type BillingAttemptEntry struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	AttemptID      string    `gorm:"column:attempt_id;size:120;not null;index"`
	EntryType      string    `gorm:"column:entry_type;size:20;not null"`
	LedgerType     string    `gorm:"column:ledger_type;size:30;not null"`
	LedgerRefID    string    `gorm:"column:ledger_ref_id;size:120;not null"`
	ReservedAmount int64     `gorm:"column:reserved_amount;not null;default:0"`
	ActualAmount   int64     `gorm:"column:actual_amount;not null;default:0"`
	RefundedAmount int64     `gorm:"column:refunded_amount;not null;default:0"`
	Status         string    `gorm:"column:status;size:20;not null;index"`
	ErrorCode      *string   `gorm:"column:error_code;size:100"`
	ErrorMessage   *string   `gorm:"column:error_message;type:text"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:160"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (BillingAttemptEntry) TableName() string {
	return "billing_attempt_entries"
}

func (e *BillingAttemptEntry) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

type ChannelWallet struct {
	ChannelID      uuid.UUID `gorm:"column:channel_id;type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"column:organization_id;type:uuid;not null;index"`
	Balance        int64     `gorm:"column:balance;not null;default:0"`
	Status         string    `gorm:"column:status;size:20;not null;default:'ACTIVE';index"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (ChannelWallet) TableName() string {
	return "channel_wallets"
}

type ChannelWalletTransaction struct {
	ID            uuid.UUID              `gorm:"column:id;type:uuid;primaryKey"`
	ChannelID     uuid.UUID              `gorm:"column:channel_id;type:uuid;not null;index"`
	AttemptID     *string                `gorm:"column:attempt_id;size:120;index"`
	Type          string                 `gorm:"column:type;size:40;not null"`
	Amount        int64                  `gorm:"column:amount;not null"`
	BalanceBefore int64                  `gorm:"column:balance_before;not null"`
	BalanceAfter  int64                  `gorm:"column:balance_after;not null"`
	Metadata      map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json"`
	CreatedAt     time.Time              `gorm:"column:created_at;not null"`
}

func (ChannelWalletTransaction) TableName() string {
	return "channel_wallet_transactions"
}

func (tx *ChannelWalletTransaction) BeforeCreate(db *gorm.DB) error {
	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}
	return nil
}

// WorkspaceQuota stores LLM quota limits and usage for workspace-subject billing.
type WorkspaceQuota struct {
	WorkspaceID    string    `gorm:"column:workspace_id;type:varchar(255);primaryKey"`
	OrganizationID uuid.UUID `gorm:"column:organization_id;type:uuid;not null;index"`
	UsedQuota      int64     `gorm:"column:used_quota;not null;default:0"`
	RemainQuota    int64     `gorm:"column:remain_quota;not null;default:0"`
	QuotaLimit     *int64    `gorm:"column:quota_limit"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (WorkspaceQuota) TableName() string {
	return "llm_workspace_quotas"
}
