package connectorfinalize

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Discard permanently retires the old Connector storage while preserving the
// native Connector tables, Agent versions and custom Skill rows. It is an
// explicit maintenance operation and is intentionally not registered as a
// normal automatic migration.
func Discard(ctx context.Context, db *gorm.DB) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockInventory(tx); err != nil {
			return err
		}
		option := Options{allowDiscardInspect: true}
		r, data, err := inspect(tx, option)
		if err != nil {
			return err
		}
		if r.Checkpoint != CheckpointID {
			return fmt.Errorf("discard requires Connector checkpoint %s", CheckpointID)
		}
		if err := ensureDiscardDependencies(r); err != nil {
			return err
		}
		hosts, err := discardHostChanges(data)
		if err != nil {
			return err
		}
		catalog, err := catalogChanges(data)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := applyCatalogChanges(tx, catalog, now); err != nil {
			return err
		}
		if err := applyHostChanges(tx, hosts); err != nil {
			return err
		}
		for _, table := range RetiredTables() {
			for _, source := range data[table] {
				id := text(source, "id")
				if id == "" {
					id = text(source, "receipt_id")
				}
				if id == "" {
					id = digest(source)
				}
				if err := insertProof(tx, "retired_source_v2", digest([]string{table, id}), digest(source), "discarded"); err != nil {
					return err
				}
			}
		}
		for _, table := range r.Source {
			if err := insertProof(tx, "retired_table_v2", table.Table, table.Digest, fmt.Sprintf("rows:%d", table.Rows)); err != nil {
				return err
			}
		}
		if err := insertProof(tx, "retirement_discard_v2", MigrationID, r.Digest, "discarded"); err != nil {
			return err
		}
		if err := retirementDDL(tx); err != nil {
			return err
		}
		if err := insertProof(tx, "retirement_v2", MigrationID, r.Digest, "finalized"); err != nil {
			return err
		}
		updated := tx.Table("connector_schema_readiness").Where("id=1 AND schema_version=1").Updates(map[string]any{
			"schema_version": 2,
			"state":          "ready",
			"report_digest":  r.Digest,
			"verified_at":    now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("schema readiness changed")
		}
		if err := tx.Table("migrations").Create(map[string]any{"id": MigrationID}).Error; err != nil {
			return fmt.Errorf("record retirement migration: %w", err)
		}
		// Native verification is deliberately the final statement in the same
		// transaction. A failure rolls back the DDL and readiness marker.
		var verifyErr error
		report, verifyErr = verify(tx, Options{}, true)
		return verifyErr
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return report, err
}

func ensureDiscardDependencies(report Report) error {
	for _, issue := range report.Issues {
		if issue.Table == "retirement_schema" || issue.Code == "dependent_function" || issue.Code == "dependent_view" || issue.Code == "unknown_dependency" {
			return fmt.Errorf("discard blocked by %s on %s", issue.Code, issue.Table)
		}
	}
	return nil
}
