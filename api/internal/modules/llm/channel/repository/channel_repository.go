package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	officialmodel "github.com/zgiai/zgi/api/internal/modules/llm/officialmodel"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ErrCredentialBindingChanged = errors.New("route credential binding changed")

// ============================================================================
// Tenant Route Repository
// ============================================================================

// TenantRouteRepository defines the interface for tenant route operations
type TenantRouteRepository interface {
	Create(ctx context.Context, route *model.LLMRoute) error
	BatchCreate(ctx context.Context, routes []*model.LLMRoute) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.LLMRoute, error)
	List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.LLMRoute, int64, error)
	Update(ctx context.Context, route *model.LLMRoute) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	GetEnabledRoutes(ctx context.Context, organizationID uuid.UUID) ([]*model.LLMRoute, error)
	FindByModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*model.LLMRoute, error)
	CountByCredentialID(ctx context.Context, organizationID uuid.UUID, credentialID uuid.UUID) (int64, error)
	GetDistinctProviders(ctx context.Context, organizationID uuid.UUID) ([]string, error)
	GetPlatformChannels(ctx context.Context) ([]*model.LLMRoute, error)
}

type CredentialBindingUpdater interface {
	UpdateWithExpectedCredential(ctx context.Context, route *model.LLMRoute, expectedCredentialID *uuid.UUID) error
}

type tenantRouteRepository struct {
	db *gorm.DB
}

// NewTenantRouteRepository creates a new tenant route repository
func NewTenantRouteRepository(db *gorm.DB) TenantRouteRepository {
	return &tenantRouteRepository{db: db}
}

func (r *tenantRouteRepository) Create(ctx context.Context, route *model.LLMRoute) error {
	if err := r.db.WithContext(ctx).Create(route).Error; err != nil {
		return err
	}
	return r.persistModelOperations(ctx, route)
}

func (r *tenantRouteRepository) BatchCreate(ctx context.Context, routes []*model.LLMRoute) error {
	if len(routes) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).CreateInBatches(routes, 100).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if err := r.persistModelOperations(ctx, route); err != nil {
			return err
		}
	}
	return nil
}

func (r *tenantRouteRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.LLMRoute, error) {
	var route model.LLMRoute
	err := r.db.WithContext(ctx).
		Preload("TenantCredential").
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&route).Error
	if err != nil {
		return nil, err
	}
	if err := r.hydrateModelOperations(ctx, &route); err != nil {
		return nil, err
	}
	if err := officialmodel.HydrateRoute(ctx, r.db, &route); err != nil {
		return nil, err
	}
	return &route, nil
}

func (r *tenantRouteRepository) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.LLMRoute, int64, error) {
	var routes []*model.LLMRoute
	var total int64

	query := r.db.WithContext(ctx).Model(&model.LLMRoute{}).
		Where("organization_id = ?", organizationID)

	if isEnabled != nil {
		query = query.Where("is_enabled = ?", *isEnabled)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.
		Preload("TenantCredential").
		Order("priority DESC, weight DESC, created_at DESC").
		Offset(offset).Limit(limit).
		Find(&routes).Error; err != nil {
		return nil, 0, err
	}
	if err := officialmodel.HydrateRoutes(ctx, r.db, routes); err != nil {
		return nil, 0, err
	}
	if err := r.hydrateModelOperationsForRoutes(ctx, routes); err != nil {
		return nil, 0, err
	}

	return routes, total, nil
}

func (r *tenantRouteRepository) Update(ctx context.Context, route *model.LLMRoute) error {
	// Use Updates with Select to only update specific fields and avoid constraint violations
	if err := r.db.WithContext(ctx).Model(route).
		Select(routeUpdateFields(route, false)).
		Updates(route).Error; err != nil {
		return err
	}
	return r.persistModelOperations(ctx, route)
}

func (r *tenantRouteRepository) UpdateWithExpectedCredential(
	ctx context.Context,
	route *model.LLMRoute,
	expectedCredentialID *uuid.UUID,
) error {
	query := r.db.WithContext(ctx).
		Model(&model.LLMRoute{}).
		Where("id = ? AND organization_id = ?", route.ID, route.OrganizationID)
	if expectedCredentialID == nil {
		query = query.Where("user_credential_id IS NULL")
	} else {
		query = query.Where("user_credential_id = ?", *expectedCredentialID)
	}

	result := query.
		Select(routeUpdateFields(route, true)).
		Updates(route)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCredentialBindingChanged
	}
	return r.persistModelOperations(ctx, route)
}

