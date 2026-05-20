package migrations

import (
	"log"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
)

// allMigrations returns all migrations in order
// This is the single source of truth for all migrations
func allMigrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		M0000_install_extensions(),                                     // Install required PostgreSQL extensions
		M0001_core_base(),                                              // Core base tables
		M0002_tenant_account_relations(),                               // Tenant and account relations
		M0003_system_settings(),                                        // System settings
		M0004_provider_models(),                                        // Provider models and plugin tables
		M0005_file_management(),                                        // File and folder management
		M0006_dataset_management(),                                     // Dataset management
		M0007_app_agent_workflow(),                                     // App agent and workflow
		M0008_conversation_messages(),                                  // Conversation and messages
		M0009_data_sources(),                                           // Data_sources management
		M0010_system_utilities(),                                       // System utilities
		M0011_system_settings_i18n(),                                   // System settings i18n support
		M0012_agent_rbac_indexes(),                                     // Agent RBAC query optimization indexes
		M0013_payment_system(),                                         // Payment and subscription system
		M0014_llm_system(),                                             // LLM system
		M0015_agents_permission_refactor_indexes(),                     // Additional indexes for refactored agent permission system
		M0016_llm_extends(),                                            // LLM system extensions
		M0017_internal_flag_indexes(),                                  // Indexes for internal flag queries on agents and workflows
		M0018_llm_tenant_api_keys_add_key_hash(),                       // Add key_hash to llm_tenant_api_keys
		M0019_llm_models_add_endpoints_finetuned(),                     // Add endpoints and is_finetuned to llm_models
		M0020_llm_models_channel_update(),                              // Add is_official to llm_tenant_channels
		M0021LLMTenantAPIKeysAddIsInternal(),                           // Add is_internal to llm_tenant_api_keys
		M0022_model_add_cost_rate(),                                    // Add cost_rate to llm_models
		M0023_llm_models_remove_unique_name(),                          // Remove unique name constraint from llm_models
		M0041_cascade_delete_credential_channels(),                     // Change credential FK to CASCADE DELETE for channels
		M0027_quota_usage_tracking(),                                   // Add quota usage history tracking
		M0028_add_protocol_layer(),                                     // Add protocol layer for multi-protocol support
		M0029_add_model_governance(),                                   // Add two-layer governance for model visibility
		M0030_add_llm_model_scope(),                                    // Add scope and L1/L2/L3 architecture fields to llm_models
		M0031_add_channel_architecture(),                               // Add credential center + system channels + tenant routes
		M0028_bank_transfer(),                                          // Bank transfer recharge requests
		M0029_contacts_module(),                                        // Contacts module: departments, department_members, enterprise account status
		M0030_enterprise_invite(),                                      // Enterprise invite links and join requests
		M0032_llm_channel_v2(),                                         // LLM Channel v2: split credentials, tenant configs, custom providers/models
		M0034_payment_query_indexes(),                                  // Payment scheduler query optimization indexes
		M0035_add_agents_web_app_id(),                                  // Add web_app_id to agents and related tables
		M0036_add_system_enabled_fields(),                              // Add is_system_enabled to llm_providers and llm_models
		M0037_llm_tenant_routes(),                                      // LLM tenant routes table for gateway refactoring
		M0037_add_supported_protocols(),                                // Add supported_protocols field to llm_tenant_routes
		M0038_llm_providers_add_sort_order(),                           // Add sort_order to llm_providers
		M0039_llm_models_add_sort_order(),                              // Add sort_order to llm_models
		M0040_fix_system_channel_fk(),                                  // Fix llm_system_channels FK to reference llm_system_credentials
		M0041_add_protocol_v2_columns(),                                // Add missing columns to llm_protocols
		M0042_fix_tenant_provider_model_fields(),                       // Add missing columns to llm_tenant_providers
		M0043_create_tenant_custom_tables(),                            // Create separate tables for tenant custom providers and models
		M0044_add_provider_metadata_column(),                           // Add provider metadata column
		M0045_seed_special_models(),                                    // Seed Rerank, Vision/Multimodal, and Audio models
		M0046_seed_special_models_v2(),                                 // Seed Rerank, Vision/Multimodal, and Audio models (fixed)
		M0047_add_channel_group(),                                      // Add channel group for aggregating system channels
		M0048_create_channel_groups_table(),                            // Create independent channel groups table
		M0049FixModelTypeEmbedding(),                                   // Fix model type: embedding -> text-embedding
		M0050SeedProtocolI18n(),                                        // Seed i18n data for protocols
		M0051FixSystemChannelProtocol(),                                // Fill protocol field for system channels
		M0052NormalizeChannelGroup(),                                   // Normalize channel group schema with FK
		M0053RemoveDeprecatedFields(),                                  // Remove deprecated fields from llm_models and llm_tenant_routes
		M0054_fix_tenant_route_fk(),                                    // Fix FK constraint on user_credential_id
		M0055_add_model_tier(),                                         // Add model_tier and is_recommended fields for model categorization
		M0056_migrate_storage_gb_to_mb(),                               // Migrate storage_gb to storage_mb in quota configs
		M0057_sales_contact_requests(),                                 // Sales contact requests table
		M0058_enterprise_group_roles_permissions(),                     // Enterprise group roles and permissions system
		M0059_gdpr_compliance(),                                        // GDPR compliance: audit logs, retention policies, consents
		M0060_unify_tenant_model_tables(),                              // Migrate llm_tenant_models to llm_tenant_model_configs
		M0061_add_model_temperature_params(),                           // Add temperature min/max/default to llm_models
		M0062_add_model_capabilities(),                                 // Add supports_audio, supports_function_call, supports_json_mode, supports_streaming
		M0063_upload_files_group_team_tenant(),                         // Add group/team fields to upload_files
		M0064_add_use_cases(),                                          // Add use_cases field to llm_models and llm_tenant_custom_models
		M0065_openai_field_naming(),                                    // Rename fields to OpenAI naming conventions
		M0066_fix_transactions_schema(),                                // Fix transactions table schema for older environments
		M0067_add_model_capabilities(),                                 // Add ModelHub-aligned capability fields to llm_models
		M0068_clean_capability_naming(),                                // Rename supports_xxx to clean names, add flat capability columns
		M0069_parameter_governance(),                                   // Add metadata-driven parameter governance support
		M0070_account_contexts(),                                       // Account contexts for current organization and workspace
		M0071_file_folders_team_tenant(),                               // Add team_tenant_id to file_folders
		M0072_fix_capability_naming_cleanup(),                          // Fix issues caused by M0068: handle duplicate columns properly
		M0073_add_route_sync_mode(),                                    // Add sync_mode and last_synced_at to tenant routes for snapshot mode
		M0074_fix_use_cases(),                                          // Fix missing use_cases data for embedding and rerank models
		M0074_AddProtocolConfigFields(),                                // Add protocol configuration and health monitoring fields
		M0074_graphflow_tables(),                                       // Create graphflow tables (kb_entities, kb_entity_mentions, etc.)
		M0075_add_legacy_function_call_column(),                        // Ensure llm_models has legacy_function_call column
		M0076_upload_files_is_temporary(),                              // Add is_temporary flag to upload_files
		M0077_org_plugin_subscriptions(),                               // Organization-level plugin subscriptions
		M0078_remove_account_roles(),                                   // Remove account_roles removes the legacy account_roles table
		M0079_merge_account_extensions(),                               // Merge account_extensions into accounts.extensions
		M0080_rename_enterprise_groups_to_organizations(),              // Rename enterprise_groups to organizations and create compatibility view
		M0081_rename_tenants_to_workspaces(),                           // Rename tenants to workspaces and create compatibility view
		M0082_inline_organization_id(),                                 // Inline organization_id to workspaces
		M0083_drop_enterprise_group_tenant_joins(),                     // Drop enterprise_group_tenant_joins table
		M0084_rename_enterprise_group_account_joins_to_members(),       // Rename enterprise_group_account_joins to members
		M0085_rename_tenant_account_joins_to_workspace_members(),       // Rename tenant_account_joins to workspace_members
		M0086_merge_tenant_account_extensions_to_workspace_members(),   // Merge tenant_account_extensions to workspace_members
		M0087_rename_enterprise_group_roles_to_roles(),                 // Rename enterprise_group_roles to roles
		M0088_merge_role_permissions_to_roles(),                        // Merge enterprise_group_role_permissions into roles.permissions
		M0089_rename_account_context_fields(),                          // Rename account_contexts fields (group->organization, team->workspace)
		M0090_rename_enterprise_invite_links_and_join_requests(),       // Rename enterprise invite links and join requests to organization
		M0091_backfill_workspace_member_role_id(),                      // Backfill workspace member role id
		M0092_add_workspace_id_to_workspace_members(),                  // Add workspace_id to workspace_members
		M0093_rename_members_group_id_to_organization_id(),             // Rename members.group_id to organization_id and keep compatibility view
		M0094_installed_plugin_info(),                                  // Installed plugin info metadata table
		M0095_account_plugin_installations(),                           // Account to plugin installation relationships
		M0096_member_subscriptions(),                                   // Add member_subscriptions table
		M0097_refactor_upload_files_ids(),                              // Refactor upload_files: tenant->organization_id, team_tenant->workspace_id, drop group_id
		M0098_refactor_file_folders_ids(),                              // Refactor file_folders: tenant->organization_id, team_tenant->workspace_id
		M0099_refactor_file_folder_permissions_ids(),                   // Refactor file_folder_permissions: tenant_id->workspace_id and indexes
		M0100_refactor_datasets_ids(),                                  // Refactor datasets fields
		M0101_refactor_dataset_folders_ids(),                           // Refactor dataset_folders fields
		M0102_refactor_dataset_child_tables_ids(),                      // Refactor dataset child tables ids
		M0103_refactor_datasource_ids(),                                // Refactor datasource fields
		M0104_graphflow_soft_delete_support(),                          // Add soft delete support for graphflow tables
		M0105AddDeletedAtToDocumentSegments,                            // Add deleted_at column to document_segments table
		M0106_sync_document_segments_soft_delete,                       // Ensure document_segments has correctly typed soft delete columns
		M0107_add_dataset_extraction_strategy(),                        // Add extraction_strategy to datasets
		M0108_add_name_to_members_and_join_requests(),                  // Add name column to members and organization_join_requests
		M0108_kb_type_definitions(),                                    // Add kb_type_definitions for multi-language entity type labels
		M0109_add_graphflow_tasks_extraction_strategy(),                // Add extraction_strategy column to graphflow_tasks table
		M0110_add_llm_models_slug(),                                    // Add slug column to llm_models table
		M0111_add_workspace_id_to_apps(),                               // Add workspace_id column to apps table
		M0112_align_custom_provider_fields(),                           // Align custom provider fields with global Provider model
		M0113_align_global_provider_columns(),                          // Align global provider columns: name→provider, display_name→provider_name
		M0114_use_cases_refactor(),                                     // Migrate type→use_cases for models
		M0115_restore_custom_provider_api_base_url(),                   // Re-add api_base_url to llm_custom_providers
		M0116_align_custom_model_capabilities(),                        // Align llm_custom_models capabilities with llm_models (issue #175)
		M0117_unique_official_route_per_org(),                          // Ensure each organization has at most one active official route
		M0118_billing_ledger_and_channel_wallet(),                      // Add billing attempts/entries and private channel wallets
		M0119_reconcile_controls_and_workspace_quota(),                 // Add reconcile controls and workspace quota table for LLM billing
		M0120_fix_llm_table_compatibility(),                            // Repair llm_tenant_* schema to match llm_* expectations
		M0121_fix_llm_modelmeta_and_stats_compatibility(),              // Repair ModelMeta/stats missing table and columns
		M0122_add_llm_models_is_configured(),                           // Add llm_models.is_configured for model sync compatibility
		M0123_fix_llm_routes_compatibility(),                           // Ensure llm_routes table and organization_id exist
		M0124_fix_llm_credentials_compatibility(),                      // Ensure llm_credentials matches organization-scoped schema
		M0125_fix_llm_routes_credential_fk(),                           // Ensure route credential FK points to llm_credentials
		M0126_seed_subscription_plans_base(),                           // Backfill base subscription plans for environments missing seed data
		M0127_fix_llm_organization_api_keys_compatibility(),            // Ensure organization-scoped API key table exists for LLM system clients
		M0128_fix_channel_wallet_compatibility(),                       // Ensure private channel wallet tables exist for local billing
		M0129_fix_llm_routes_official_constraint_compatibility(),       // Repair official route constraints for schema-drifted llm_routes tables
		M0130_add_official_model_snapshot(),                            // Create single-row official model snapshot table and backfill from legacy official routes
		M0131_add_workspace_and_gateway_key_statistics_compatibility(), // Add workspace_id usage compatibility and rebuild stats view
		M0133_tool_files_lifecycle(),                                   // Add lifecycle and expiry metadata to workflow tool files
		M0134_tool_files_schema_alignment(),                            // Repair legacy tool_files schema to match current workflow file model
		M0135_account_sso_identity_fields(),                            // Add SSO identity lookup fields and uniqueness guards for accounts
		M0136_add_catalog_sync_state(),                                 // Track last applied platform catalog publication
		M0137_normalize_supported_parameters_shape(),                   // Normalize supported_parameters to ParameterDefinitions arrays
		M0138_fix_llm_custom_models_runtime_compatibility(),            // Repair llm_custom_models runtime columns after compatibility drift
		M0139_create_automation_mvp_tables(),                           // Create MVP automation task tables, indexes, and constraints
		M0139_add_llm_default_models(),                                 // Add organization-scoped default models
		M0139_add_llm_usage_bills(),                                    // Add settled usage bill fact table for LLM billing/statistics
		M0140_scale_ai_credit_units(),                                  // Scale AI credit units and stock data by 1000x
		M0141_add_llm_model_config_parameters(),                        // Add config_parameters to global and custom LLM models
		M0111_fix_transactions_tenant_id_nullable(),                    // Make transactions.tenant_id nullable, ensure group_id NOT NULL
		M0112_migrate_old_permissions(),                                // Replace group.* permissions with workspace.* in roles table
		M0113_add_dataset_segmentation_method(),                        // Add segmentation_method to datasets for chunk mode
		M0114_simplify_dataset_fields(),                                // Simplify dataset fields: rename retrieval_model to retrieval_config, remove deprecated fields
		M0131_add_image_pricing(),                                      // Add image_prices to llm_models
		M0132_seed_qwen_image_pricing(),                                // Seed wanx-v1 image pricing
		M0133_seed_doubao_image_pricing(),                              // Seed doubao-seedream image pricing
		M0141_add_dataset_entity_model_fields(),                        // Add entity_model and entity_model_provider to datasets
		M0142_remove_payment_subscription_system(),                     // Remove legacy payment subscription tables and columns
		M0143_add_seed_executions(),                                    // Add marker table for one-time bootstrap seeds
		M0144_drop_llm_route_foreign_keys(),                            // Drop legacy LLM foreign keys and rely on application-level cleanup
		M0145_add_accounts_soft_delete(),                               // Add deleted_at soft delete support for accounts
		M0146_add_bootstrap_locks(),                                    // Add bootstrap lock table for first-time setup serialization
		M0147_remove_llm_model_type(),                                  // Backfill use_cases and remove legacy model type columns
		M0148_add_agents_web_app_status(),                              // Add WebApp publish status fields to agents
		M0149_create_content_parse_provider_tables(),                   // Create independent content parse provider config and health tables
		M0150_create_content_parse_policy_tables(),                     // Create independent content parse route policy tables
		M0151_create_content_parse_run_tables(),                        // Create independent content parse run shadow tables
		M0152_create_content_parse_chunking_run_tables(),               // Create independent content parse chunking shadow tables
		M0153_create_content_parse_artifact_tables(),                   // Create independent content parse artifact registry table
		M0154_create_content_parse_playground_run_tables(),             // Create explicit-save content parse playground history tables
		M0155_add_content_parse_playground_source_files(),              // Add saved playground source-file storage metadata
		M0156_harden_content_parse_playground_sharing(),                // Require explicit sharing for content parse playground records
		M0157_create_content_parse_chunk_artifact_sets(),               // Create reusable content parse chunk artifact sets
		M0158_create_data_library_document_assets(),                    // Create Data Library document asset/version foundation
		M0159_create_data_library_reuse_events(),                       // Create Data Library reuse and lineage event foundation
		M0160_create_data_library_processing_requests(),                // Create Data Library processing request planning table
		M0161_create_data_library_vector_artifacts(),                   // Create Data Library vector artifact metadata table
		M0162_add_data_library_processing_request_execution_state(),    // Add Data Library processing request execution state columns
		M0163_create_data_library_knowledge_base_asset_refs(),          // Create Data Library Knowledge Base asset reference table
		M0164_create_data_library_database_asset_refs(),                // Create Data Library Database asset reference table
		M0165_create_data_library_extraction_artifacts(),               // Create Data Library extraction artifact metadata table
	}
}

