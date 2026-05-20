package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupAICreditAccount represents a group's AI credit account
// Renamed from UserAICreditAccount to match group_wallets naming convention
type GroupAICreditAccount struct {
	ID                  string     `gorm:"type:varchar(255);primaryKey" json:"id"`
	AccountID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_account_group_credit;index:idx_credit_account" json:"account_id"` // Redundant for query convenience
	GroupID             uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_account_group_credit;index:idx_credit_group" json:"group_id"`
	SubscriptionCredits int64      `gorm:"-" json:"subscription_credits"`               // Legacy field kept for response compatibility, always zero
	PurchasedCredits    int64      `gorm:"not null;default:0" json:"purchased_credits"` // Purchased credits balance (no reset)
	TotalEarned         int64      `gorm:"not null;default:0" json:"total_earned"`      // Total credits earned (cumulative)
	TotalSpent          int64      `gorm:"not null;default:0" json:"total_spent"`       // Total credits spent (cumulative)
	LastResetAt         *time.Time `gorm:"-" json:"last_reset_at"`                      // Legacy field kept for response compatibility, always nil
	NextResetAt         *time.Time `gorm:"-" json:"next_reset_at"`                      // Legacy field kept for response compatibility, always nil
	CreatedAt           time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the table name
func (GroupAICreditAccount) TableName() string {
	return "group_ai_credit_accounts"
}

// BeforeCreate hook
func (gaca *GroupAICreditAccount) BeforeCreate(tx *gorm.DB) error {
	if gaca.ID == "" {
		gaca.ID = uuid.New().String()
	}
	now := time.Now()
	if gaca.CreatedAt.IsZero() {
		gaca.CreatedAt = now
	}
	if gaca.UpdatedAt.IsZero() {
		gaca.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook
func (gaca *GroupAICreditAccount) BeforeUpdate(tx *gorm.DB) error {
	gaca.UpdatedAt = time.Now()
	return nil
}

// GetTotalCredits returns the total credit balance
func (gaca *GroupAICreditAccount) GetTotalCredits() int64 {
	return gaca.PurchasedCredits
}

// GetSubscriptionBalance returns the subscription credit balance
func (gaca *GroupAICreditAccount) GetSubscriptionBalance() int64 {
	return 0
}

// GetPurchasedBalance returns the purchased credit balance
func (gaca *GroupAICreditAccount) GetPurchasedBalance() int64 {
	return gaca.PurchasedCredits
}

// CanSpend checks if there are enough credits to spend
func (gaca *GroupAICreditAccount) CanSpend(amount int64) bool {
	return gaca.GetTotalCredits() >= amount
}

// Note: AI credit transactions are now unified into the "transactions" table.
// Use model.Transaction with CurrencyType = "subscription_credits" or "purchased_credits"
// See model/transaction.go for the unified Transaction struct.