func routeUpdateFields(route *model.LLMRoute, includeCredential bool) []string {
	fields := []string{
		"name",
		"provider",
		"api_base_url",
		"native_protocols",
		"model_maps",
		"param_override",
		"header_override",
		"validation_report",
		"tags",
		"description",
		"priority",
		"weight",
		"is_enabled",
		"auto_ban",
		"last_synced_at",
		"updated_at",
	}
	if includeCredential {
		fields = append(fields, "user_credential_id")
	}
	if !route.IsOfficial {
		fields = append(fields, "models")
	}
	return fields
}

func (r *tenantRouteRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.LLMRoute{}).Error
}

func (r *tenantRouteRepository) GetEnabledRoutes(ctx context.Context, organizationID uuid.UUID) ([]*model.LLMRoute, error) {
	var routes []*model.LLMRoute
	logger.DebugContext(ctx, "Enabled LLM routes query started", zap.Stringer("organization_id", organizationID))

	err := r.db.WithContext(ctx).
		Preload("TenantCredential", "deleted_at IS NULL").
		Joins("LEFT JOIN llm_credentials usr_cred ON llm_routes.user_credential_id = usr_cred.id").
		Where("llm_routes.organization_id = ? AND llm_routes.is_enabled = true", organizationID).
		Where("llm_routes.deleted_at IS NULL").
		Where(`(
			llm_routes.user_credential_id IS NULL
			OR (
usr_cred.id IS NOT NULL
AND usr_cred.is_active = true
AND usr_cred.deleted_at IS NULL
)
		)`).
		Order("llm_routes.priority DESC, llm_routes.weight DESC").
		Find(&routes).Error

	if err == nil {
		err = r.hydrateModelOperationsForRoutes(ctx, routes)
	}
	if err == nil {
		err = officialmodel.HydrateRoutes(ctx, r.db, routes)
	}

	fields := []interface{}{
		zap.Stringer("organization_id", organizationID),
		zap.Int("route_count", len(routes)),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		logger.WarnContext(ctx, "Enabled LLM routes query failed", fields...)
		return routes, err
	}

	logger.DebugContext(ctx, "Enabled LLM routes query completed", fields...)
	for i, route := range routes {
		logger.DebugContext(ctx, "Enabled LLM route selected",
			zap.Stringer("organization_id", organizationID),
			zap.Int("route_index", i),
			zap.Stringer("route_id", route.ID),
			zap.String("provider", route.ChannelProvider),
			zap.String("route_type", string(route.Type)),
			zap.Bool("is_official", route.IsOfficial),
			zap.Int("model_count", len(route.Models)),
		)
	}

	return routes, err
}

func (r *tenantRouteRepository) CountByCredentialID(ctx context.Context, organizationID uuid.UUID, credentialID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.LLMRoute{}).
		Where("organization_id = ? AND user_credential_id = ?", organizationID, credentialID).
		Count(&count).Error
	return count, err
}

func (r *tenantRouteRepository) GetDistinctProviders(ctx context.Context, organizationID uuid.UUID) ([]string, error) {
	var providers []string
	err := r.db.WithContext(ctx).
		Model(&model.LLMRoute{}).
		Where("organization_id = ? AND provider IS NOT NULL AND provider != ''", organizationID).
		Distinct("provider").
		Pluck("provider", &providers).Error
	return providers, err
}

