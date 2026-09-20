package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openPricingIssuesDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE llm_models (
			id text PRIMARY KEY,
			provider text NOT NULL,
			name text NOT NULL,
			display_name text NOT NULL,
			use_cases text NOT NULL DEFAULT '[]',
			pricing text,
			input_price real,
			output_price real,
			cached_input_price real,
			cache_read_price real,
			cache_write_price real,
			cache_write_5m_price real,
			cache_write_1h_price real,
			is_active boolean NOT NULL DEFAULT 1,
			deleted_at datetime
		)`,
		`CREATE TABLE llm_custom_models (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			provider text NOT NULL,
			name text NOT NULL,
			display_name text NOT NULL,
			use_cases text NOT NULL DEFAULT '[]',
			pricing text,
			input_price real,
			output_price real,
			cost_cache_read real,
			cost_cache_write real,
			cost_cache_write_5m real,
			cost_cache_write_1h real,
			is_active boolean NOT NULL DEFAULT 1,
			deleted_at datetime
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestListPricingIssues_IncludesOrgCustomModels(t *testing.T) {
	db := openPricingIssuesDB(t)
	orgID := uuid.New()
	otherOrgID := uuid.New()

	insert := func(table string, values map[string]interface{}) {
		t.Helper()
		if err := db.Table(table).Create(values).Error; err != nil {
			t.Fatalf("insert %s: %v", table, err)
		}
	}

	insert("llm_models", map[string]interface{}{
		"id": uuid.New(), "provider": "openai", "name": "unpriced-global",
		"display_name": "Unpriced Global", "use_cases": `["text-chat"]`, "pricing": ``, "is_active": true,
	})
	insert("llm_models", map[string]interface{}{
		"id": uuid.New(), "provider": "openai", "name": "priced-global",
		"display_name": "Priced Global", "use_cases": `["text-chat"]`, "pricing": ``,
		"input_price": 1.0, "output_price": 2.0, "is_active": true,
	})
	insert("llm_custom_models", map[string]interface{}{
		"id": uuid.New(), "organization_id": orgID, "provider": "1", "name": "unpriced-custom",
		"display_name": "Unpriced Custom", "use_cases": `["text-chat"]`, "pricing": ``, "is_active": true,
	})
	insert("llm_custom_models", map[string]interface{}{
		"id": uuid.New(), "organization_id": orgID, "provider": "2", "name": "priced-music",
		"display_name": "Priced Music", "use_cases": `["music-gen"]`,
		"pricing":   `{"metered":[{"operation":"music_generation","meter":"output_track","base_unit":"track","price":{"amount":"1","currency":"USD","per_quantity":1}}]}`,
		"is_active": true,
	})
	insert("llm_custom_models", map[string]interface{}{
		"id": uuid.New(), "organization_id": otherOrgID, "provider": "3", "name": "other-org",
		"display_name": "Other Org", "use_cases": `["text-chat"]`, "pricing": ``, "is_active": true,
	})

	svc := &modelService{db: db}
	result, err := svc.ListPricingIssues(context.Background(), orgID)
	if err != nil {
		t.Fatalf("ListPricingIssues error = %v", err)
	}

	byModel := make(map[string]bool, len(result.Items))
	for _, item := range result.Items {
		byModel[item.Model] = item.IsCustom
	}
	if _, ok := byModel["unpriced-global"]; !ok {
		t.Fatal("global model missing pricing must be reported")
	}
	if _, ok := byModel["priced-global"]; ok {
		t.Fatal("fully priced global model must not be reported")
	}
	isCustom, ok := byModel["unpriced-custom"]
	if !ok {
		t.Fatal("org custom model missing pricing must be reported")
	}
	if !isCustom {
		t.Fatal("custom pricing issue must be marked is_custom")
	}
	if _, ok := byModel["priced-music"]; ok {
		t.Fatal("custom model with metered pricing must not be reported")
	}
	if _, ok := byModel["other-org"]; ok {
		t.Fatal("another organization's custom model must not leak into the report")
	}
}
