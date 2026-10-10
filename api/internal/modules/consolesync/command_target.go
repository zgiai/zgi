// Command targeting (plan A2 / F4-c): Console names the target organization of an
// organization-scoped command in payload.organization_id (a pre organization id). The
// command is applied to that organization only; an unknown id fails instead of guessing.
//
// A command without the key comes from an older Console. It falls back to the old
// "the only organization of this database" rule with a warning; remove the fallback
// once every Console sends the target.
package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
)

// commandTargetWarn is replaced by tests.
var commandTargetWarn = log.Printf

// commandOrganization returns the organization a command applies to and whether the
// payload named it explicitly.
func (a *Applier) commandOrganization(
	ctx context.Context,
	tx *gorm.DB,
	commandType string,
	payload map[string]json.RawMessage,
) (string, bool, error) {
	organizationID := rawString(payload, "organization_id")
	if organizationID == "" {
		commandTargetWarn("[consolesync] %s without organization_id (older console); falling back to the only organization of this database", commandType)
		resolved, err := a.store.ResolveOrganizationIDWith(ctx, tx)
		return resolved, false, err
	}
	var organization struct {
		ID string `gorm:"column:id"`
	}
	err := tx.WithContext(ctx).Table("organizations").
		Select("id").Where("id = ?", organizationID).Take(&organization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", true, fmt.Errorf("%s: organization %s does not exist", commandType, organizationID)
	}
	if err != nil {
		return "", true, fmt.Errorf("%s: read organization: %w", commandType, err)
	}
	return organizationID, true, nil
}

// requireWorkspaceInOrganization refuses a workspace that is unknown or belongs to
// another organization.
func requireWorkspaceInOrganization(ctx context.Context, tx *gorm.DB, workspaceID, organizationID string) error {
	var workspace struct {
		OrganizationID *string `gorm:"column:organization_id"`
	}
	err := tx.WithContext(ctx).Table("workspaces").
		Select("organization_id").Where("id = ?", workspaceID).Take(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("quota.set: workspace %s does not exist", workspaceID)
	}
	if err != nil {
		return fmt.Errorf("quota.set: read workspace: %w", err)
	}
	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		return fmt.Errorf("quota.set: workspace %s does not belong to organization %s", workspaceID, organizationID)
	}
	return nil
}

// scopeAPIKey limits an API key lookup to the command's organization when it is named.
func scopeAPIKey(query *gorm.DB, organizationID string) *gorm.DB {
	if organizationID == "" {
		return query
	}
	return query.Where("organization_id = ?", organizationID)
}