// GetPlatformChannels returns all ZGI_CLOUD (official) routes across all organizations.
// Platform channels are global resources provided by ZGI Cloud.
func (r *tenantRouteRepository) GetPlatformChannels(ctx context.Context) ([]*model.LLMRoute, error) {
	var routes []*model.LLMRoute
	err := r.db.WithContext(ctx).
		Preload("TenantCredential").
		Where("type = ?", "ZGI_CLOUD").
		Order("priority DESC, weight DESC, created_at DESC").
		Find(&routes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get platform channels: %w", err)
	}
	if err := officialmodel.HydrateRoutes(ctx, r.db, routes); err != nil {
		return nil, fmt.Errorf("failed to hydrate platform channel models: %w", err)
	}
	if err := r.hydrateModelOperationsForRoutes(ctx, routes); err != nil {
		return nil, fmt.Errorf("failed to load platform channel operations: %w", err)
	}
	return routes, nil
}

func (r *tenantRouteRepository) hasModelOperationsColumn() bool {
	return r.db.Migrator().HasColumn("llm_routes", "model_operations")
}

func (r *tenantRouteRepository) persistModelOperations(ctx context.Context, route *model.LLMRoute) error {
	if route == nil || !r.hasModelOperationsColumn() {
		return nil
	}
	raw, err := json.Marshal(route.ModelOperations)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Table("llm_routes").Where("id = ?", route.ID).Update("model_operations", raw).Error
}

func (r *tenantRouteRepository) hydrateModelOperations(ctx context.Context, route *model.LLMRoute) error {
	return r.hydrateModelOperationsForRoutes(ctx, []*model.LLMRoute{route})
}

func (r *tenantRouteRepository) hydrateModelOperationsForRoutes(ctx context.Context, routes []*model.LLMRoute) error {
	if len(routes) == 0 || !r.hasModelOperationsColumn() {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(routes))
	byID := make(map[uuid.UUID]*model.LLMRoute, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		ids = append(ids, route.ID)
		byID[route.ID] = route
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		ID  uuid.UUID `gorm:"column:id"`
		Raw []byte    `gorm:"column:model_operations"`
	}
	if err := r.db.WithContext(ctx).Table("llm_routes").Select("id, model_operations").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if route := byID[row.ID]; route != nil && len(row.Raw) > 0 && string(row.Raw) != "null" {
			_ = json.Unmarshal(row.Raw, &route.ModelOperations)
		}
	}
	return nil
}

// FindByModel finds all enabled routes that support a specific model
// Uses JSONB containment operator (@>) for efficient querying
func (r *tenantRouteRepository) FindByModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*model.LLMRoute, error) {
	var routes []*model.LLMRoute

	err := r.db.WithContext(ctx).
		Preload("TenantCredential").
		Where("organization_id = ?", organizationID).
		Where("is_enabled = ?", true).
		Where("is_official = ?", false).
		Where("models @> ?", fmt.Sprintf(`["%s"]`, modelName)).
		Order("priority DESC, weight DESC").
		Find(&routes).Error

	if err != nil {
		return nil, fmt.Errorf("failed to find routes by model %s: %w", modelName, err)
	}

	effectiveModels, err := officialmodel.GetEffectiveModels(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("failed to load official model snapshot: %w", err)
	}
	if officialmodel.ContainsModel(effectiveModels, modelName) {
		var officialRoutes []*model.LLMRoute
		err := r.db.WithContext(ctx).
			Preload("TenantCredential").
			Where("organization_id = ? AND is_enabled = ? AND is_official = ?", organizationID, true, true).
			Order("priority DESC, weight DESC").
			Find(&officialRoutes).Error
		if err != nil {
			return nil, fmt.Errorf("failed to find official routes by model %s: %w", modelName, err)
		}
		if err := officialmodel.HydrateRoutes(ctx, r.db, officialRoutes); err != nil {
			return nil, fmt.Errorf("failed to hydrate official routes by model %s: %w", modelName, err)
		}
		routes = append(routes, officialRoutes...)
	}
	if err := r.hydrateModelOperationsForRoutes(ctx, routes); err != nil {
		return nil, fmt.Errorf("failed to load route operations for model %s: %w", modelName, err)
	}

	return routes, nil
}
