package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"plugin_runner/internal/dataplane"
	"plugin_runner/internal/plugin"
)

// Registry stores plugin manifests in DB/Redis (if configured) with an in-memory fallback.
type Registry struct {
	db    *gorm.DB
	cache redis.UniversalClient
	log   *zap.Logger

	mu      sync.RWMutex
	memory  map[string]plugin.Manifest
	enabled bool
}

type Option func(*Registry)

// New builds a Registry backed by DB/Redis if available.
func New(conns *dataplane.Connections, log *zap.Logger) *Registry {
	r := &Registry{
		log:     log,
		memory:  make(map[string]plugin.Manifest),
		enabled: conns != nil && conns.ORM != nil,
	}
	if conns != nil {
		r.db = conns.ORM
		r.cache = conns.Cache
	}
	return r
}

// Save inserts or updates a manifest.
func (r *Registry) Save(ctx context.Context, manifest plugin.Manifest) (*plugin.Manifest, error) {
	key := manifestKey(manifest)
	if key == "" {
		return nil, fmt.Errorf("manifest must include author, name,version or id")
	}

	if manifest.ID == "" {
		manifest.ID = key
	}

	payload, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}

	if r.enabled && r.db != nil {
		if err := r.upsertDB(ctx, manifest, string(payload)); err != nil {
			return nil, err
		}
	}

	if r.cache != nil {
		if err := r.cache.Set(ctx, cacheKey(key), payload, 0).Err(); err != nil {
			r.log.Warn("set manifest cache failed", zap.String("key", key), zap.Error(err))
		}
	}

	r.mu.Lock()
	// TODO: this uses a plain map for now and should be improved later
	r.memory[key] = manifest
	r.mu.Unlock()
	return &manifest, nil
}

// List returns all manifests.
func (r *Registry) List(ctx context.Context) ([]plugin.Manifest, error) {
	if r.enabled && r.db != nil {
		var records []dataplane.PluginRecord
		if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
			return nil, fmt.Errorf("list manifests: %w", err)
		}
		return recordsToManifests(records)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []plugin.Manifest
	for _, m := range r.memory {
		out = append(out, m)
	}
	return out, nil
}

