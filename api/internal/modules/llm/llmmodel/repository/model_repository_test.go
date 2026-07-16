package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListAvailableFilteredExcludesDeprecatedModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	createModelRepositoryTestTable(t, db)

	insertModelRepositoryTestModel(t, db, "11111111-1111-1111-1111-111111111111", "openai", "gpt-5-mini", "GPT 5 Mini", "active")
	insertModelRepositoryTestModel(t, db, "22222222-2222-2222-2222-222222222222", "openai", "gpt-old", "GPT Old", "deprecated")

	repo := NewModelRepository(db)
	models, err := repo.ListAvailableFiltered(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("ListAvailableFiltered() error = %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("models length = %d, want 1", len(models))
	}
	if models[0].Model != "gpt-5-mini" {
		t.Fatalf("model = %q, want gpt-5-mini", models[0].Model)
	}
	if models[0].Status != "active" {
		t.Fatalf("status = %q, want active", models[0].Status)
	}
}

func TestListWithProviderIDAndStatusQualifiesModelStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	createFullModelRepositoryTestTable(t, db)
	createModelRepositoryProviderTestTable(t, db)

	providerID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	insertModelRepositoryTestProvider(t, db, providerID.String(), "openai", "active")
	insertModelRepositoryTestModel(t, db, "11111111-1111-1111-1111-111111111111", "openai", "gpt-5-mini", "GPT 5 Mini", "active")
	insertModelRepositoryTestModel(t, db, "22222222-2222-2222-2222-222222222222", "openai", "gpt-old", "GPT Old", "deprecated")

	repo := NewModelRepository(db)
	models, total, err := repo.List(context.Background(), &providerID, "", "", "active", nil, 0, 100)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(models) != 1 {
		t.Fatalf("models length = %d, want 1", len(models))
	}
	if models[0].Model != "gpt-5-mini" {
		t.Fatalf("model = %q, want gpt-5-mini", models[0].Model)
	}
}

func createFullModelRepositoryTestTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT,
			name TEXT,
			display_name TEXT,
			family TEXT,
			family_name TEXT,
			parent_id TEXT,
			family_default BOOLEAN DEFAULT false,
			status TEXT,
			replacement_provider TEXT,
			replacement_model TEXT,
			deprecation_reason TEXT,
			tagline TEXT,
			description TEXT,
			is_flagship BOOLEAN DEFAULT false,
			is_featured BOOLEAN DEFAULT false,
			is_new BOOLEAN DEFAULT false,
			access_type TEXT,
			currency TEXT,
			use_cases TEXT DEFAULT '{}',
			reasoning BOOLEAN DEFAULT false,
			function_calling BOOLEAN DEFAULT false,
			structured_output BOOLEAN DEFAULT false,
			temperature BOOLEAN DEFAULT false,
			top_p BOOLEAN DEFAULT false,
			presence_penalty BOOLEAN DEFAULT false,
			frequency_penalty BOOLEAN DEFAULT false,
			logit_bias BOOLEAN DEFAULT false,
			seed BOOLEAN DEFAULT false,
			stop BOOLEAN DEFAULT false,
			max_stop_sequences INTEGER DEFAULT 0,
			vision BOOLEAN DEFAULT false,
			audio BOOLEAN DEFAULT false,
			json_mode BOOLEAN DEFAULT false,
			streaming BOOLEAN DEFAULT false,
			chat_completions BOOLEAN DEFAULT false,
			embeddings BOOLEAN DEFAULT false,
			image_generation BOOLEAN DEFAULT false,
			speech_generation BOOLEAN DEFAULT false,
			transcription BOOLEAN DEFAULT false,
			translation BOOLEAN DEFAULT false,
			moderation BOOLEAN DEFAULT false,
			videos BOOLEAN DEFAULT false,
			image_edit BOOLEAN DEFAULT false,
			realtime BOOLEAN DEFAULT false,
			batch BOOLEAN DEFAULT false,
			fine_tuning BOOLEAN DEFAULT false,
			assistants BOOLEAN DEFAULT false,
			responses BOOLEAN DEFAULT false,
			distillation BOOLEAN DEFAULT false,
			system_prompt BOOLEAN DEFAULT false,
			logprobs BOOLEAN DEFAULT false,
			web_search BOOLEAN DEFAULT false,
			file_search BOOLEAN DEFAULT false,
			code_interpreter BOOLEAN DEFAULT false,
			computer_use BOOLEAN DEFAULT false,
			mcp BOOLEAN DEFAULT false,
			parallel_tool_calls BOOLEAN DEFAULT false,
			reasoning_effort BOOLEAN DEFAULT false,
			input_modalities TEXT,
			output_modalities TEXT,
			knowledge_cutoff TEXT,
			open_weights BOOLEAN DEFAULT false,
			context_window INTEGER DEFAULT 0,
			max_output_tokens INTEGER DEFAULT 0,
			max_input_tokens INTEGER DEFAULT 0,
			supported_parameters TEXT,
			config_parameters TEXT,
			default_parameters TEXT,
			is_moderated BOOLEAN DEFAULT false,
			is_finetuned BOOLEAN DEFAULT false,
			cost_rate TEXT,
			input_price NUMERIC,
			output_price NUMERIC,
			input_price_configured BOOLEAN DEFAULT false,
			output_price_configured BOOLEAN DEFAULT false,
			cached_input_price NUMERIC,
			pricing TEXT,
			cost_cache_read NUMERIC,
			cost_cache_write NUMERIC,
			cost_context_over_200k TEXT,
			image_prices TEXT,
			is_active BOOLEAN DEFAULT true,
			is_configured BOOLEAN DEFAULT false,
			sort_order INTEGER DEFAULT 0,
			model_tier TEXT,
			is_recommended BOOLEAN DEFAULT false,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create full llm_models: %v", err)
	}
}

func createModelRepositoryTestTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT,
			name TEXT,
			display_name TEXT,
			status TEXT,
			use_cases TEXT DEFAULT '{}',
			reasoning BOOLEAN DEFAULT false,
			function_calling BOOLEAN DEFAULT false,
			structured_output BOOLEAN DEFAULT false,
			temperature BOOLEAN DEFAULT false,
			top_p BOOLEAN DEFAULT false,
			presence_penalty BOOLEAN DEFAULT false,
			frequency_penalty BOOLEAN DEFAULT false,
			logit_bias BOOLEAN DEFAULT false,
			seed BOOLEAN DEFAULT false,
			stop BOOLEAN DEFAULT false,
			max_stop_sequences INTEGER DEFAULT 0,
			vision BOOLEAN DEFAULT false,
			json_mode BOOLEAN DEFAULT false,
			streaming BOOLEAN DEFAULT false,
			chat_completions BOOLEAN DEFAULT false,
			embeddings BOOLEAN DEFAULT false,
			image_generation BOOLEAN DEFAULT false,
			speech_generation BOOLEAN DEFAULT false,
			transcription BOOLEAN DEFAULT false,
			moderation BOOLEAN DEFAULT false,
			realtime BOOLEAN DEFAULT false,
			batch BOOLEAN DEFAULT false,
			assistants BOOLEAN DEFAULT false,
			responses BOOLEAN DEFAULT false,
			system_prompt BOOLEAN DEFAULT false,
			logprobs BOOLEAN DEFAULT false,
			web_search BOOLEAN DEFAULT false,
			file_search BOOLEAN DEFAULT false,
			code_interpreter BOOLEAN DEFAULT false,
			computer_use BOOLEAN DEFAULT false,
			mcp BOOLEAN DEFAULT false,
			parallel_tool_calls BOOLEAN DEFAULT false,
			reasoning_effort BOOLEAN DEFAULT false,
			context_window INTEGER DEFAULT 0,
			max_output_tokens INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create llm_models: %v", err)
	}
}

func createModelRepositoryProviderTestTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE llm_providers (
			id TEXT PRIMARY KEY,
			provider TEXT,
			status TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create llm_providers: %v", err)
	}
}

func insertModelRepositoryTestProvider(t *testing.T, db *gorm.DB, id, provider, status string) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO llm_providers (id, provider, status)
		VALUES (?, ?, ?)
	`, id, provider, status).Error; err != nil {
		t.Fatalf("insert provider %s: %v", provider, err)
	}
}

func insertModelRepositoryTestModel(t *testing.T, db *gorm.DB, id, provider, name, displayName, status string) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO llm_models (id, provider, name, display_name, status, is_active, sort_order)
		VALUES (?, ?, ?, ?, ?, true, 0)
	`, id, provider, name, displayName, status).Error; err != nil {
		t.Fatalf("insert model %s: %v", name, err)
	}
}
