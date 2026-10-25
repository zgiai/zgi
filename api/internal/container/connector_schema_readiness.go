package container

import (
	"context"
	"fmt"
	"log"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
)

// EnsureNativeConnectorSchema is checked before any Connector route, worker or
// runtime is exposed. Only the offline converter may attest a populated upgrade.
func (c *ServiceContainer) EnsureNativeConnectorSchema(ctx context.Context) error {
	if c == nil || c.db == nil {
		return fmt.Errorf("Connector database is unavailable")
	}
	var row struct {
		SchemaVersion int
		State         string
	}
	if err := c.db.WithContext(ctx).Table("connector_schema_readiness").Where("id = 1").Take(&row).Error; err != nil {
		return fmt.Errorf("Connector native schema migration is required")
	}
	if row.SchemaVersion == 1 && (row.State == "ready" || row.State == "conversion_required") {
		if config.ConnectorRetirementDeferred() {
			return nil
		}
		// In auto mode the migration run deferred retirement because legacy
		// storage holds data; the ledger records that, so every process follows
		// it without its own configuration.
		if config.ConnectorRetirementAdaptive() {
			deferred, err := connectorfinalize.DeferredInDatabase(ctx, c.db)
			if err != nil {
				return fmt.Errorf("Connector retirement state is unavailable: %w", err)
			}
			if deferred {
				config.MarkConnectorRetirementDeferred()
				log.Printf("Connector retirement is deferred: legacy Connector storage awaits connector-finalize (plan, archive, apply and verify, or discard)")
				return nil
			}
		}
	}
	if row.SchemaVersion != 2 || row.State != "ready" {
		return fmt.Errorf("Connector schema2 finalization and verification are required before startup")
	}
	return nil
}