// Get finds a manifest by marketplace_version_id, with external_id as fallback.
func (r *Registry) Get(ctx context.Context, id string) (*plugin.Manifest, error) {
	key := normalizeKey(id)

	if r.cache != nil {
		if m, ok := r.getFromCache(ctx, key); ok {
			return m, nil
		}
	}

	if r.enabled && r.db != nil {
		if m, ok, err := r.getFromDB(ctx, key); err != nil {
			return nil, err
		} else if ok {
			return m, nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.memory[key]; ok {
		return &m, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// Delete removes a manifest from registry.
func (r *Registry) Delete(ctx context.Context, id string) error {
	key := normalizeKey(id)

	if r.enabled && r.db != nil {
		if err := r.deleteFromDB(ctx, key); err != nil {
			return err
		}
	}
	if r.cache != nil {
		if err := r.cache.Del(ctx, cacheKey(key)).Err(); err != nil && !errors.Is(err, redis.Nil) {
			r.log.Warn("delete manifest cache failed", zap.String("key", key), zap.Error(err))
		}
	}

	r.mu.Lock()
	delete(r.memory, key)
	r.mu.Unlock()
	return nil
}

func (r *Registry) upsertDB(ctx context.Context, manifest plugin.Manifest, manifestJSON string) error {
	record := dataplane.PluginRecord{
		ExternalID:           manifest.ID,
		MarketplacePluginID:  manifest.MarketplacePluginID,
		MarketplaceVersionID: manifest.MarketplaceVersionID,
		Name:                 manifest.Name,
		Version:              manifest.Version,
		ManifestJSON:         manifestJSON,
		Status:               "active",
	}

	tx := r.db.WithContext(ctx).Where("name = ? AND version = ?", manifest.Name, manifest.Version).
		Assign(record).
		FirstOrCreate(&record)
	if tx.Error != nil {
		return fmt.Errorf("save manifest: %w", tx.Error)
	}
	return nil
}

func (r *Registry) getFromDB(ctx context.Context, key string) (*plugin.Manifest, bool, error) {
	record, found, err := r.findRecordByKey(ctx, key)
	if err != nil {
		return nil, false, fmt.Errorf("get manifest: %w", err)
	}
	if !found {
		return nil, false, nil
	}

	manifest, err := decodeManifest(record.ManifestJSON)
	if err != nil {
		return nil, false, err
	}
	return manifest, true, nil
}

func (r *Registry) deleteFromDB(ctx context.Context, key string) error {
	rec, found, err := r.findRecordByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("find plugin for delete: %w", err)
	}
	if !found {
		return nil // Already deleted
	}

	// Soft delete related records (GORM handles DeletedAt automatically)
	if err := r.db.WithContext(ctx).Where("plugin_id = ?", rec.ID).Delete(&dataplane.PluginTenantBinding{}).Error; err != nil {
		r.log.Warn("failed to soft delete tenant bindings", zap.Uint("plugin_id", rec.ID), zap.Error(err))
	}
	if err := r.db.WithContext(ctx).Where("plugin_id = ?", rec.ID).Delete(&dataplane.PluginInstall{}).Error; err != nil {
		r.log.Warn("failed to soft delete install records", zap.Uint("plugin_id", rec.ID), zap.Error(err))
	}
	// PluginRun does not have soft delete - keep for audit

	// Soft delete the plugin record itself
	if err := r.db.WithContext(ctx).Delete(rec).Error; err != nil {
		return fmt.Errorf("soft delete plugin: %w", err)
	}
	return nil
}

func (r *Registry) getFromCache(ctx context.Context, key string) (*plugin.Manifest, bool) {
	val, err := r.cache.Get(ctx, cacheKey(key)).Result()
	if err != nil {
		return nil, false
	}
	manifest, err := decodeManifest(val)
	if err != nil {
		r.log.Warn("decode manifest cache failed", zap.String("key", key), zap.Error(err))
		return nil, false
	}
	return manifest, true
}

func manifestKey(m plugin.Manifest) string {
	if strings.TrimSpace(m.ID) != "" {
		return normalizeKey(m.ID)
	}
	if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.Author) == "" {
		return ""
	}
	// Generate key in format: author:name:version
	return fmt.Sprintf("%s:%s:%s", strings.TrimSpace(m.Author), strings.TrimSpace(m.Name), strings.TrimSpace(m.Version))

}

func normalizeKey(key string) string {
	key = strings.TrimSpace(key)
	return key
}

func cacheKey(key string) string {
	return fmt.Sprintf("plugin:meta:%s", key)
}

func decodeManifest(payload string) (*plugin.Manifest, error) {
	var m plugin.Manifest
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &m, nil
}

func recordsToManifests(records []dataplane.PluginRecord) ([]plugin.Manifest, error) {
	var out []plugin.Manifest
	for _, rec := range records {
		m, err := decodeManifest(rec.ManifestJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, nil
}

func (r *Registry) findRecordByKey(ctx context.Context, key string) (*dataplane.PluginRecord, bool, error) {
	if r.db == nil {
		return nil, false, fmt.Errorf("database not configured")
	}

	rec, err := r.getRecordByField(ctx, "marketplace_version_id", key)
	if err == nil {
		return rec, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	rec, err = r.getRecordByField(ctx, "external_id", key)
	if err == nil {
		return rec, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (r *Registry) getRecordByField(ctx context.Context, field, value string) (*dataplane.PluginRecord, error) {
	var rec dataplane.PluginRecord
	if err := r.db.WithContext(ctx).Where(field+" = ?", value).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// Tenant operations

func (r *Registry) CreateTenant(ctx context.Context, name string) (*dataplane.Tenant, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	t := &dataplane.Tenant{Name: strings.TrimSpace(name)}
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return t, nil
}

func (r *Registry) ListTenants(ctx context.Context) ([]dataplane.Tenant, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	var tenants []dataplane.Tenant
	if err := r.db.WithContext(ctx).Find(&tenants).Error; err != nil {
		return nil, err
	}
	return tenants, nil
}

func (r *Registry) BindPluginTenant(ctx context.Context, pluginID uint, tenantID uint, configJSON string, enabled bool) error {
	if r.db == nil {
		return fmt.Errorf("database not configured")
	}
	binding := dataplane.PluginTenantBinding{
		PluginID:   pluginID,
		TenantID:   tenantID,
		ConfigJSON: configJSON,
		Enabled:    enabled,
	}
	return r.db.WithContext(ctx).
		Where("plugin_id = ? AND tenant_id = ?", pluginID, tenantID).
		Assign(binding).
		FirstOrCreate(&binding).Error
}

func (r *Registry) ListPluginTenants(ctx context.Context, pluginID uint) ([]dataplane.PluginTenantBinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	var bindings []dataplane.PluginTenantBinding
	if err := r.db.WithContext(ctx).Where("plugin_id = ?", pluginID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (r *Registry) ResolvePluginRecord(ctx context.Context, id string) (*dataplane.PluginRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	key := normalizeKey(id)
	rec, found, err := r.findRecordByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, gorm.ErrRecordNotFound
	}
	return rec, nil
}

// GetTenantBinding returns the binding for a specific plugin and tenant.
// Returns nil if no binding exists or if database is not configured.
func (r *Registry) GetTenantBinding(ctx context.Context, pluginID uint, tenantID uint) (*dataplane.PluginTenantBinding, error) {
	if r.db == nil {
		return nil, nil // No database, allow access (single-tenant mode)
	}
	var binding dataplane.PluginTenantBinding
	err := r.db.WithContext(ctx).
		Where("plugin_id = ? AND tenant_id = ?", pluginID, tenantID).
		First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &binding, nil
}

// CheckTenantAccess verifies if a tenant has enabled access to a plugin.
// Returns the binding if access is granted, otherwise returns an error.
func (r *Registry) CheckTenantAccess(ctx context.Context, pluginName, pluginVersion string, tenantID uint) (*dataplane.PluginTenantBinding, error) {
	if r.db == nil {
		return nil, nil // No database, allow access (single-tenant mode)
	}

	// Find plugin record first
	var pluginRec dataplane.PluginRecord
	if err := r.db.WithContext(ctx).
		Where("name = ? AND version = ?", pluginName, pluginVersion).
		First(&pluginRec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("plugin %s:%s not found", pluginName, pluginVersion)
		}
		return nil, err
	}

	// Check tenant binding
	binding, err := r.GetTenantBinding(ctx, pluginRec.ID, tenantID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, fmt.Errorf("tenant %d is not bound to plugin %s:%s", tenantID, pluginName, pluginVersion)
	}
	if !binding.Enabled {
		return nil, fmt.Errorf("tenant %d access to plugin %s:%s is disabled", tenantID, pluginName, pluginVersion)
	}
	return binding, nil
}
