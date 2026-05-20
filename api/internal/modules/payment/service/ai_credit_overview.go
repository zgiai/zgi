package service

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrOfficialCreditUnavailable = errors.New("official ai credit unavailable")

// OfficialAICreditBalance is the official (console-managed) AI credit balance.
type OfficialAICreditBalance struct {
	Balance int64  `json:"balance"`
	Source  string `json:"source"`
}

// PrivateChannelFundDetail is the per-channel private wallet snapshot.
type PrivateChannelFundDetail struct {
	ChannelID   uuid.UUID `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	Balance     int64     `json:"balance"`
	Status      string    `json:"status"`
	Currency    string    `json:"currency"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PrivateChannelFundSummary is the aggregate private channel funds view.
type PrivateChannelFundSummary struct {
	Total    int64                      `json:"total"`
	Channels []PrivateChannelFundDetail `json:"channels"`
}

// AICreditAccountOverview extends legacy fields with official/private balances.
// Legacy credit fields are intentionally zeroed to avoid stale semantics.
type AICreditAccountOverview struct {
	ID                  string                    `json:"id"`
	AccountID           uuid.UUID                 `json:"account_id"`
	GroupID             uuid.UUID                 `json:"group_id"`
	SubscriptionCredits int64                     `json:"subscription_credits"`
	PurchasedCredits    int64                     `json:"purchased_credits"`
	TotalEarned         int64                     `json:"total_earned"`
	TotalSpent          int64                     `json:"total_spent"`
	LastResetAt         *time.Time                `json:"last_reset_at"`
	NextResetAt         *time.Time                `json:"next_reset_at"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
	OfficialAICredits   OfficialAICreditBalance   `json:"official_ai_credits"`
	PrivateChannelFunds PrivateChannelFundSummary `json:"private_channel_funds"`
}
