package migrationsv2

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	baselinepkg "github.com/zgiai/zgi/api/internal/migrationsv2/baseline"
)

var removedBaselineTables = []string{
	"app_extensions",
	"app_dataset_joins",
	"app_model_configs",
	"apps",
	"ai_credit_products",
	"completions",
	"dataset_retriever_resources",
	"embeddings",
	"enterprise_group_providers",
	"installed_apps",
	"llm_protocols",
	"llm_provider_protocols",
	"llm_channel_groups",
	"llm_organization_balances",
	"llm_system_credentials",
	"llm_system_channels",
	"llm_redeem_codes",
	"llm_redeem_usages",
	"llm_tenant_channels",
	"llm_tenant_credentials",
	"llm_tenant_model_settings",
	"llm_tenant_balances",
	"llm_tenant_providers",
	"llm_tenant_transactions",
	"llm_tenant_usage_logs",
	"llm_transactions",
	"live_agents_runtime_logs",
	"message_agent_thoughts",
	"model_configs",
	"load_balancing_model_configs",
	"provider_model_settings",
	"provider_models",
	"provider_settings",
	"providers",
	"sites",
	"statistics",
	"group_quotas",
	"group_subscriptions",
	"subscription_history",
	"subscription_plans",
	"system_settings",
	"settings_audit_logs",
	"tenant_default_models",
	"tenant_preferred_model_providers",
	"tool_workflow_providers",
}

func TestAllMigrationsOrder(t *testing.T) {
	want := []string{
		migrationV2InstallExtensionsID,
		migrationV2CutoverBaselineID,
		migrationV2AddLLMDefaultModelsID,
		migrationV2AddLLMUsageBillsID,
		migrationV2ScaleAICreditUnitsID,
		migrationV2MarketAPICompatibilityID,
		migrationV2AddModelConfigParamsID,
		migrationV2AddDatasetEntityFieldsID,
		migrationV2DropUnusedLegacySchemaID,
		migrationV2DropProviderModelSchemaID,
		migrationV2DropPaymentSubscriptionID,
		migrationV2DropAICreditProductsID,
		migrationV2AddSeedExecutionsID,
		migrationV2DropSystemSettingsID,
		migrationV2AddAccountsSoftDeleteID,
		migrationV2DropAgentAPIKeyQuotaID,
		migrationV2AddBootstrapLocksID,
		migrationV2DropLLMRouteForeignKeysID,
		migrationV2DropLegacyLLMControlPlaneID,
		migrationV2DropLegacyLLMWalletID,
		migrationV2AddAccountSuperAdminID,
		migrationV2AddLLMProviderConfigsID,
		migrationV2FixModelConfigPriceFieldsID,
		migrationV2WorkflowApprovalRuntimeID,
		migrationV2WorkflowPauseReasonsID,
		migrationV2AddAgentsWebAppStatusID,
		migrationV2AddCustomModelPricesID,
		migrationV2FixCustomModelRuntimeID,
		migrationV2AddRouteNativeProtocolsID,
		migrationV2BackfillModelResponsesID,
		migrationV2CreateAIChatTablesID,
		migrationV2CleanupLLMUsageLogsID,
		migrationV2AddDiagnosisContextToLogsID,
		migrationV2DataSourceExcelImportJobsID,
		migrationV2WorkflowPauseConversationID,
		migrationV2ContentParsePlatformID,
		migrationV2ContentParseSourceFilesID,
		migrationV2ContentParseShareHardeningID,
		migrationV2CreatePromptLibraryID,
		migrationV2PromptOptimizationRunsID,
		migrationV2AddAIChatConversationMetadataID,
		migrationV2AddUsageBillBillingLaneID,
		migrationV2AIChatOrganizationSkillsID,
		migrationV2WorkflowDefaultPromptsID,
		migrationV2AIChatCustomSkillsID,
		migrationV2DataLibraryFoundationID,
		migrationV2CreateWorkflowTestTablesID,
		migrationV2AddWorkflowTestJudgeModelID,
		migrationV2AddWorkflowTestVersionScopeID,
		migrationV2AddWorkflowTestExpectedResultID,
	}

	got := allMigrations()
	if len(got) != len(want) {
		t.Fatalf("expected %d migrations, got %d", len(want), len(got))
	}
	for i, migration := range got {
		if migration.ID != want[i] {
			t.Fatalf("expected migration[%d] = %s, got %s", i, want[i], migration.ID)
		}
	}
}

