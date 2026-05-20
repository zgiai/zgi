package baseline

import (
	"fmt"

	"gorm.io/gorm"
)

type RequiredColumn struct {
	Table  string
	Column string
}

type Signature struct {
	Tables      []string
	Views       []string
	Columns     []RequiredColumn
	Indexes     []string
	Constraints []string
}

var cutoverBridgeSignature = Signature{
	Tables: []string{
		"accounts",
		"organizations",
		"workspaces",
		"members",
		"roles",
		"workspace_members",
		"llm_models",
		"llm_credentials",
		"llm_routes",
		"datasets",
		"workflows",
		"tool_files",
		"automation_tasks",
		"channel_wallets",
	},
	Views: []string{
		"enterprise_group_account_joins",
		"enterprise_group_roles",
		"enterprise_groups",
		"enterprise_invite_links",
		"enterprise_join_requests",
		"tenant_account_joins",
		"tenants",
	},
	Columns: []RequiredColumn{
		{Table: "accounts", Column: "extensions"},
		{Table: "accounts", Column: "mobile_e164"},
		{Table: "workspaces", Column: "organization_id"},
		{Table: "members", Column: "organization_id"},
		{Table: "workspace_members", Column: "workspace_id"},
		{Table: "workspace_members", Column: "role_id"},
		{Table: "llm_models", Column: "slug"},
		{Table: "llm_models", Column: "image_prices"},
		{Table: "llm_models", Column: "is_configured"},
		{Table: "llm_routes", Column: "organization_id"},
		{Table: "datasets", Column: "segmentation_method"},
		{Table: "datasets", Column: "retrieval_config"},
		{Table: "tool_files", Column: "lifecycle"},
		{Table: "automation_tasks", Column: "schedule_config"},
	},
	Indexes: []string{
		"idx_accounts_email_unique_nonempty",
		"idx_accounts_mobile_e164_unique",
		"idx_workspace_members_workspace_id",
		"idx_workspace_members_role_id",
		"idx_llm_routes_organization_id",
		"idx_tool_files_lifecycle_expires_at",
		"idx_automation_tasks_status_next_run_at",
		"idx_channel_wallets_org_status",
		"idx_datasets_segmentation_method",
		"uq_automation_task_runs_task_id_scheduled_for_trigger_source",
	},
	Constraints: []string{
		"uk_roles_group_name",
		"uk_workspace_members_workspace_account",
		"channel_wallets_channel_id_fkey",
		"channel_wallet_transactions_channel_id_fkey",
		"automation_task_actions_task_id_fkey",
		"automation_task_runs_task_id_fkey",
	},
}

func CutoverBridgeSignature() Signature {
	return Signature{
		Tables:      append([]string(nil), cutoverBridgeSignature.Tables...),
		Views:       append([]string(nil), cutoverBridgeSignature.Views...),
		Columns:     append([]RequiredColumn(nil), cutoverBridgeSignature.Columns...),
		Indexes:     append([]string(nil), cutoverBridgeSignature.Indexes...),
		Constraints: append([]string(nil), cutoverBridgeSignature.Constraints...),
	}
}

func ValidateBridgeSignature(tx *gorm.DB) error {
	missing := make([]string, 0)

	for _, table := range cutoverBridgeSignature.Tables {
		if !tableExists(tx, table) {
			missing = append(missing, "table:"+table)
		}
	}

	for _, view := range cutoverBridgeSignature.Views {
		exists, err := viewExists(tx, view)
		if err != nil {
			return fmt.Errorf("check bridge view %s: %w", view, err)
		}
		if !exists {
			missing = append(missing, "view:"+view)
		}
	}

	for _, column := range cutoverBridgeSignature.Columns {
		if !tx.Migrator().HasColumn(column.Table, column.Column) {
			missing = append(missing, "column:"+column.Table+"."+column.Column)
		}
	}

	for _, index := range cutoverBridgeSignature.Indexes {
		exists, err := indexExists(tx, index)
		if err != nil {
			return fmt.Errorf("check bridge index %s: %w", index, err)
		}
		if !exists {
			missing = append(missing, "index:"+index)
		}
	}

	for _, constraint := range cutoverBridgeSignature.Constraints {
		exists, err := constraintExists(tx, constraint)
		if err != nil {
			return fmt.Errorf("check bridge constraint %s: %w", constraint, err)
		}
		if !exists {
			missing = append(missing, "constraint:"+constraint)
		}
	}

	if err := describeMissingItems(missing); err != nil {
		return fmt.Errorf("cutover baseline bridge validation failed: %w", err)
	}

	return nil
}
