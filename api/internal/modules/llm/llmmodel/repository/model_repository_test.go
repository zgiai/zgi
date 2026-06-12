package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListAvailableFilteredExcludesDeprecatedModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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

func insertModelRepositoryTestModel(t *testing.T, db *gorm.DB, id, provider, name, displayName, status string) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO llm_models (id, provider, name, display_name, status, is_active, sort_order)
		VALUES (?, ?, ?, ?, ?, true, 0)
	`, id, provider, name, displayName, status).Error; err != nil {
		t.Fatalf("insert model %s: %v", name, err)
	}
}
