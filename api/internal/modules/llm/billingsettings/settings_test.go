package billingsettings

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStoreCatalogRatePersistsAndUpdates(t *testing.T) {
	db := openSettingsTestDB(t)
	store := Init(context.Background(), db)

	if got := store.PointsPerUSD(); got != DefaultPointsPerUSD {
		t.Fatalf("empty table should yield default %d, got %d", DefaultPointsPerUSD, got)
	}
	if store.HasValue() {
		t.Fatal("empty table must not report a loaded value")
	}

	if err := store.StoreCatalogRate(context.Background(), 1000); err != nil {
		t.Fatal(err)
	}
	if got := store.PointsPerUSD(); got != 1000 {
		t.Fatalf("cached rate = %d, want 1000", got)
	}

	var setting Setting
	if err := db.Where("key = ?", KeyPointsPerUSD).First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != "1000" {
		t.Fatalf("persisted value = %q, want 1000", setting.Value)
	}

	if err := store.StoreCatalogRate(context.Background(), 2000); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", KeyPointsPerUSD).First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != "2000" {
		t.Fatalf("persisted value after update = %q, want 2000", setting.Value)
	}

	var count int64
	if err := db.Table("llm_billing_settings").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings row count = %d, want 1", count)
	}
}

func TestStoreCatalogRateIgnoresNonPositive(t *testing.T) {
	db := openSettingsTestDB(t)
	store := Init(context.Background(), db)

	if err := store.StoreCatalogRate(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreCatalogRate(context.Background(), -3); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Table("llm_billing_settings").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("non-positive rates must not be persisted, row count = %d", count)
	}
	if got := store.PointsPerUSD(); got != DefaultPointsPerUSD {
		t.Fatalf("rate = %d, want default %d", got, DefaultPointsPerUSD)
	}
}

func TestInitLoadsExistingValue(t *testing.T) {
	db := openSettingsTestDB(t)
	if err := db.Exec(
		"INSERT INTO llm_billing_settings (key, value, updated_at) VALUES (?, ?, ?)",
		KeyPointsPerUSD, "1500", time.Now(),
	).Error; err != nil {
		t.Fatal(err)
	}

	store := Init(context.Background(), db)
	if got := store.PointsPerUSD(); got != 1500 {
		t.Fatalf("loaded rate = %d, want 1500", got)
	}
	if !store.HasValue() {
		t.Fatal("loaded store must report a value")
	}
}

func TestInitFallsBackOnInvalidValue(t *testing.T) {
	db := openSettingsTestDB(t)
	if err := db.Exec(
		"INSERT INTO llm_billing_settings (key, value, updated_at) VALUES (?, ?, ?)",
		KeyPointsPerUSD, "not-a-number", time.Now(),
	).Error; err != nil {
		t.Fatal(err)
	}

	store := Init(context.Background(), db)
	if got := store.PointsPerUSD(); got != DefaultPointsPerUSD {
		t.Fatalf("invalid value should fall back to %d, got %d", DefaultPointsPerUSD, got)
	}
}

func openSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE llm_billing_settings (key text PRIMARY KEY, value text NOT NULL, updated_at datetime NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}