// Run executes all database migrations
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

// RunWithDB executes all database migrations with a provided DB instance
func RunWithDB(db *gorm.DB) error {
	// Pre-migration fix: Ensure `migrations` table ID column is varchar or bigint
	// The error "value out of range for type integer" suggests it's currently INTEGER.
	// We force it to VARCHAR(255) to accommodate 'YYYYMMDDHHMMSS' style IDs.
	if db.Migrator().HasTable("migrations") {
		// PostgreSQL specific: Alter type to VARCHAR
		if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "id" TYPE varchar(255)`).Error; err != nil {
			log.Printf("Warning: Failed to alter migrations.id type: %v", err)
			// Don't return error, let gormigrate handle it or fail again
		} else {
			log.Println("Successfully altered migrations.id to varchar(255)")
		}

		// Also fix "migration" column not-null constraint if it exists.
		// Old versions might have this column as NOT NULL, but v2 inserts only ID.
		if db.Migrator().HasColumn(&gormigrate.Migration{}, "migration") { // This check might be tricky as gormigrate struct isn't a GORM model directly mapped usually
			// Just try to alter it, ignore error if column doesn't exist
			if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "migration" DROP NOT NULL`).Error; err != nil {
				log.Printf("Warning: Failed to drop NOT NULL from migrations.migration: %v", err)
			} else {
				log.Println("Successfully dropped NOT NULL from migrations.migration")
			}
		} else {
			// Try blind alter if column check is hard, usually safe enough on Postgres if column exists
			// Or better, check information_schema
			// Let's just try running it blindly, worst case it fails which is logged
			_ = db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "migration" DROP NOT NULL`)
		}

		// Also fix "batch" column not-null constraint if it exists (some other migration tools use this).
		if db.Migrator().HasColumn(&gormigrate.Migration{}, "batch") {
			if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "batch" DROP NOT NULL`).Error; err != nil {
				log.Printf("Warning: Failed to drop NOT NULL from migrations.batch: %v", err)
			} else {
				log.Println("Successfully dropped NOT NULL from migrations.batch")
			}
		} else {
			_ = db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "batch" DROP NOT NULL`)
		}
	}

	m := gormigrate.New(db, gormigrate.DefaultOptions, allMigrations())

	if err := m.Migrate(); err != nil {
		log.Printf("Migration failed: %v", err)
		return err
	}

	log.Println("Database migration completed successfully")
	return nil
}

// Rollback rolls back the last database migration
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

// RollbackWithDB rolls back the last migration with a provided DB instance
func RollbackWithDB(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, allMigrations())

	if err := m.RollbackLast(); err != nil {
		log.Printf("Rollback failed: %v", err)
		return err
	}

	log.Println("Database rollback completed successfully")
	return nil
}
