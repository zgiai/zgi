package agentbindings

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"gorm.io/gorm"
)

type Repository struct {
	db          *gorm.DB
	tokenSecret []byte
}

func NewRepository(db *gorm.DB) *Repository {
	secret := ""
	if appconfig.GlobalConfig != nil {
		secret = strings.TrimSpace(appconfig.GlobalConfig.App.SecretKey)
	}
	return NewRepositoryWithTokenSecret(db, secret)
}

func NewRepositoryWithTokenSecret(db *gorm.DB, secret string) *Repository {
	return &Repository{db: db, tokenSecret: []byte(strings.TrimSpace(secret))}
}

func (r *Repository) WithTx(tx *gorm.DB) *Repository {
	if tx == nil {
		return r
	}
	return &Repository{db: tx, tokenSecret: append([]byte(nil), r.tokenSecret...)}
}

func (r *Repository) ListScope(ctx context.Context, ref ScopeRef) ([]Binding, error) {
	if err := validateScopeRef(ref); err != nil {
		return nil, err
	}
	var bindings []Binding
	query := r.db.WithContext(ctx).Where("agent_id = ? AND binding_scope = ?", ref.AgentID, ref.Scope)
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	if err := query.Order("binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list agent resource bindings: %w", err)
	}
	return bindings, nil
}

// HasBinding rechecks one concrete tool-step resource against the current
// draft or published-head index. Callers should invoke it for every step and
// continuation rather than relying only on a prepared RunConfig snapshot.
func (r *Repository) HasBinding(ctx context.Context, ref ScopeRef, match Match) (bool, error) {
	if err := validateScopeRef(ref); err != nil {
		return false, err
	}
	if r == nil || r.db == nil {
		return false, fmt.Errorf("agent bindings database is required")
	}
	if match.BindingType == "" || strings.TrimSpace(match.ResourceID) == "" {
		return false, fmt.Errorf("binding type and resource id are required")
	}
	query := r.db.WithContext(ctx).Model(&Binding{}).
		Where("agent_id = ? AND binding_scope = ? AND binding_type = ? AND resource_id = ? AND parent_resource_id = ?",
			ref.AgentID, ref.Scope, match.BindingType, strings.TrimSpace(match.ResourceID), strings.TrimSpace(match.ParentResourceID))
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	switch strings.ToLower(strings.TrimSpace(match.AccessMode)) {
	case "read":
		query = query.Where("access_mode IN ?", []string{"read", "write"})
	case "write", "execute":
		query = query.Where("access_mode = ?", strings.ToLower(strings.TrimSpace(match.AccessMode)))
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check agent resource binding: %w", err)
	}
	return count > 0, nil
}

