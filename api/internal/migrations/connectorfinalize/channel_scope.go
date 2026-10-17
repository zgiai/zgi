package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

// RestoreNativeChannelScope reuses the fixed post-retirement scope validator
// when a verified schema2 deployment installs DigitalHuman channels later.
// It neither executes retirement DDL nor recreates any legacy relation.
func RestoreNativeChannelScope(db *gorm.DB) error {
	if err := db.Exec(channelScopeWithoutLegacyConnectionSQL).Error; err != nil {
		return fmt.Errorf("restore native DigitalHuman channel scope: %w", err)
	}
	return nil
}
