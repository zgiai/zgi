package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/payment/model"
)

// MonthlyConsumptionStats represents monthly consumption statistics
type MonthlyConsumptionStats struct {
	Period struct {
		Year  int       `json:"year"`
		Month int       `json:"month"`
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"period"`
	Cash struct {
		TotalConsumed float64 `json:"total_consumed"` // Total cash spent (positive value)
		Currency      string  `json:"currency"`
	} `json:"cash"`
	Credits struct {
		SubscriptionCreditsConsumed int64 `json:"subscription_credits_consumed"` // Subscription credits consumed (positive value)
		PurchasedCreditsConsumed    int64 `json:"purchased_credits_consumed"`    // Purchased credits consumed (positive value)
		TotalCreditsConsumed        int64 `json:"total_credits_consumed"`        // Total credits consumed
	} `json:"credits"`
}

// TransactionService handles transaction query business logic
type TransactionService struct {
	db *gorm.DB
}

// NewTransactionService creates a new transaction service
func NewTransactionService(db *gorm.DB) *TransactionService {
	return &TransactionService{
		db: db,
	}
}

// GetMonthlyConsumptionStats returns monthly consumption statistics for a group
// If year and month are 0, it returns the current month's statistics
func (s *TransactionService) GetMonthlyConsumptionStats(ctx context.Context, groupID uuid.UUID, year, month int) (*MonthlyConsumptionStats, error) {
	// Determine the time range for the month
	now := time.Now()
	if year == 0 || month == 0 {
		year = now.Year()
		month = int(now.Month())
	}

	// Calculate start and end of the month
	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)

	stats := &MonthlyConsumptionStats{}
	stats.Period.Year = year
	stats.Period.Month = month
	stats.Period.Start = startOfMonth
	stats.Period.End = endOfMonth
	stats.Cash.Currency = "CNY" // Default currency

	// Query for cash consumption (negative amounts = expenses)
	var cashConsumed float64
	err := s.db.Model(&model.Transaction{}).
		Where("group_id = ?", groupID).
		Where("currency_type = ?", model.CurrencyTypeCash).
		Where("amount < 0"). // Only expenses
		Where("created_at >= ? AND created_at <= ?", startOfMonth, endOfMonth).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&cashConsumed).Error
	if err != nil {
		return nil, fmt.Errorf("failed to calculate cash consumption: %w", err)
	}
	stats.Cash.TotalConsumed = cashConsumed

	stats.Credits.SubscriptionCreditsConsumed = 0

	// Query for purchased credits consumption
	var purchasedCreditsConsumed float64
	err = s.db.Model(&model.Transaction{}).
		Where("group_id = ?", groupID).
		Where("currency_type = ?", model.CurrencyTypePurchasedCredits).
		Where("amount < 0"). // Only expenses
		Where("created_at >= ? AND created_at <= ?", startOfMonth, endOfMonth).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&purchasedCreditsConsumed).Error
	if err != nil {
		return nil, fmt.Errorf("failed to calculate purchased credits consumption: %w", err)
	}
	stats.Credits.PurchasedCreditsConsumed = int64(purchasedCreditsConsumed)
	stats.Credits.TotalCreditsConsumed = stats.Credits.PurchasedCreditsConsumed

	return stats, nil
}
