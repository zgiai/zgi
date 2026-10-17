package connectorfinalize

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
)

// This closed inventory permits only feature tables whose creation migration
// has never run. Native Connector tables and all other host tables stay required.
var digitalHumanHostCreationMigrations = map[string]string{
	"digital_humans":                         "202608181104207370_create_digital_human_foundation",
	"digital_human_versions":                 "202608181104207370_create_digital_human_foundation",
	"digital_human_persona_templates":        "202608181129156409_create_digital_human_persona_templates_and_bindings",
	"digital_human_resource_bindings":        "202608181129156409_create_digital_human_persona_templates_and_bindings",
	"digital_human_runs":                     "202608181152308612_create_digital_human_runs",
	"digital_human_run_outbox":               "202608181152308612_create_digital_human_runs",
	"digital_human_memory_states":            "202608190900000000_create_digital_human_memory",
	"digital_human_memories":                 "202608190900000000_create_digital_human_memory",
	"digital_human_memory_events":            "202608190900000000_create_digital_human_memory",
	"digital_human_memory_jobs":              "202608190900000000_create_digital_human_memory",
	"digital_human_memory_undo_records":      "202608190900000000_create_digital_human_memory",
	"digital_human_action_ledger":            "202608190900000000_create_digital_human_memory",
	"channel_installations":                  "202608191032439914_create_digital_human_channels",
	"external_conversation_mappings":         "202608191032439914_create_digital_human_channels",
	"channel_bindings":                       "202608191032439914_create_digital_human_channels",
	"channel_inbound_events":                 "202608191032439914_create_digital_human_channels",
	"channel_delivery_outbox":                "202608191032439914_create_digital_human_channels",
	"channel_installation_credentials":       "202608231300000000_add_channel_installation_credentials",
	"automation_task_activities":             "202608251000000000_add_digital_human_tasks",
	"digital_human_channel_task_policies":    "202608251000000000_add_digital_human_tasks",
	"digital_human_direct_skill_credentials": "202608261100000000_add_digital_human_direct_skill_credentials",
}

// VerifyBeforeDigitalHumanInstall retains every schema2 proof and native-data
// check while allowing the closed, not-yet-installed DigitalHuman inventory.
// Callers must run ordinary Verify after installing the feature, before commit.
func VerifyBeforeDigitalHumanInstall(ctx context.Context, db *gorm.DB, options Options) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hosts []string
		for _, table := range hostTables() {
			migration, optional := digitalHumanHostCreationMigrations[table]
			if optional && !tx.Migrator().HasTable(table) {
				var applied int64
				if err := tx.Table("public.migrations").Where("id = ?", migration).Count(&applied).Error; err != nil {
					return err
				}
				if applied != 0 {
					return fmt.Errorf("DigitalHuman table %s is missing despite applied creation migration", table)
				}
				continue
			}
			hosts = append(hosts, table)
		}
		var err error
		report, err = verifyWithHosts(tx, options, true, hosts)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}
