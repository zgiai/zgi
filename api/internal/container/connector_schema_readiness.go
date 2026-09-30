package container

import (
	"context"
	"fmt"
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
	if row.SchemaVersion != 2 || row.State != "ready" {
		return fmt.Errorf("Connector schema2 finalization and verification are required before startup")
	}
	return nil
}
