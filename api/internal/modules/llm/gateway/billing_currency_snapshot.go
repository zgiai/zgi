package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func captureOrganizationBillingCurrency(ctx context.Context, db *gorm.DB, bc *BillingContext) error {
	if bc == nil {
		return fmt.Errorf("billing context is nil")
	}
	if bc.USDToCNYRate.IsPositive() && validBillingDisplayCurrency(bc.DisplayCurrency) {
		return nil
	}
	if db == nil {
		return fmt.Errorf("billing database is nil")
	}
	organizationID := strings.TrimSpace(bc.OrganizationID)
	if organizationID == "" {
		return fmt.Errorf("organization_id is empty")
	}
	var row struct {
		BillingDisplayCurrency string `gorm:"column:billing_display_currency"`
		USDToCNYRate           string `gorm:"column:usd_to_cny_rate"`
	}
	if err := db.WithContext(ctx).
		Table("organizations").
		Select("billing_display_currency, usd_to_cny_rate").
		Where("id = ?", organizationID).
		Take(&row).Error; err != nil {
		return err
	}
	rate, ok := parsePositiveBillingDecimal(row.USDToCNYRate)
	if !ok {
		return fmt.Errorf("organization has invalid usd_to_cny_rate %q", row.USDToCNYRate)
	}
	currency := strings.ToUpper(strings.TrimSpace(row.BillingDisplayCurrency))
	if !validBillingDisplayCurrency(currency) {
		return fmt.Errorf("organization has invalid billing_display_currency %q", row.BillingDisplayCurrency)
	}
	bc.DisplayCurrency = currency
	bc.USDToCNYRate = rate
	return nil
}

func applyLocalBillingCurrencySnapshot(bc *BillingContext) {
	if bc == nil || !bc.USDToCNYRate.IsPositive() || !validBillingDisplayCurrency(bc.DisplayCurrency) {
		return
	}
	values := map[string]interface{}{}
	if len(bc.PricingSnapshot) > 0 && string(bc.PricingSnapshot) != "null" {
		_ = json.Unmarshal(bc.PricingSnapshot, &values)
	}
	values["billing_display_currency"] = strings.ToUpper(strings.TrimSpace(bc.DisplayCurrency))
	values["cny_per_usd"] = bc.USDToCNYRate.String()
	values["total_cost_usd"] = bc.TotalUSD.String()
	values["total_cost_cny"] = bc.TotalUSD.Mul(bc.USDToCNYRate).String()
	values["exchange_rate_source"] = "organization_call_snapshot"
	bc.PricingSnapshot = buildPricingSnapshot(values)
}

func validBillingDisplayCurrency(currency string) bool {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "USD", "CNY":
		return true
	default:
		return false
	}
}

func parseNonNegativeBillingDecimal(raw string) (decimal.Decimal, bool) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	return value, err == nil && !value.IsNegative()
}

func parsePositiveBillingDecimal(raw string) (decimal.Decimal, bool) {
	value, ok := parseNonNegativeBillingDecimal(raw)
	return value, ok && value.IsPositive()
}
