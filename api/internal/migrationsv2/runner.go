package migrationsv2

import (
	"log"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
)

func allMigrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		M0000_install_extensions(),
		M0001_cutover_baseline(),
		M0002_add_llm_default_models(),
		M0003_add_llm_usage_bills(),
		M0004_scale_ai_credit_units(),
		M0005_market_api_compatibility(),
		M0006_add_llm_model_config_parameters(),
		M0007_add_dataset_entity_fields(),
		M0008_drop_unused_legacy_schema(),
		M0009_drop_provider_model_schema(),
		M0010_drop_payment_subscription_schema(),
		M0011_drop_ai_credit_products_schema(),
		M0012_add_seed_executions(),
		M0013_drop_system_settings_schema(),
		M0014_add_accounts_soft_delete(),
		M0015_drop_agent_api_key_daily_monthly_quota(),
		M0016_add_bootstrap_locks(),
		M0017_drop_llm_route_foreign_keys(),
		M0018_drop_legacy_llm_control_plane(),
		M0019_drop_legacy_llm_wallet_schema(),
		M0020_add_account_super_admin(),
		M0021_add_llm_provider_configs(),
		M0022_fix_llm_model_config_price_fields(),
		M0023_add_workflow_approval_runtime(),
		M0024_add_workflow_pause_reasons(),
		M0025_add_agents_web_app_status(),
		M0026_add_custom_model_price_columns(),
		M0027_fix_custom_model_runtime_schema(),
		M0028_add_route_native_protocols(),
		M0029_backfill_llm_model_responses_from_provider_spec(),
		M0030_create_aichat_tables(),
		M0031_cleanup_llm_usage_logs(),
		M0028_add_diagnosis_context_to_logs(),
		M0030_data_source_excel_import_jobs(),
		M0031_add_workflow_pause_conversation_id(),
		M0031_create_content_parse_platform_tables(),
		M0032_add_content_parse_playground_source_files(),
		M0033_harden_content_parse_playground_sharing(),
		M0032_create_prompt_library(),
		M0033_create_prompt_optimization_runs(),
		M0034_add_aichat_conversation_metadata(),
		M0034_add_usage_bill_billing_lane(),
		M0035_create_aichat_organization_skill_configs(),
		M0034_seed_workflow_default_prompts(),
		M0036_create_aichat_custom_skills(),
		M0037_create_data_library_foundation_tables(),
		M0031_create_workflow_test_tables(),
		M0032_add_workflow_test_judge_model(),
		M0033_add_workflow_test_version_scope(),
		M0034_add_workflow_test_expected_result(),
	}
}

func migrationOptions() *gormigrate.Options {
	options := *gormigrate.DefaultOptions
	options.ValidateUnknownMigrations = false
	return &options
}

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		return err
	}

	return RunWithDB(db)
}

func RunWithDB(db *gorm.DB) error {
	normalizeMigrationTableColumns(db)

	plan, err := resolveMigrationPlan(db)
	if err != nil {
		log.Printf("Failed to resolve migrationsv2 plan: %v", err)
		return err
	}

	log.Printf("Using migrationsv2 plan %s with %d migrations", plan.name, len(plan.migrations))

	m := gormigrate.New(db, migrationOptions(), plan.migrations)
	if err := m.Migrate(); err != nil {
		log.Printf("migrationsv2 failed: %v", err)
		return err
	}

	log.Println("migrationsv2 completed successfully")
	return nil
}

func Rollback() error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		return err
	}

	return RollbackWithDB(db)
}

func RollbackWithDB(db *gorm.DB) error {
	normalizeMigrationTableColumns(db)

	plan, err := resolveMigrationPlan(db)
	if err != nil {
		log.Printf("Failed to resolve migrationsv2 rollback plan: %v", err)
		return err
	}

	log.Printf("Using migrationsv2 rollback plan %s with %d migrations", plan.name, len(plan.migrations))

	m := gormigrate.New(db, migrationOptions(), plan.migrations)
	if err := m.RollbackLast(); err != nil {
		log.Printf("migrationsv2 rollback failed: %v", err)
		return err
	}

	log.Println("migrationsv2 rollback completed successfully")
	return nil
}

func normalizeMigrationTableColumns(db *gorm.DB) {
	if !db.Migrator().HasTable("migrations") {
		return
	}

	if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "id" TYPE varchar(255)`).Error; err != nil {
		log.Printf("Warning: Failed to alter migrations.id type: %v", err)
	}

	if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "migration" DROP NOT NULL`).Error; err != nil {
		log.Printf("Warning: Failed to drop NOT NULL from migrations.migration: %v", err)
	}

	if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "batch" DROP NOT NULL`).Error; err != nil {
		log.Printf("Warning: Failed to drop NOT NULL from migrations.batch: %v", err)
	}
}
