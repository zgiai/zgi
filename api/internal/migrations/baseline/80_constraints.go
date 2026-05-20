package baseline

var ConstraintsSchema = File{
	Name: "80_constraints",
	Statements: []string{
		`ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.enterprise_group_plugin_subscriptions_id_seq'::regclass);`,
		`ALTER TABLE ONLY public.account_contexts
    ADD CONSTRAINT account_contexts_pkey PRIMARY KEY (account_id);`,
		`ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT account_integrate_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.accounts
    ADD CONSTRAINT account_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.account_plugin_installations
    ADD CONSTRAINT account_plugin_installations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agent_api_key_usage_logs
    ADD CONSTRAINT agent_api_key_usage_logs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agent_api_keys
    ADD CONSTRAINT agent_api_keys_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agent_extensions
    ADD CONSTRAINT agent_extensions_agent_id_key UNIQUE (agent_id);`,
		`ALTER TABLE ONLY public.agent_extensions
    ADD CONSTRAINT agent_extensions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agents_configs
    ADD CONSTRAINT agents_configs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agents_conversations
    ADD CONSTRAINT agents_conversations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agents_messages
    ADD CONSTRAINT agents_messages_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.aichat_conversations
    ADD CONSTRAINT aichat_conversations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.aichat_custom_skills
    ADD CONSTRAINT aichat_custom_skills_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.aichat_messages
    ADD CONSTRAINT aichat_messages_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.aichat_organization_skill_configs
    ADD CONSTRAINT aichat_organization_skill_configs_pkey PRIMARY KEY (organization_id, skill_id);`,
		`ALTER TABLE ONLY public.app_prompt_optimization_runs
    ADD CONSTRAINT app_prompt_optimization_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.app_prompt_versions
    ADD CONSTRAINT app_prompt_versions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.app_prompt_versions
    ADD CONSTRAINT app_prompt_versions_prompt_id_version_key UNIQUE (prompt_id, version);`,
		`ALTER TABLE ONLY public.app_prompts
    ADD CONSTRAINT app_prompts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.automation_task_actions
    ADD CONSTRAINT automation_task_actions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.automation_task_runs
    ADD CONSTRAINT automation_task_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.automation_tasks
    ADD CONSTRAINT automation_tasks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.bank_transfer_requests
    ADD CONSTRAINT bank_transfer_requests_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT batch_hit_testing_tasks_pkey PRIMARY KEY (task_id);`,
		`ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT billing_attempt_entries_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.billing_attempts
    ADD CONSTRAINT billing_attempts_pkey PRIMARY KEY (attempt_id);`,
		`ALTER TABLE ONLY public.channel_wallet_transactions
    ADD CONSTRAINT channel_wallet_transactions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.channel_wallets
    ADD CONSTRAINT channel_wallets_pkey PRIMARY KEY (channel_id);`,
		`ALTER TABLE ONLY public.child_chunks
    ADD CONSTRAINT child_chunks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_artifacts
    ADD CONSTRAINT content_parse_artifacts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_chunk_artifact_sets
    ADD CONSTRAINT content_parse_chunk_artifact_sets_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_chunking_runs
    ADD CONSTRAINT content_parse_chunking_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_playground_runs
    ADD CONSTRAINT content_parse_playground_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_provider_configs
    ADD CONSTRAINT content_parse_provider_configs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_provider_health_checks
    ADD CONSTRAINT content_parse_provider_health_checks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_route_policies
    ADD CONSTRAINT content_parse_route_policies_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_route_policy_rules
    ADD CONSTRAINT content_parse_route_policy_rules_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.content_parse_runs
    ADD CONSTRAINT content_parse_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.conversation_group
    ADD CONSTRAINT conversation_group_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_document_assets
    ADD CONSTRAINT data_library_document_assets_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_document_versions
    ADD CONSTRAINT data_library_document_versions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_refs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_processing_requests
    ADD CONSTRAINT data_library_processing_requests_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_reuse_events
    ADD CONSTRAINT data_library_reuse_events_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_library_vector_artifacts
    ADD CONSTRAINT data_library_vector_artifacts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_retention_policies
    ADD CONSTRAINT data_retention_policies_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_source_import_job_errors
    ADD CONSTRAINT data_source_import_job_errors_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_source_import_jobs
    ADD CONSTRAINT data_source_import_jobs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_source_sql_operations
    ADD CONSTRAINT data_source_sql_operations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_source_table_prompts
    ADD CONSTRAINT data_source_table_prompts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_source_tables
    ADD CONSTRAINT data_source_tables_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_collection_bindings
    ADD CONSTRAINT dataset_collection_bindings_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT dataset_folder_joins_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_folders
    ADD CONSTRAINT dataset_folders_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_metadata_bindings
    ADD CONSTRAINT dataset_metadata_bindings_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_metadatas
    ADD CONSTRAINT dataset_metadatas_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_permissions
    ADD CONSTRAINT dataset_permissions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_process_rules
    ADD CONSTRAINT dataset_process_rules_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.dataset_queries
    ADD CONSTRAINT dataset_queries_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.datasets
    ADD CONSTRAINT datasets_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT department_members_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT document_segment_questions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.document_segments
    ADD CONSTRAINT document_segments_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.end_users
    ADD CONSTRAINT end_user_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT enterprise_group_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions
    ADD CONSTRAINT enterprise_group_plugin_subscriptions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT enterprise_invite_links_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT enterprise_join_requests_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.file_favorites
    ADD CONSTRAINT file_favorites_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.file_folder_joins
    ADD CONSTRAINT file_folder_joins_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.file_folder_permissions
    ADD CONSTRAINT file_folder_permissions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.file_folders
    ADD CONSTRAINT file_folders_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.gdpr_audit_logs
    ADD CONSTRAINT gdpr_audit_logs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT graphflow_tasks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.group_ai_credit_accounts
    ADD CONSTRAINT group_ai_credit_accounts_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.group_wallets
    ADD CONSTRAINT group_wallets_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions
    ADD CONSTRAINT idx_group_account_installation_unique UNIQUE (group_id, account_id, installation_id);`,
		`ALTER TABLE ONLY public.installed_agents
    ADD CONSTRAINT installed_agents_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.installed_plugin_info
    ADD CONSTRAINT installed_plugin_info_marketplace_version_id_key UNIQUE (marketplace_version_id);`,
		`ALTER TABLE ONLY public.installed_plugin_info
    ADD CONSTRAINT installed_plugin_info_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.kb_entities
    ADD CONSTRAINT kb_entities_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT kb_entity_mentions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT kb_relationships_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT kb_triple_mentions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT kb_type_definitions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_catalog_sync_states
    ADD CONSTRAINT llm_catalog_sync_states_pkey PRIMARY KEY (sync_key);`,
		`ALTER TABLE ONLY public.llm_credentials
    ADD CONSTRAINT llm_credentials_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_default_models
    ADD CONSTRAINT llm_default_models_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_models
    ADD CONSTRAINT llm_models_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_official_model_snapshots
    ADD CONSTRAINT llm_official_model_snapshots_pkey PRIMARY KEY (source_key);`,
		`ALTER TABLE ONLY public.llm_provider_configs
    ADD CONSTRAINT llm_provider_configs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_providers
    ADD CONSTRAINT llm_providers_name_key UNIQUE (provider);`,
		`ALTER TABLE ONLY public.llm_providers
    ADD CONSTRAINT llm_providers_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_organization_api_keys
    ADD CONSTRAINT llm_tenant_api_keys_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_custom_models
    ADD CONSTRAINT llm_tenant_custom_models_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_custom_providers
    ADD CONSTRAINT llm_tenant_custom_providers_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_model_configs
    ADD CONSTRAINT llm_tenant_model_configs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT llm_tenant_models_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_routes
    ADD CONSTRAINT llm_tenant_routes_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_usage_bills
    ADD CONSTRAINT llm_usage_bills_attempt_id_key UNIQUE (attempt_id);`,
		`ALTER TABLE ONLY public.llm_usage_bills
    ADD CONSTRAINT llm_usage_bills_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.llm_workspace_quotas
    ADD CONSTRAINT llm_workspace_quotas_pkey PRIMARY KEY (workspace_id);`,
		`ALTER TABLE ONLY public.members
    ADD CONSTRAINT members_pkey PRIMARY KEY (organization_id, account_id);`,
		`ALTER TABLE ONLY public.message_annotations
    ADD CONSTRAINT message_annotations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.message_feedbacks
    ADD CONSTRAINT message_feedbacks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.message_files
    ADD CONSTRAINT message_files_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.payment_callbacks
    ADD CONSTRAINT payment_callbacks_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.payment_transactions
    ADD CONSTRAINT payment_transactions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.plugin_installations
    ADD CONSTRAINT plugin_installations_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.quota_usage_history
    ADD CONSTRAINT quota_usage_history_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.refund_records
    ADD CONSTRAINT refund_records_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.sales_contact_requests
    ADD CONSTRAINT sales_contact_requests_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.seed_executions
    ADD CONSTRAINT seed_executions_pkey PRIMARY KEY (name, version);`,
		`ALTER TABLE ONLY public.workspaces
    ADD CONSTRAINT tenant_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.tool_files
    ADD CONSTRAINT tool_files_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT uk_dept_member UNIQUE (department_id, account_id);`,
		`ALTER TABLE ONLY public.departments
    ADD CONSTRAINT uk_dept_name_parent UNIQUE (group_id, parent_id, name);`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT uk_enterprise_invite_links_token UNIQUE (token);`,
		`ALTER TABLE ONLY public.roles
    ADD CONSTRAINT uk_roles_group_name UNIQUE (group_id, name);`,
		`ALTER TABLE ONLY public.workspace_members
    ADD CONSTRAINT uk_workspace_members_workspace_account UNIQUE (workspace_id, account_id);`,
		`ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT unique_account_provider UNIQUE (account_id, provider);`,
		`ALTER TABLE ONLY public.dataset_collection_bindings
    ADD CONSTRAINT unique_collection_name UNIQUE (collection_name);`,
		`ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT unique_provider_open_id UNIQUE (provider, open_id);`,
		`ALTER TABLE ONLY public.upload_files
    ADD CONSTRAINT upload_files_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.account_plugin_installations
    ADD CONSTRAINT uq_account_plugin_installations_tenant_version UNIQUE (tenant_id, marketplace_version_id);`,
		`ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT uq_billing_attempt_entry UNIQUE (attempt_id, entry_type, ledger_type);`,
		`ALTER TABLE ONLY public.data_retention_policies
    ADD CONSTRAINT uq_data_retention_policies_data_type UNIQUE (data_type);`,
		`ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT uq_tenant_provider_model UNIQUE (tenant_id, provider, model);`,
		`ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT uq_type_per_dataset UNIQUE (dataset_id, type_key);`,
		`ALTER TABLE ONLY public.workflow_test_settings
    ADD CONSTRAINT uq_workflow_test_settings_agent UNIQUE (agent_id);`,
		`ALTER TABLE ONLY public.user_consents
    ADD CONSTRAINT user_consents_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_app_logs
    ADD CONSTRAINT workflow_app_logs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_approval_deliveries
    ADD CONSTRAINT workflow_approval_deliveries_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_approval_forms
    ADD CONSTRAINT workflow_approval_forms_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_approval_recipients
    ADD CONSTRAINT workflow_approval_recipients_access_token_key UNIQUE (access_token);`,
		`ALTER TABLE ONLY public.workflow_approval_recipients
    ADD CONSTRAINT workflow_approval_recipients_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_conversation_variables
    ADD CONSTRAINT workflow_conversation_variables_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_node_executions
    ADD CONSTRAINT workflow_node_executions_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_node_runtime_logs
    ADD CONSTRAINT workflow_node_runtime_logs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_run_events
    ADD CONSTRAINT workflow_run_events_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_run_logs
    ADD CONSTRAINT workflow_run_logs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_run_pause_reasons
    ADD CONSTRAINT workflow_run_pause_reasons_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_run_pauses
    ADD CONSTRAINT workflow_run_pauses_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_runs
    ADD CONSTRAINT workflow_runs_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_test_batch_items
    ADD CONSTRAINT workflow_test_batch_items_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_test_batches
    ADD CONSTRAINT workflow_test_batches_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_test_cases
    ADD CONSTRAINT workflow_test_cases_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_test_scenarios
    ADD CONSTRAINT workflow_test_scenarios_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflow_test_settings
    ADD CONSTRAINT workflow_test_settings_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workflows
    ADD CONSTRAINT workflows_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.workspace_members
    ADD CONSTRAINT workspace_members_pkey PRIMARY KEY (id);`,
		`ALTER TABLE ONLY public.zgi_bootstrap_locks
    ADD CONSTRAINT zgi_bootstrap_locks_pkey PRIMARY KEY (key);`,
		`ALTER TABLE ONLY public.zgi_setups
    ADD CONSTRAINT zgi_setups_pkey PRIMARY KEY (version);`,
	},
}
