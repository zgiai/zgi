package connections

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

// Lock order is admission budget -> onboarding flows -> owner -> connection,
// matching registration's flow -> connection finalization order. This must run
// before ForceRemoveConnection obtains a connection row lock.
func cancelAppOnboarding(ctx context.Context, tx *gorm.DB, c core.PersonalConnection) error {
	if c.AuthMethodID != "feishu.app_credentials" {
		return nil
	}
	if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended('connector-onboarding-budget', 0))").Error; err != nil {
		return internalError("cancel app setup")
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE connector_onboarding_flows SET state = 'cancelled', terminal_at = CURRENT_TIMESTAMP,
 updated_at = CURRENT_TIMESTAMP, revision = revision + 1, lease_owner = NULL, lease_until = NULL, safe_error_code = NULL
 WHERE organization_id = ? AND owner_account_id = ? AND connection_id = ? AND state IN ('starting','awaiting_authorization','verifying')`, c.OrganizationID, c.OwnerAccountID, c.ID).Error; err != nil {
		return internalError("cancel app setup")
	}
	if err := tx.WithContext(ctx).Exec(`DELETE FROM connector_onboarding_secrets WHERE organization_id = ? AND owner_account_id = ?
 AND flow_id IN (SELECT id FROM connector_onboarding_flows WHERE organization_id = ? AND owner_account_id = ? AND connection_id = ?)`, c.OrganizationID, c.OwnerAccountID, c.OrganizationID, c.OwnerAccountID, c.ID).Error; err != nil {
		return internalError("cancel app setup")
	}
	return nil
}