func TestAllMigrationsRegistersExcelImportJobsInV2(t *testing.T) {
	for _, migration := range allMigrations() {
		if migration.ID == migrationV2DataSourceExcelImportJobsID {
			return
		}
	}
	t.Fatalf("expected v2 migration %s to be registered", migrationV2DataSourceExcelImportJobsID)
}

func TestAllMigrationsRegistersContentParsePlatformInV2(t *testing.T) {
	for _, migration := range allMigrations() {
		if migration.ID == migrationV2ContentParsePlatformID {
			return
		}
	}
	t.Fatalf("expected v2 migration %s to be registered", migrationV2ContentParsePlatformID)
}

func TestAllMigrationsRegistersContentParseSourceFilesInV2(t *testing.T) {
	for _, migration := range allMigrations() {
		if migration.ID == migrationV2ContentParseSourceFilesID {
			return
		}
	}
	t.Fatalf("expected v2 migration %s to be registered", migrationV2ContentParseSourceFilesID)
}

func TestAllMigrationsRegistersDataLibraryFoundationInV2(t *testing.T) {
	for _, migration := range allMigrations() {
		if migration.ID == migrationV2DataLibraryFoundationID {
			return
		}
	}
	t.Fatalf("expected v2 migration %s to be registered", migrationV2DataLibraryFoundationID)
}

func TestChooseMigrationPlanFresh(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{})
	if err != nil {
		t.Fatalf("expected fresh plan, got error: %v", err)
	}
	if plan.name != "fresh" {
		t.Fatalf("expected fresh plan, got %s", plan.name)
	}
}

func TestChooseMigrationPlanV2Resume(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		appliedIDs: map[string]bool{
			migrationV2InstallExtensionsID: true,
		},
	})
	if err != nil {
		t.Fatalf("expected v2 plan, got error: %v", err)
	}
	if plan.name != "v2" {
		t.Fatalf("expected v2 plan, got %s", plan.name)
	}
}

func TestChooseMigrationPlanBridgeAIYoung(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		appliedIDs: map[string]bool{
			legacyAutomationMVPID: true,
			legacyAiYoungTailID:   true,
		},
	})
	if err != nil {
		t.Fatalf("expected ai-young bridge plan, got error: %v", err)
	}
	if plan.name != "bridge-ai-young" {
		t.Fatalf("expected bridge-ai-young plan, got %s", plan.name)
	}
}

func TestChooseMigrationPlanBridgeJingzhi(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		appliedIDs: map[string]bool{
			legacyMarketAPIRuntimeTablesID: true,
		},
	})
	if err != nil {
		t.Fatalf("expected jingzhi bridge plan, got error: %v", err)
	}
	if plan.name != "bridge-jingzhi-dev" {
		t.Fatalf("expected bridge-jingzhi-dev plan, got %s", plan.name)
	}
}