// LockResources serializes binding creation, rollback, publish, move, and
// deletion for the same concrete resources. Callers must pass their active
// transaction so PostgreSQL holds these advisory locks until commit/rollback.
func (r *Repository) LockResources(ctx context.Context, tx *gorm.DB, refs []ResourceRef) error {
	if tx == nil {
		return fmt.Errorf("agent binding resource lock transaction is required")
	}
	keys := map[string]struct{}{}
	for _, ref := range refs {
		if ref.OrganizationID == uuid.Nil || ref.BindingType == "" || strings.TrimSpace(ref.ResourceID) == "" {
			return fmt.Errorf("organization, binding type, and resource id are required for agent binding resource lock")
		}
		keys[resourceLockKey(ref.OrganizationID, ref.BindingType, ref.ParentResourceID, ref.ResourceID)] = struct{}{}
		if ref.BindingType == BindingTypeDatabaseTable && strings.TrimSpace(ref.ParentResourceID) != "" {
			keys[resourceLockKey(ref.OrganizationID, BindingTypeDatabase, "", ref.ParentResourceID)] = struct{}{}
		}
		if ref.BindingType == BindingTypeWorkflow && strings.TrimSpace(ref.ParentResourceID) != "" {
			keys[resourceLockKey(ref.OrganizationID, BindingTypeWorkflow, "", ref.ParentResourceID)] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error; err != nil {
			return fmt.Errorf("lock agent binding resource: %w", err)
		}
	}
	return nil
}

// LockAgents serializes Agent workspace moves with config mutations even when
// the Agent currently has no resource binding rows.
func (r *Repository) LockAgents(ctx context.Context, tx *gorm.DB, agentIDs []uuid.UUID) error {
	if tx == nil {
		return fmt.Errorf("agent binding agent lock transaction is required")
	}
	keys := map[string]struct{}{}
	for _, agentID := range agentIDs {
		if agentID == uuid.Nil {
			return fmt.Errorf("agent id is required for agent binding agent lock")
		}
		keys[agentScopeLockKey(agentID)] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error; err != nil {
			return fmt.Errorf("lock agent binding agent scope: %w", err)
		}
	}
	return nil
}

func resourceLockKey(organizationID uuid.UUID, bindingType BindingType, parentResourceID, resourceID string) string {
	return strings.Join([]string{
		"agent-binding-resource",
		organizationID.String(),
		string(bindingType),
		strings.TrimSpace(parentResourceID),
		strings.TrimSpace(resourceID),
	}, ":")
}

func agentScopeLockKey(agentID uuid.UUID) string {
	return "agent-binding-agent:" + agentID.String()
}

func (r *Repository) ReplaceScope(ctx context.Context, tx *gorm.DB, ref ScopeRef, bindings []Binding) error {
	if err := validateScopeRef(ref); err != nil {
		return err
	}
	db := r.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return fmt.Errorf("agent bindings database is required")
	}
	replace := func(current *gorm.DB) error {
		deleteQuery := current.WithContext(ctx).Where("agent_id = ? AND binding_scope = ?", ref.AgentID, ref.Scope)
		deleteQuery = applyVersionScope(deleteQuery, ref.PublishedVersionUUID)
		if err := deleteQuery.Delete(&Binding{}).Error; err != nil {
			return fmt.Errorf("clear agent resource binding scope: %w", err)
		}
		if len(bindings) == 0 {
			return nil
		}
		now := time.Now()
		rows := make([]Binding, 0, len(bindings))
		for _, binding := range bindings {
			binding.ID = uuid.Nil
			binding.AgentID = ref.AgentID
			binding.BindingScope = ref.Scope
			binding.PublishedVersionUUID = cloneUUID(ref.PublishedVersionUUID)
			binding.ResourceID = strings.TrimSpace(binding.ResourceID)
			binding.ParentResourceID = strings.TrimSpace(binding.ParentResourceID)
			binding.DisplayName = strings.TrimSpace(binding.DisplayName)
			binding.AccessMode = strings.TrimSpace(binding.AccessMode)
			binding.CreatedAt = now
			binding.UpdatedAt = now
			rows = append(rows, binding)
		}
		if err := current.WithContext(ctx).Create(&rows).Error; err != nil {
			return fmt.Errorf("create agent resource bindings: %w", err)
		}
		return nil
	}
	if tx != nil {
		return replace(db)
	}
	return db.WithContext(ctx).Transaction(replace)
}

// ReplacePublishedHead atomically removes every historical published binding
// scope for an Agent and writes the newly published version as the sole head.
func (r *Repository) ReplacePublishedHead(ctx context.Context, tx *gorm.DB, ref ScopeRef, bindings []Binding) error {
	if err := validateScopeRef(ref); err != nil {
		return err
	}
	if ref.Scope != ScopePublished {
		return fmt.Errorf("published head replacement requires published scope")
	}
	db := r.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return fmt.Errorf("agent bindings database is required")
	}
	replace := func(current *gorm.DB) error {
		if err := current.WithContext(ctx).
			Where("agent_id = ? AND binding_scope = ?", ref.AgentID, ScopePublished).
			Delete(&Binding{}).Error; err != nil {
			return fmt.Errorf("clear historical agent published bindings: %w", err)
		}
		return r.ReplaceScope(ctx, current, ref, bindings)
	}
	if tx != nil {
		return replace(db)
	}
	return db.WithContext(ctx).Transaction(replace)
}

func (r *Repository) ListImpact(ctx context.Context, ref ResourceRef) ([]Binding, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	if ref.OrganizationID == uuid.Nil || strings.TrimSpace(ref.ResourceID) == "" || ref.BindingType == "" {
		return nil, fmt.Errorf("organization, binding type, and resource id are required")
	}
	query := r.db.WithContext(ctx).Where("organization_id = ?", ref.OrganizationID)
	if ref.BindingType == BindingTypeDatabase {
		query = query.Where(
			"(binding_type = ? AND resource_id = ?) OR (binding_type = ? AND parent_resource_id = ?)",
			BindingTypeDatabase, strings.TrimSpace(ref.ResourceID), BindingTypeDatabaseTable, strings.TrimSpace(ref.ResourceID),
		)
	} else if ref.BindingType == BindingTypeWorkflow {
		query = query.Where(
			"binding_type = ? AND (resource_id = ? OR parent_resource_id = ?)",
			BindingTypeWorkflow, strings.TrimSpace(ref.ResourceID), strings.TrimSpace(ref.ResourceID),
		)
	} else {
		query = query.Where("binding_type = ? AND resource_id = ?", ref.BindingType, strings.TrimSpace(ref.ResourceID))
	}
	if ref.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *ref.WorkspaceID)
	}
	if ref.AgentID != nil {
		query = query.Where("agent_id = ?", *ref.AgentID)
	}
	if ref.Scope != nil {
		query = query.Where("binding_scope = ?", *ref.Scope)
	}
	if parentID := strings.TrimSpace(ref.ParentResourceID); parentID != "" {
		query = query.Where("parent_resource_id = ?", parentID)
	}
	var bindings []Binding
	if err := query.Order("agent_id ASC, binding_scope ASC, published_version_uuid ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list agent resource binding impact: %w", err)
	}
	return bindings, nil
}

