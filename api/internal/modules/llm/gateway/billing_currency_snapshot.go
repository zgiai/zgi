package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Exchange-rate snapshot contract (rating spec §9): every pre-deduct,
// settlement, and usage bill carries the currency segment
// {rate, rate_source, rate_at, display_currency} beside its pricing snapshot.
// rate_source identifies the lane authority. Platform rows mirror the Console
// system rate (console_system); private rows use the local organization
// resolver (pre_org). The segment is written into the existing pricing_snapshot
// JSON, never into a new column.
const (
	exchangeRateSourceConsoleSystem = "console_system"
	exchangeRateSourcePreOrg        = "pre_org"

	billingDisplayCurrencyUSD = "USD"
	billingDisplayCurrencyCNY = "CNY"

	exchangeRateSnapshotRateKey            = "rate"
	exchangeRateSnapshotRateSourceKey      = "rate_source"
	exchangeRateSnapshotRateAtKey          = "rate_at"
	exchangeRateSnapshotDisplayCurrencyKey = "display_currency"

	// Console's §7.1 settlement snapshot keys, read when mirroring a platform
	// settlement. Console always writes "exchange_rate"; only newer builders
	// additionally write "cny_per_usd".
	consoleSettlementCurrencyKey     = "currency"
	consoleSettlementExchangeRateKey = "exchange_rate"
)

// captureOrganizationBillingCurrency fills the request-scoped currency fields
// from the organization row. The organization presentment currency defaults to
// USD; it is never defaulted to CNY. A missing or non-positive usd_to_cny_rate
// is fail-closed.
func captureOrganizationBillingCurrency(ctx context.Context, db *gorm.DB, bc *BillingContext) error {
	if bc == nil {
		return fmt.Errorf("billing context is nil")
	}
	if bc.USDToCNYRate.IsPositive() {
		bc.DisplayCurrency = normalizeBillingDisplayCurrency(bc.DisplayCurrency)
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
		return fmt.Errorf("%w: organization has invalid usd_to_cny_rate %q", ErrExchangeRateUnavailable, row.USDToCNYRate)
	}
	bc.DisplayCurrency = normalizeBillingDisplayCurrency(row.BillingDisplayCurrency)
	bc.USDToCNYRate = rate
	return nil
}

// applyLocalBillingCurrencySnapshot writes the private-lane currency segment
// and the CNY totals used by the statistics projection. It fails closed when
// the organization rate is missing.
func applyLocalBillingCurrencySnapshot(bc *BillingContext) error {
	if bc == nil {
		return fmt.Errorf("billing context is nil")
	}
	if !bc.USDToCNYRate.IsPositive() {
		return fmt.Errorf("%w: organization usd_to_cny_rate is not configured", ErrExchangeRateUnavailable)
	}
	if err := mergeExchangeRateSnapshot(bc, bc.USDToCNYRate, exchangeRateSourcePreOrg, time.Now().UTC()); err != nil {
		return err
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	values["total_cost_usd"] = bc.TotalUSD.String()
	values["total_cost_cny"] = bc.TotalUSD.Mul(bc.USDToCNYRate).String()
	bc.PricingSnapshot = buildPricingSnapshot(values)
	return nil
}

// mergeExchangeRateSnapshot writes the canonical currency segment into the
// pricing snapshot. It is the only writer of rate/rate_source/rate_at/
// display_currency, so the four keys can never disagree. A missing rate or
// source is fail-closed: it never substitutes 1 or a silent USD label.
func mergeExchangeRateSnapshot(bc *BillingContext, rate decimal.Decimal, rateSource string, rateAt time.Time) error {
	if bc == nil {
		return fmt.Errorf("billing context is nil")
	}
	if !rate.IsPositive() {
		return fmt.Errorf("%w: a positive USD to CNY rate is required", ErrExchangeRateUnavailable)
	}
	source := strings.TrimSpace(rateSource)
	if source == "" {
		return fmt.Errorf("%w: an exchange rate source is required", ErrExchangeRateUnavailable)
	}
	display := normalizeBillingDisplayCurrency(bc.DisplayCurrency)
	bc.DisplayCurrency = display
	if rateAt.IsZero() {
		rateAt = time.Now().UTC()
	}
	mergePricingSnapshot(bc, map[string]interface{}{
		exchangeRateSnapshotRateKey:            rate.String(),
		exchangeRateSnapshotRateSourceKey:      source,
		exchangeRateSnapshotRateAtKey:          rateAt.UTC().Format(time.RFC3339),
		exchangeRateSnapshotDisplayCurrencyKey: display,
		// Backward-compatible keys read by the statistics projection.
		"cny_per_usd":              rate.String(),
		"billing_display_currency": display,
	})
	return nil
}

// normalizeBillingDisplayCurrency returns the supported organization
// presentment currency, defaulting to USD. It never defaults to CNY.
func normalizeBillingDisplayCurrency(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case billingDisplayCurrencyCNY:
		return billingDisplayCurrencyCNY
	case billingDisplayCurrencyUSD:
		return billingDisplayCurrencyUSD
	default:
		return billingDisplayCurrencyUSD
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
