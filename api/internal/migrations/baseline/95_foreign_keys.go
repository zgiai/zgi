package baseline

var ForeignKeysSchema = File{
	Name: "95_foreign_keys",
	Statements: []string{
		`ALTER TABLE ONLY public.app_prompt_optimization_runs
    ADD CONSTRAINT app_prompt_optimization_runs_adopted_prompt_version_id_fkey FOREIGN KEY (adopted_prompt_version_id) REFERENCES public.app_prompt_versions(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.app_prompt_optimization_runs
    ADD CONSTRAINT app_prompt_optimization_runs_prompt_id_fkey FOREIGN KEY (prompt_id) REFERENCES public.app_prompts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.app_prompt_versions
    ADD CONSTRAINT app_prompt_versions_prompt_id_fkey FOREIGN KEY (prompt_id) REFERENCES public.app_prompts(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_task_action_id_fkey FOREIGN KEY (task_action_id) REFERENCES public.automation_task_actions(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_task_run_id_fkey FOREIGN KEY (task_run_id) REFERENCES public.automation_task_runs(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.automation_task_actions
    ADD CONSTRAINT automation_task_actions_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.automation_tasks(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.automation_task_runs
    ADD CONSTRAINT automation_task_runs_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.automation_tasks(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT billing_attempt_entries_attempt_id_fkey FOREIGN KEY (attempt_id) REFERENCES public.billing_attempts(attempt_id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.channel_wallet_transactions
    ADD CONSTRAINT channel_wallet_transactions_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channel_wallets(channel_id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.channel_wallets
    ADD CONSTRAINT channel_wallets_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.llm_routes(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.content_parse_chunk_artifact_sets
    ADD CONSTRAINT content_parse_chunk_artifact_sets_parse_artifact_id_fkey FOREIGN KEY (parse_artifact_id) REFERENCES public.content_parse_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.content_parse_chunk_artifact_sets
    ADD CONSTRAINT content_parse_chunk_artifact_sets_parse_run_id_fkey FOREIGN KEY (parse_run_id) REFERENCES public.content_parse_runs(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.content_parse_chunking_runs
    ADD CONSTRAINT content_parse_chunking_runs_parse_run_id_fkey FOREIGN KEY (parse_run_id) REFERENCES public.content_parse_runs(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.content_parse_provider_health_checks
    ADD CONSTRAINT content_parse_provider_health_checks_provider_config_id_fkey FOREIGN KEY (provider_config_id) REFERENCES public.content_parse_provider_configs(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.content_parse_route_policy_rules
    ADD CONSTRAINT content_parse_route_policy_rules_policy_id_fkey FOREIGN KEY (policy_id) REFERENCES public.content_parse_route_policies(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.content_parse_runs
    ADD CONSTRAINT content_parse_runs_route_policy_id_fkey FOREIGN KEY (route_policy_id) REFERENCES public.content_parse_route_policies(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES public.data_sources(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_parse_artifact_id_fkey FOREIGN KEY (parse_artifact_id) REFERENCES public.content_parse_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_table_id_fkey FOREIGN KEY (table_id) REFERENCES public.data_source_tables(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT data_library_database_asset_refs_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.data_library_document_versions(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_document_versions
    ADD CONSTRAINT data_library_document_versions_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_document_versions
    ADD CONSTRAINT data_library_document_versions_chunk_artifact_set_id_fkey FOREIGN KEY (chunk_artifact_set_id) REFERENCES public.content_parse_chunk_artifact_sets(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_document_versions
    ADD CONSTRAINT data_library_document_versions_parse_artifact_id_fkey FOREIGN KEY (parse_artifact_id) REFERENCES public.content_parse_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES public.data_sources(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_parse_artifact_id_fkey FOREIGN KEY (parse_artifact_id) REFERENCES public.content_parse_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_table_id_fkey FOREIGN KEY (table_id) REFERENCES public.data_source_tables(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_extraction_artifacts
    ADD CONSTRAINT data_library_extraction_artifacts_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.data_library_document_versions(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_re_chunk_artifact_set_id_fkey FOREIGN KEY (chunk_artifact_set_id) REFERENCES public.content_parse_chunk_artifact_sets(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_refs_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_refs_dataset_id_fkey FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_refs_vector_artifact_id_fkey FOREIGN KEY (vector_artifact_id) REFERENCES public.data_library_vector_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_knowledge_base_asset_refs
    ADD CONSTRAINT data_library_knowledge_base_asset_refs_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.data_library_document_versions(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_processing_requests
    ADD CONSTRAINT data_library_processing_requests_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_reuse_events
    ADD CONSTRAINT data_library_reuse_events_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_reuse_events
    ADD CONSTRAINT data_library_reuse_events_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.data_library_document_versions(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.data_library_vector_artifacts
    ADD CONSTRAINT data_library_vector_artifacts_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_vector_artifacts
    ADD CONSTRAINT data_library_vector_artifacts_chunk_artifact_set_id_fkey FOREIGN KEY (chunk_artifact_set_id) REFERENCES public.content_parse_chunk_artifact_sets(id) ON DELETE RESTRICT;`,
		`ALTER TABLE ONLY public.data_library_vector_artifacts
    ADD CONSTRAINT data_library_vector_artifacts_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.data_library_document_versions(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_source_import_job_errors
    ADD CONSTRAINT data_source_import_job_errors_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.data_source_import_jobs(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.members
    ADD CONSTRAINT enterprise_group_account_joins_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.members
    ADD CONSTRAINT enterprise_group_account_joins_group_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT fk_batch_hit_testing_task_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT fk_batch_hit_testing_task_tenant FOREIGN KEY (organization_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.data_library_database_asset_refs
    ADD CONSTRAINT fk_data_library_db_asset_refs_extraction_artifact FOREIGN KEY (extraction_artifact_id) REFERENCES public.data_library_extraction_artifacts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT fk_dataset_folder_join_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT fk_dataset_folder_join_folder FOREIGN KEY (folder_id) REFERENCES public.dataset_folders(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.dataset_folders
    ADD CONSTRAINT fk_dataset_folder_tenant FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.departments
    ADD CONSTRAINT fk_dept_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.departments
    ADD CONSTRAINT fk_dept_parent FOREIGN KEY (parent_id) REFERENCES public.departments(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT fk_dm_account FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT fk_dm_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_document FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_tenant FOREIGN KEY (organization_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_eg_roles_created_by FOREIGN KEY (created_by) REFERENCES public.accounts(id) ON DELETE RESTRICT;`,
		`ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_eg_roles_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_created_by FOREIGN KEY (created_by) REFERENCES public.accounts(id) ON DELETE RESTRICT;`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_account FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_invite_link FOREIGN KEY (invite_link_id) REFERENCES public.organization_invite_links(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_reviewer FOREIGN KEY (reviewer_id) REFERENCES public.accounts(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.kb_entities
    ADD CONSTRAINT fk_entities_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT fk_graphflow_tasks_document FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT fk_graphflow_tasks_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_custom_models
    ADD CONSTRAINT fk_llm_custom_models_organization FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_custom_providers
    ADD CONSTRAINT fk_llm_custom_providers_organization FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_entity FOREIGN KEY (entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_head FOREIGN KEY (head_entity_id) REFERENCES public.kb_entities(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_tail FOREIGN KEY (tail_entity_id) REFERENCES public.kb_entities(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_model_configs
    ADD CONSTRAINT fk_tenant_model_config_model FOREIGN KEY (model_id) REFERENCES public.llm_models(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_provider FOREIGN KEY (provider) REFERENCES public.llm_providers(provider) ON UPDATE CASCADE ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_provider_model FOREIGN KEY (provider, model) REFERENCES public.llm_models(provider, name) ON UPDATE CASCADE ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON UPDATE CASCADE ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.llm_provider_configs
    ADD CONSTRAINT fk_tenant_provider_config_provider FOREIGN KEY (provider_id) REFERENCES public.llm_providers(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_head FOREIGN KEY (head_entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_tail FOREIGN KEY (tail_entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT fk_type_definitions_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_batch_items
    ADD CONSTRAINT workflow_test_batch_items_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_batch_items
    ADD CONSTRAINT workflow_test_batch_items_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES public.workflow_test_batches(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_batches
    ADD CONSTRAINT workflow_test_batches_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_cases
    ADD CONSTRAINT workflow_test_cases_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_cases
    ADD CONSTRAINT workflow_test_cases_scenario_id_fkey FOREIGN KEY (scenario_id) REFERENCES public.workflow_test_scenarios(id) ON DELETE SET NULL;`,
		`ALTER TABLE ONLY public.workflow_test_scenarios
    ADD CONSTRAINT workflow_test_scenarios_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;`,
		`ALTER TABLE ONLY public.workflow_test_settings
    ADD CONSTRAINT workflow_test_settings_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;`,
	},
}