func TestChooseMigrationPlanBridgeLegacyTail(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		appliedIDs: map[string]bool{
			legacyAutomationMVPID:        true,
			legacyAiYoungTailID:          true,
			legacyLLMDefaultModelsID:     true,
			legacyLLMUsageBillsID:        true,
			legacyScaleAICreditUnitsID:   true,
			legacyLLMModelConfigParamsID: true,
			legacyDatasetEntityFieldsID:  true,
			legacyPaymentSubscriptionID:  true,
			legacySeedExecutionsID:       true,
		},
	})
	if err != nil {
		t.Fatalf("expected legacy-tail bridge plan, got error: %v", err)
	}
	if plan.name != "bridge-legacy-tail" {
		t.Fatalf("expected bridge-legacy-tail plan, got %s", plan.name)
	}

	want := []string{
		migrationV2InstallExtensionsID,
		migrationV2CutoverBaselineID,
		migrationV2MarketAPICompatibilityID,
		migrationV2DropUnusedLegacySchemaID,
		migrationV2DropProviderModelSchemaID,
		migrationV2DropPaymentSubscriptionID,
		migrationV2DropAICreditProductsID,
		migrationV2AddSeedExecutionsID,
		migrationV2DropSystemSettingsID,
		migrationV2AddAccountsSoftDeleteID,
		migrationV2DropAgentAPIKeyQuotaID,
		migrationV2AddBootstrapLocksID,
		migrationV2DropLLMRouteForeignKeysID,
		migrationV2DropLegacyLLMControlPlaneID,
		migrationV2DropLegacyLLMWalletID,
		migrationV2AddAccountSuperAdminID,
		migrationV2AddLLMProviderConfigsID,
		migrationV2FixModelConfigPriceFieldsID,
		migrationV2WorkflowApprovalRuntimeID,
		migrationV2WorkflowPauseReasonsID,
		migrationV2AddAgentsWebAppStatusID,
		migrationV2AddCustomModelPricesID,
		migrationV2FixCustomModelRuntimeID,
		migrationV2AddRouteNativeProtocolsID,
		migrationV2BackfillModelResponsesID,
		migrationV2CreateAIChatTablesID,
		migrationV2CleanupLLMUsageLogsID,
		migrationV2AddDiagnosisContextToLogsID,
		migrationV2DataSourceExcelImportJobsID,
		migrationV2WorkflowPauseConversationID,
		migrationV2ContentParsePlatformID,
		migrationV2ContentParseSourceFilesID,
		migrationV2ContentParseShareHardeningID,
		migrationV2CreatePromptLibraryID,
		migrationV2PromptOptimizationRunsID,
		migrationV2AddAIChatConversationMetadataID,
		migrationV2AddUsageBillBillingLaneID,
		migrationV2AIChatOrganizationSkillsID,
		migrationV2WorkflowDefaultPromptsID,
		migrationV2AIChatCustomSkillsID,
		migrationV2DataLibraryFoundationID,
		migrationV2CreateWorkflowTestTablesID,
		migrationV2AddWorkflowTestJudgeModelID,
		migrationV2AddWorkflowTestVersionScopeID,
		migrationV2AddWorkflowTestExpectedResultID,
	}
	if len(plan.migrations) != len(want) {
		t.Fatalf("expected %d legacy-tail bridge migrations, got %d", len(want), len(plan.migrations))
	}
	for i, migration := range plan.migrations {
		if migration.ID != want[i] {
			t.Fatalf("expected legacy-tail bridge migration[%d] = %s, got %s", i, want[i], migration.ID)
		}
	}
}

func TestChooseMigrationPlanRejectsUnsupportedLegacyLineage(t *testing.T) {
	_, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		appliedIDs: map[string]bool{
			legacyLLMDefaultModelsID: true,
		},
	})
	if err == nil {
		t.Fatal("expected unsupported legacy lineage error")
	}
}

func TestChooseMigrationPlanRejectsSchemaWithoutHistory(t *testing.T) {
	_, err := chooseMigrationPlan(migrationState{hasApplicationData: true})
	if err == nil {
		t.Fatal("expected error for schema without history")
	}
}

func TestBaselineChunksOrder(t *testing.T) {
	want := []string{
		"baseline/001_identity_workspace.sql",
		"baseline/002_catalog_system.sql",
		"baseline/003_billing_llm.sql",
		"baseline/004_file_dataset_graphflow.sql",
		"baseline/005_agent_workflow.sql",
		"baseline/006_automation.sql",
		"baseline/007_compatibility_views.sql",
	}

	chunks := baselinepkg.Chunks()
	if len(chunks) != len(want) {
		t.Fatalf("expected %d baseline chunks, got %d", len(want), len(chunks))
	}

	for i, chunk := range chunks {
		got := filepath.ToSlash(filepath.Join("baseline", chunk.File))
		if got != want[i] {
			t.Fatalf("expected chunk[%d] = %s, got %s", i, want[i], got)
		}
	}
}

func TestCutoverBaselineDoesNotUseAutoMigrate(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "m0001_cutover_baseline.go"))
	if err != nil {
		t.Fatalf("read baseline migration file: %v", err)
	}

	body := string(content)
	if strings.Contains(body, "AutoMigrate") {
		t.Fatal("cutover baseline must not use AutoMigrate")
	}
	if strings.Contains(body, "currentSchemaModels") {
		t.Fatal("cutover baseline must not depend on runtime models")
	}
}