func (r *Repository) RevokeResource(ctx context.Context, tx *gorm.DB, ref ResourceRef) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return fmt.Errorf("agent bindings database is required")
	}
	if ref.OrganizationID == uuid.Nil || strings.TrimSpace(ref.ResourceID) == "" || ref.BindingType == "" {
		return fmt.Errorf("organization, binding type, and resource id are required")
	}
	query := db.WithContext(ctx).Where("organization_id = ?", ref.OrganizationID)
	if ref.BindingType == BindingTypeDatabase {
		query = query.Where(
			"(binding_type = ? AND resource_id = ?) OR (binding_type = ? AND parent_resource_id = ?)",
			BindingTypeDatabase, strings.TrimSpace(ref.ResourceID), BindingTypeDatabaseTable, strings.TrimSpace(ref.ResourceID),
		)
	} else if ref.BindingType == BindingTypeWorkflow {
		query = query.Where(
			"binding_type = ? AND (resource_id = ? OR parent_resource_id = ?)",
			BindingTypeWorkflow, strings.TrimSpace(ref.ResourceID), strings.TrimSpace(ref.ResourceID),
		)
	} else {
		query = query.Where("binding_type = ? AND resource_id = ?", ref.BindingType, strings.TrimSpace(ref.ResourceID))
	}
	if ref.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *ref.WorkspaceID)
	}
	if ref.AgentID != nil {
		query = query.Where("agent_id = ?", *ref.AgentID)
	}
	if ref.Scope != nil {
		query = query.Where("binding_scope = ?", *ref.Scope)
	}
	if parentID := strings.TrimSpace(ref.ParentResourceID); parentID != "" {
		query = query.Where("parent_resource_id = ?", parentID)
	}
	if err := query.Delete(&Binding{}).Error; err != nil {
		return fmt.Errorf("revoke agent resource bindings: %w", err)
	}
	if ref.BindingType == BindingTypeDatabaseTable {
		if err := removeEmptyDatabaseParentBindings(ctx, db, ref); err != nil {
			return err
		}
	}
	return nil
}

func removeEmptyDatabaseParentBindings(ctx context.Context, db *gorm.DB, ref ResourceRef) error {
	parentResourceID := strings.TrimSpace(ref.ParentResourceID)
	if parentResourceID == "" {
		return fmt.Errorf("database table binding requires parent resource id")
	}
	clauses := []string{
		"parent.organization_id = ?",
		"parent.binding_type = ?",
		"parent.resource_id = ?",
		`NOT EXISTS (
			SELECT 1
			FROM agent_resource_bindings AS child
			WHERE child.agent_id = parent.agent_id
			  AND child.binding_scope = parent.binding_scope
			  AND child.published_version_uuid IS NOT DISTINCT FROM parent.published_version_uuid
			  AND child.binding_type = ?
			  AND child.parent_resource_id = parent.resource_id
		)`,
	}
	args := []interface{}{ref.OrganizationID, BindingTypeDatabase, parentResourceID, BindingTypeDatabaseTable}
	if ref.WorkspaceID != nil {
		clauses = append(clauses, "parent.workspace_id = ?")
		args = append(args, *ref.WorkspaceID)
	}
	if ref.AgentID != nil {
		clauses = append(clauses, "parent.agent_id = ?")
		args = append(args, *ref.AgentID)
	}
	if ref.Scope != nil {
		clauses = append(clauses, "parent.binding_scope = ?")
		args = append(args, *ref.Scope)
	}
	statement := "DELETE FROM agent_resource_bindings AS parent WHERE " + strings.Join(clauses, " AND ")
	if err := db.WithContext(ctx).Exec(statement, args...).Error; err != nil {
		return fmt.Errorf("clear empty agent database parent bindings: %w", err)
	}
	return nil
}

func validateScopeRef(ref ScopeRef) error {
	if ref.AgentID == uuid.Nil {
		return fmt.Errorf("agent id is required")
	}
	switch ref.Scope {
	case ScopeDraft:
		if ref.PublishedVersionUUID != nil {
			return fmt.Errorf("draft binding scope cannot have a published version")
		}
	case ScopePublished:
		if ref.PublishedVersionUUID == nil || *ref.PublishedVersionUUID == uuid.Nil {
			return fmt.Errorf("published binding scope requires a version")
		}
	default:
		return fmt.Errorf("invalid agent binding scope %q", ref.Scope)
	}
	return nil
}

func applyVersionScope(query *gorm.DB, versionUUID *uuid.UUID) *gorm.DB {
	if versionUUID == nil {
		return query.Where("published_version_uuid IS NULL")
	}
	return query.Where("published_version_uuid = ?", *versionUUID)
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
