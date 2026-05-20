package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CurrencyType represents the type of currency in transactions
type CurrencyType string

const (
	CurrencyTypeCash                CurrencyType = "cash"
	CurrencyTypeSubscriptionCredits CurrencyType = "subscription_credits"
	CurrencyTypePurchasedCredits    CurrencyType = "purchased_credits"
)

// TransactionType represents the type of transaction (5 categories)
type TransactionType string

const (
	// TransactionTypeRechargePurchase - Recharge & Purchase (wallet recharge, credits package purchase)
	TransactionTypeRechargePurchase TransactionType = "recharge_purchase"
	// TransactionTypeSubscription - Subscription (new, renewal, plan change)
	TransactionTypeSubscription TransactionType = "subscription"
	// TransactionTypeAIConsumption - AI Consumption (model calls, AI value-added services)
	TransactionTypeAIConsumption TransactionType = "ai_consumption"
	// TransactionTypeBudgetAdjustment - Budget Adjustment (allocation, reclaim)
	TransactionTypeBudgetAdjustment TransactionType = "budget_adjustment"
	// TransactionTypeOther - Other (refund, expiration, system compensation, reward)
	TransactionTypeOther TransactionType = "other"
)

// TransactionTypeLabel returns the display label for a transaction type
func TransactionTypeLabel(txType TransactionType) string {
	switch txType {
	case TransactionTypeRechargePurchase:
		return "充值购买"
	case TransactionTypeSubscription:
		return "订阅服务"
	case TransactionTypeAIConsumption:
		return "AI消耗"
	case TransactionTypeBudgetAdjustment:
		return "预算调整"
	case TransactionTypeOther:
		return "其他"
	default:
		return string(txType)
	}
}

// ReferenceType constants
const (
	ReferenceTypeOrder        = "order"
	ReferenceTypeSubscription = "subscription"
	ReferenceTypeRefund       = "refund"
	ReferenceTypeConversation = "conversation"
)

// Transaction represents a unified transaction record for both cash and credits
type Transaction struct {
	ID                string                 `gorm:"type:varchar(255);primaryKey" json:"id"`                        // Format: TXN-xxxxxxxx
	BatchID           string                 `gorm:"type:varchar(255);not null;index:idx_tx_batch" json:"batch_id"` // Format: BAT-xxxxxxxx, same batch for related transactions
	GroupID           uuid.UUID              `gorm:"type:uuid;not null;index:idx_tx_group;index:idx_tx_group_currency" json:"group_id"`
	TenantID          *uuid.UUID             `gorm:"type:uuid;index:idx_tx_tenant" json:"tenant_id,omitempty"`                   // Department ID for AI usage
	CurrencyType      string                 `gorm:"type:varchar(20);not null;index:idx_tx_group_currency" json:"currency_type"` // cash / subscription_credits / purchased_credits
	Type              string                 `gorm:"column:type;type:varchar(30);index:idx_tx_type" json:"type"`                 // recharge_purchase, subscription, ai_consumption, budget_adjustment, other (nullable, legacy)
	TransactionType   string                 `gorm:"type:varchar(30);not null;index:idx_tx_type" json:"transaction_type"`        // recharge_purchase, subscription, ai_consumption, budget_adjustment, other
	Amount            float64                `gorm:"type:decimal(16,4);not null" json:"amount"`                                  // Positive for income, negative for expense
	BalanceBefore     float64                `gorm:"type:decimal(16,4);not null" json:"balance_before"`
	BalanceAfter      float64                `gorm:"type:decimal(16,4);not null" json:"balance_after"`
	Currency          *string                `gorm:"type:varchar(10)" json:"currency,omitempty"`                              // Only for cash type: CNY/USD
	ReferenceType     *string                `gorm:"type:varchar(50);index:idx_tx_reference" json:"reference_type,omitempty"` // order / subscription / refund / conversation
	ReferenceID       *string                `gorm:"type:varchar(255);index:idx_tx_reference" json:"reference_id,omitempty"`
	Description       *string                `gorm:"type:varchar(500)" json:"description,omitempty"` // Human-readable description
	TransactionDetail map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"transaction_detail,omitempty"`
	CreatedAt         time.Time              `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;index:idx_tx_created_at" json:"created_at"`
}

// TableName returns the table name
func (Transaction) TableName() string {
	return "transactions"
}

// BeforeCreate hook
func (t *Transaction) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = "TXN-" + uuid.New().String()[:8]
	}
	if t.BatchID == "" {
		t.BatchID = "BAT-" + uuid.New().String()[:8]
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	return nil
}

// GetTypeLabel returns the display label for this transaction's type
func (t *Transaction) GetTypeLabel() string {
	return TransactionTypeLabel(TransactionType(t.TransactionType))
}

// IsCashTransaction checks if this is a cash transaction
func (t *Transaction) IsCashTransaction() bool {
	return t.CurrencyType == string(CurrencyTypeCash)
}

// IsCreditsTransaction checks if this is a credits transaction
func (t *Transaction) IsCreditsTransaction() bool {
	return t.CurrencyType == string(CurrencyTypeSubscriptionCredits) ||
		t.CurrencyType == string(CurrencyTypePurchasedCredits)
}
