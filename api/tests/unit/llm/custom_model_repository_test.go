package llm_test

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	providermodel "github.com/zgiai/ginext/internal/modules/llm/provider/model"
	"gorm.io/gorm"
)

func TestCustomModelRepositoryActiveLookupRequiresActiveProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:custom_model_repo_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := createCustomModelLookupTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	orgID := uuid.New()
	deletedProvider := &providermodel.CustomProvider{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Provider:       "ollama",
		ProviderName:   "Ollama",
		APIBaseURL:     "http://127.0.0.1:18081/embed-exact #",
		IsActive:       true,
	}
	activeProvider := &providermodel.CustomProvider{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Provider:       "ollama",
		ProviderName:   "Ollama",
		APIBaseURL:     "http://localhost:11434",
		IsActive:       true,
	}
	if err := insertCustomProvider(db, deletedProvider, nil); err != nil {
		t.Fatalf("create deleted provider: %v", err)
	}
	if err := insertCustomProvider(db, activeProvider, nil); err != nil {
		t.Fatalf("create active provider: %v", err)
	}
	deletedAt := time.Now()
	if err := db.Table("llm_custom_providers").Where("id = ?", deletedProvider.ID.String()).Update("deleted_at", deletedAt).Error; err != nil {
		t.Fatalf("soft delete provider: %v", err)
	}

	staleModel := &llmmodelmodel.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     deletedProvider.ID,
		Provider:       "ollama",
		Name:           "qwen3.5:4b",
		DisplayName:    "qwen3.5:4b",
		IsActive:       true,
	}
	activeModel := &llmmodelmodel.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     activeProvider.ID,
		Provider:       "ollama",
		Name:           "qwen3.5:4b",
		DisplayName:    "qwen3.5:4b",
		IsActive:       true,
	}
	if err := insertCustomModel(db, staleModel); err != nil {
		t.Fatalf("create stale model: %v", err)
	}
	if err := insertCustomModel(db, activeModel); err != nil {
		t.Fatalf("create active model: %v", err)
	}

	repo := llmmodelrepo.NewCustomModelRepository(db)
	active := true
	models, err := repo.ListByNames(context.Background(), orgID, []string{"qwen3.5:4b"}, &active)
	if err != nil {
		t.Fatalf("ListByNames() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != activeModel.ID {
		t.Fatalf("ListByNames() = %+v, want active provider model %s", models, activeModel.ID)
	}

	listed, total, err := repo.List(context.Background(), orgID, nil, "", "", &active, 0, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(listed) != 1 || listed[0].ID != activeModel.ID {
		t.Fatalf("List() = total %d models %+v, want active provider model %s", total, listed, activeModel.ID)
	}

	model, err := repo.GetByProviderAndModel(context.Background(), orgID, "ollama", "qwen3.5:4b")
	if err != nil {
		t.Fatalf("GetByProviderAndModel() error = %v", err)
	}
	if model.ID != activeModel.ID {
		t.Fatalf("GetByProviderAndModel() = %s, want %s", model.ID, activeModel.ID)
	}
}

func createCustomModelLookupTables(db *gorm.DB) error {
	if err := db.Exec(`
CREATE TABLE llm_custom_providers (
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	provider text NOT NULL,
	provider_name text NOT NULL,
	api_base_url text,
	is_active boolean NOT NULL DEFAULT true,
	sort_order integer NOT NULL DEFAULT 0,
	created_at datetime NOT NULL,
	updated_at datetime NOT NULL,
	deleted_at datetime
)`).Error; err != nil {
		return err
	}

	return db.Exec(`
CREATE TABLE llm_custom_models (
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	provider_id text NOT NULL,
	provider text NOT NULL,
	name text NOT NULL,
	display_name text NOT NULL,
	family text,
	status text DEFAULT 'active',
	tagline text,
	description text,
	use_cases text DEFAULT '{}',
	is_flagship boolean DEFAULT false,
	is_featured boolean DEFAULT false,
	is_new boolean DEFAULT false,
	access_type text DEFAULT 'closed',
	currency text DEFAULT 'USD',
	reasoning boolean DEFAULT false,
	function_calling boolean DEFAULT false,
	structured_output boolean DEFAULT false,
	temperature boolean DEFAULT true,
	top_p boolean DEFAULT true,
	presence_penalty boolean DEFAULT false,
	frequency_penalty boolean DEFAULT false,
	logit_bias boolean DEFAULT false,
	seed boolean DEFAULT false,
	stop boolean DEFAULT true,
	max_stop_sequences integer DEFAULT 4,
	vision boolean DEFAULT false,
	audio boolean DEFAULT false,
	json_mode boolean DEFAULT false,
	streaming boolean DEFAULT true,
	chat_completions boolean DEFAULT true,
	embeddings boolean DEFAULT false,
	image_generation boolean DEFAULT false,
	speech_generation boolean DEFAULT false,
	transcription boolean DEFAULT false,
	translation boolean DEFAULT false,
	moderation boolean DEFAULT false,
	realtime boolean DEFAULT false,
	batch boolean DEFAULT false,
	fine_tuning boolean DEFAULT false,
	assistants boolean DEFAULT false,
	responses boolean DEFAULT false,
	distillation boolean DEFAULT false,
	system_prompt boolean DEFAULT true,
	logprobs boolean DEFAULT false,
	web_search boolean DEFAULT false,
	file_search boolean DEFAULT false,
	code_interpreter boolean DEFAULT false,
	computer_use boolean DEFAULT false,
	mcp boolean DEFAULT false,
	parallel_tool_calls boolean DEFAULT false,
	reasoning_effort boolean DEFAULT false,
	input_modalities text DEFAULT '[]',
	output_modalities text DEFAULT '[]',
	knowledge_cutoff text,
	context_window integer DEFAULT 0,
	max_output_tokens integer DEFAULT 0,
	max_input_tokens integer DEFAULT 0,
	supported_parameters text DEFAULT '[]',
	config_parameters text DEFAULT '[]',
	default_parameters text DEFAULT '{}',
	input_price text DEFAULT '0',
	output_price text DEFAULT '0',
	is_active boolean NOT NULL DEFAULT true,
	sort_order integer NOT NULL DEFAULT 0,
	metadata text DEFAULT '{}',
	created_at datetime NOT NULL,
	updated_at datetime NOT NULL,
	deleted_at datetime
)`).Error
}

func insertCustomProvider(db *gorm.DB, provider *providermodel.CustomProvider, deletedAt *time.Time) error {
	now := time.Now()
	return db.Table("llm_custom_providers").Create(map[string]interface{}{
		"id":              provider.ID.String(),
		"organization_id": provider.OrganizationID.String(),
		"provider":        provider.Provider,
		"provider_name":   provider.ProviderName,
		"api_base_url":    provider.APIBaseURL,
		"is_active":       provider.IsActive,
		"created_at":      now,
		"updated_at":      now,
		"deleted_at":      deletedAt,
	}).Error
}

func insertCustomModel(db *gorm.DB, model *llmmodelmodel.CustomModel) error {
	now := time.Now()
	return db.Table("llm_custom_models").Create(map[string]interface{}{
		"id":              model.ID.String(),
		"organization_id": model.OrganizationID.String(),
		"provider_id":     model.ProviderID.String(),
		"provider":        model.Provider,
		"name":            model.Name,
		"display_name":    model.DisplayName,
		"status":          "active",
		"use_cases":       "{}",
		"currency":        "USD",
		"input_price":     "0",
		"output_price":    "0",
		"metadata":        "{}",
		"is_active":       model.IsActive,
		"created_at":      now,
		"updated_at":      now,
	}).Error
}