func TestCutoverBaselineUsesStaticSQLFiles(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	dir := filepath.Join(filepath.Dir(filename), "baseline")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read baseline dir: %v", err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		got = append(got, entry.Name())
	}
	slices.Sort(got)

	want := []string{
		"001_identity_workspace.sql",
		"002_catalog_system.sql",
		"003_billing_llm.sql",
		"004_file_dataset_graphflow.sql",
		"005_agent_workflow.sql",
		"006_automation.sql",
		"007_compatibility_views.sql",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected baseline SQL files %v, got %v", want, got)
	}

	content, err := os.ReadFile(filepath.Join(dir, "001_identity_workspace.sql"))
	if err != nil {
		t.Fatalf("read baseline sql file: %v", err)
	}
	if !strings.Contains(string(content), "CREATE TABLE public.accounts") {
		t.Fatal("expected baseline identity SQL to contain accounts table snapshot")
	}
	if !strings.Contains(string(content), "deleted_at timestamp with time zone") {
		t.Fatal("expected baseline identity SQL to include accounts.deleted_at")
	}
	if !strings.Contains(string(content), "CREATE INDEX idx_accounts_deleted_at ON public.accounts USING btree (deleted_at);") {
		t.Fatal("expected baseline identity SQL to include idx_accounts_deleted_at")
	}
}

func TestBaselineSQLExcludesRemovedLegacyTables(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	dir := filepath.Join(filepath.Dir(filename), "baseline")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read baseline dir: %v", err)
	}

	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}

	text := combined.String()
	for _, table := range removedBaselineTables {
		patterns := []string{
			"CREATE TABLE public." + table,
			"ALTER TABLE ONLY public." + table,
			"COMMENT ON TABLE public." + table,
			"REFERENCES public." + table + "(",
			" ON public." + table + " USING ",
			" ON public." + table + "(",
		}
		for _, pattern := range patterns {
			if strings.Contains(text, pattern) {
				t.Fatalf("expected removed baseline table %s to be absent from baseline SQL; found pattern %q", table, pattern)
			}
		}
	}
	if strings.Contains(text, "system_channel_id") {
		t.Fatal("expected legacy llm_routes.system_channel_id reference to be absent from baseline SQL")
	}
	if strings.Contains(text, "channel_group_id") {
		t.Fatal("expected legacy llm_routes.channel_group_id reference to be absent from baseline SQL")
	}
	if strings.Contains(text, "app_model_config_id") {
		t.Fatal("expected legacy app_model_config_id columns to be absent from baseline SQL")
	}
	for _, relation := range []string{
		"llm_usage_logs",
		"llm_tenant_usage_logs",
		"llm_organization_usage_logs",
	} {
		if strings.Contains(text, relation) {
			t.Fatalf("expected legacy LLM usage relation %s to be absent from baseline SQL", relation)
		}
	}
}

func TestAgentWorkflowBaselineExcludesRetiredAPIKeyQuotaColumns(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	content, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "baseline", "005_agent_workflow.sql"))
	if err != nil {
		t.Fatalf("read agent workflow baseline: %v", err)
	}

	text := string(content)
	for _, column := range []string{
		"daily_quota",
		"monthly_quota",
		"daily_usage",
		"monthly_usage",
		"last_reset_date",
	} {
		if strings.Contains(text, column) {
			t.Fatalf("expected retired agent API key quota column %s to be absent from baseline SQL", column)
		}
	}
}

func TestCutoverBridgeSignatureIncludesCriticalChecks(t *testing.T) {
	signature := baselinepkg.CutoverBridgeSignature()
	if len(signature.Views) == 0 || len(signature.Indexes) == 0 || len(signature.Constraints) == 0 {
		t.Fatal("expected cutover bridge signature to validate views, indexes, and constraints")
	}

	if !slices.Contains(signature.Views, "enterprise_groups") {
		t.Fatal("expected enterprise_groups compatibility view in bridge signature")
	}
	if !slices.Contains(signature.Indexes, "idx_tool_files_lifecycle_expires_at") {
		t.Fatal("expected tool_files lifecycle index in bridge signature")
	}
	if !slices.Contains(signature.Constraints, "uk_workspace_members_workspace_account") {
		t.Fatal("expected workspace_members unique constraint in bridge signature")
	}
	if slices.Contains(signature.Constraints, "fk_llm_routes_credential") {
		t.Fatal("expected dropped llm route foreign key to be absent from bridge signature")
	}
}

func TestMigrationsV2DoesNotImportLegacyMigrations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	root := filepath.Dir(filename)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), `"github.com/zgiai/zgi/api/internal/migrations"`) {
			return fmt.Errorf("unexpected legacy migration import in %s", filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
