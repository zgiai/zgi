package gateway

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCaptureOrganizationBillingCurrencyUsesRequestTimeSettings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE organizations (
		id TEXT PRIMARY KEY,
		billing_display_currency TEXT NOT NULL,
		usd_to_cny_rate NUMERIC NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO organizations (id, billing_display_currency, usd_to_cny_rate) VALUES (?, ?, ?)`, "org-1", "CNY", "7.234567").Error; err != nil {
		t.Fatal(err)
	}

	bc := &BillingContext{OrganizationID: "org-1"}
	if err := captureOrganizationBillingCurrency(context.Background(), db, bc); err != nil {
		t.Fatalf("captureOrganizationBillingCurrency() error = %v", err)
	}
	if bc.DisplayCurrency != "CNY" || !bc.USDToCNYRate.Equal(decimal.RequireFromString("7.234567")) {
		t.Fatalf("captured currency = %s/%s", bc.DisplayCurrency, bc.USDToCNYRate)
	}

	if err := db.Exec(`UPDATE organizations SET billing_display_currency = ?, usd_to_cny_rate = ? WHERE id = ?`, "USD", "8.1", "org-1").Error; err != nil {
		t.Fatal(err)
	}
	if err := captureOrganizationBillingCurrency(context.Background(), db, bc); err != nil {
		t.Fatalf("second captureOrganizationBillingCurrency() error = %v", err)
	}
	if bc.DisplayCurrency != "CNY" || !bc.USDToCNYRate.Equal(decimal.RequireFromString("7.234567")) {
		t.Fatalf("call-time snapshot changed after organization update: %s/%s", bc.DisplayCurrency, bc.USDToCNYRate)
	}
}
