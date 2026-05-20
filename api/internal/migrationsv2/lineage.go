package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type migrationPlan struct {
	name       string
	migrations []*gormigrate.Migration
}

type migrationState struct {
	hasMigrationsTable bool
	hasApplicationData bool
	appliedIDs         map[string]bool
}

var lineageProbeTables = []string{
	"accounts",
	"organizations",
	"enterprise_groups",
	"workspaces",
	"tenants",
	"llm_models",
}

func resolveMigrationPlan(db *gorm.DB) (migrationPlan, error) {
	state := migrationState{
		hasMigrationsTable: db.Migrator().HasTable("migrations"),
		hasApplicationData: hasApplicationSchema(db),
	}

	if state.hasMigrationsTable {
		appliedIDs, err := appliedMigrationIDs(db)
		if err != nil {
			return migrationPlan{}, fmt.Errorf("load applied migrations: %w", err)
		}
		state.appliedIDs = appliedIDs
	}

	return chooseMigrationPlan(state)
}

func chooseMigrationPlan(state migrationState) (migrationPlan, error) {
	if !state.hasMigrationsTable || len(state.appliedIDs) == 0 {
		if state.hasApplicationData {
			return migrationPlan{}, fmt.Errorf("database contains application tables but is missing supported migration history; only fresh, ai-young tip, and jingzhi-dev tip databases can enter migrationsv2")
		}
		return migrationPlan{name: "fresh", migrations: allMigrations()}, nil
	}

	if hasAnyV2MigrationIDs(state.appliedIDs) {
		return migrationPlan{name: "v2", migrations: allMigrations()}, nil
	}

	if isLegacyTailBridgeState(state.appliedIDs) {
		return migrationPlan{name: "bridge-legacy-tail", migrations: legacyTailBridgeMigrations()}, nil
	}

	if isJingzhiBridgeState(state.appliedIDs) {
		return migrationPlan{name: "bridge-jingzhi-dev", migrations: allMigrations()}, nil
	}

	if isAiYoungBridgeState(state.appliedIDs) {
		return migrationPlan{name: "bridge-ai-young", migrations: allMigrations()}, nil
	}

	return migrationPlan{}, fmt.Errorf("unsupported legacy migration lineage; only fresh, ai-young tip, and jingzhi-dev tip databases can enter migrationsv2")
}

func appliedMigrationIDs(db *gorm.DB) (map[string]bool, error) {
	ids := make([]string, 0)
	if err := db.Table("migrations").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool, len(ids))
	for _, id := range ids {
		applied[id] = true
	}

	return applied, nil
}

func hasApplicationSchema(db *gorm.DB) bool {
	for _, table := range lineageProbeTables {
		if db.Migrator().HasTable(table) {
			return true
		}
	}

	return false
}

func hasAnyV2MigrationIDs(appliedIDs map[string]bool) bool {
	for _, id := range []string{
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
	} {
		if appliedIDs[id] {
			return true
		}
	}
	return false
}

func isAiYoungBridgeState(appliedIDs map[string]bool) bool {
	return hasAllMigrationIDs(appliedIDs,
		legacyAutomationMVPID,
		legacyAiYoungTailID,
	) &&
		!hasAnyPostCutoverLegacyIDs(appliedIDs)
}

func isLegacyTailBridgeState(appliedIDs map[string]bool) bool {
	return hasAllMigrationIDs(appliedIDs,
		legacyAutomationMVPID,
		legacyAiYoungTailID,
		legacyLLMDefaultModelsID,
		legacyLLMUsageBillsID,
		legacyScaleAICreditUnitsID,
		legacyLLMModelConfigParamsID,
		legacyDatasetEntityFieldsID,
		legacyPaymentSubscriptionID,
		legacySeedExecutionsID,
	)
}

func isJingzhiBridgeState(appliedIDs map[string]bool) bool {
	return appliedIDs[legacyMarketAPIRuntimeTablesID]
}

func hasAnyPostCutoverLegacyIDs(appliedIDs map[string]bool) bool {
	for _, id := range []string{
		legacyLLMDefaultModelsID,
		legacyLLMUsageBillsID,
		legacyScaleAICreditUnitsID,
		legacyLLMModelConfigParamsID,
		legacyDatasetEntityFieldsID,
	} {
		if appliedIDs[id] {
			return true
		}
	}
	return false
}

func hasAllMigrationIDs(appliedIDs map[string]bool, ids ...string) bool {
	for _, id := range ids {
		if !appliedIDs[id] {
			return false
		}
	}
	return true
}

func legacyTailBridgeMigrations() []*gormigrate.Migration {
	coveredLegacyEquivalentIDs := map[string]struct{}{
		migrationV2AddLLMDefaultModelsID:    {},
		migrationV2AddLLMUsageBillsID:       {},
		migrationV2ScaleAICreditUnitsID:     {},
		migrationV2AddModelConfigParamsID:   {},
		migrationV2AddDatasetEntityFieldsID: {},
	}

	migrations := allMigrations()
	filtered := make([]*gormigrate.Migration, 0, len(migrations))
	for _, migration := range migrations {
		if _, covered := coveredLegacyEquivalentIDs[migration.ID]; covered {
			continue
		}
		filtered = append(filtered, migration)
	}

	return filtered
}
