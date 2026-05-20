package gateway

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetShadowOrganizationID returns the shadow tenant ID (enterprise group ID) for a tenant
// If the tenant belongs to an enterprise group, it returns the group ID
// Otherwise, it returns the original tenant ID
// Optimized: Single query with LEFT JOIN to reduce database roundtrips
func GetShadowOrganizationID(ctx context.Context, db *gorm.DB, organizationID uuid.UUID) (uuid.UUID, error) {
	// Single optimized query using LEFT JOIN
	var result struct {
		OrganizationID *string `gorm:"column:organization_id"`
		IsOrganization bool    `gorm:"column:is_organization"`
	}

	err := db.WithContext(ctx).Raw(`
		SELECT 
			w.organization_id,
			CASE WHEN o.id IS NOT NULL THEN true ELSE false END as is_organization
		FROM (SELECT ?::uuid as id) t
		LEFT JOIN workspaces w ON w.id = t.id
		LEFT JOIN organizations o ON o.id = t.id
		LIMIT 1
	`, organizationID.String()).Scan(&result).Error

	if err != nil {
		// On error, fallback to original tenant ID
		return organizationID, nil
	}

	// If it's an organization (enterprise group), return it directly
	if result.IsOrganization {
		return organizationID, nil
	}

	// If workspace has organization_id, return it
	if result.OrganizationID != nil && *result.OrganizationID != "" {
		groupID, parseErr := uuid.Parse(*result.OrganizationID)
		if parseErr != nil {
			return organizationID, nil
		}
		return groupID, nil
	}

	// No enterprise group found, use original tenant ID
	return organizationID, nil
}

// GetShadowTenantOwnerID returns the owner account ID for a shadow tenant (enterprise group)
// This is used to create AI credit accounts
func GetShadowTenantOwnerID(ctx context.Context, db *gorm.DB, shadowOrganizationID uuid.UUID) (uuid.UUID, error) {
	// Query members to find the owner
	var join struct {
		AccountID string `gorm:"column:account_id"`
	}

	err := db.WithContext(ctx).Table("members").
		Where("organization_id = ? AND role = ?", shadowOrganizationID.String(), "owner").
		First(&join).Error

	if err != nil {
		return uuid.Nil, err
	}

	accountID, parseErr := uuid.Parse(join.AccountID)
	if parseErr != nil {
		return uuid.Nil, parseErr
	}

	return accountID, nil
}

// resolveShadowContext returns unified shadow tenant context used by all gateway flows.
// ownerID may be uuid.Nil if owner lookup fails and caller can continue (e.g. remote billing path).
func (s *llmGatewayServiceImpl) resolveShadowContext(
	ctx context.Context,
	organizationID uuid.UUID,
) (shadowOrganizationID uuid.UUID, ownerID uuid.UUID, err error) {
	return s.getShadowTenantInfo(ctx, organizationID)
}
