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
	query := r.db.WithContext(ctx).Where(
		"agent_id = ? AND consumer_type = ? AND binding_scope = ?",
		ref.AgentID, ConsumerTypeAgent, ref.Scope,
	)
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	if err := query.Order("binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list agent resource bindings: %w", err)
	}
	return bindings, nil
}

// ListWorkflowScope returns the bindings extracted from one concrete Workflow
// draft or published graph. It never mixes Agent runtime-config bindings into
// the result, even though both consumers share the same ledger table.
func (r *Repository) ListWorkflowScope(ctx context.Context, ref WorkflowScopeRef) ([]Binding, error) {
	if err := validateWorkflowScopeRef(ref); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	var bindings []Binding
	query := r.db.WithContext(ctx).Where(
		"agent_id = ? AND consumer_type = ? AND consumer_id = ? AND binding_scope = ?",
		ref.AgentID, ConsumerTypeWorkflow, ref.WorkflowID, ref.Scope,
	)
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	if err := query.Order("binding_instance_id ASC, binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list workflow resource bindings: %w", err)
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
		Where("agent_id = ? AND consumer_type = ? AND binding_scope = ? AND binding_type = ? AND resource_id = ? AND parent_resource_id = ?",
			ref.AgentID, ConsumerTypeAgent, ref.Scope, match.BindingType, strings.TrimSpace(match.ResourceID), strings.TrimSpace(match.ParentResourceID))
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

// FindBinding returns one concrete binding from the effective draft or
// published scope. It is useful when authorization depends on binding metadata
// in addition to the resource identity.
func (r *Repository) FindBinding(ctx context.Context, ref ScopeRef, match Match) (*Binding, error) {
	if err := validateScopeRef(ref); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	if match.BindingType == "" || strings.TrimSpace(match.ResourceID) == "" {
		return nil, fmt.Errorf("binding type and resource id are required")
	}
	query := r.db.WithContext(ctx).
		Where("agent_id = ? AND consumer_type = ? AND binding_scope = ? AND binding_type = ? AND resource_id = ? AND parent_resource_id = ?",
			ref.AgentID, ConsumerTypeAgent, ref.Scope, match.BindingType, strings.TrimSpace(match.ResourceID), strings.TrimSpace(match.ParentResourceID))
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	switch strings.ToLower(strings.TrimSpace(match.AccessMode)) {
	case "read":
		query = query.Where("access_mode IN ?", []string{"read", "write"})
	case "write", "execute":
		query = query.Where("access_mode = ?", strings.ToLower(strings.TrimSpace(match.AccessMode)))
	}
	var binding Binding
	if err := query.Take(&binding).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find agent resource binding: %w", err)
	}
	return &binding, nil
}

// HasWorkflowNodeBinding rechecks the exact connector node binding against
// the current Workflow graph scope. Callers should perform this check before
// every connector node execution instead of trusting graph JSON alone.
func (r *Repository) HasWorkflowNodeBinding(
	ctx context.Context,
	ref WorkflowScopeRef,
	nodeID string,
	match Match,
) (bool, error) {
	binding, err := r.FindWorkflowNodeBinding(ctx, ref, nodeID, match)
	return binding != nil, err
}

// FindWorkflowNodeBinding returns authorization evidence for one exact
// Workflow node and resource tuple.
func (r *Repository) FindWorkflowNodeBinding(
	ctx context.Context,
	ref WorkflowScopeRef,
	nodeID string,
	match Match,
) (*Binding, error) {
	if err := validateWorkflowScopeRef(ref); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || match.BindingType == "" || strings.TrimSpace(match.ResourceID) == "" {
		return nil, fmt.Errorf("workflow node id, binding type, and resource id are required")
	}
	query := r.db.WithContext(ctx).Where(
		"agent_id = ? AND consumer_type = ? AND consumer_id = ? AND binding_instance_id = ? AND binding_scope = ? AND binding_type = ? AND resource_id = ? AND parent_resource_id = ?",
		ref.AgentID,
		ConsumerTypeWorkflow,
		ref.WorkflowID,
		nodeID,
		ref.Scope,
		match.BindingType,
		strings.TrimSpace(match.ResourceID),
		strings.TrimSpace(match.ParentResourceID),
	)
	query = applyVersionScope(query, ref.PublishedVersionUUID)
	switch strings.ToLower(strings.TrimSpace(match.AccessMode)) {
	case "read":
		query = query.Where("access_mode IN ?", []string{"read", "write"})
	case "write", "execute":
		query = query.Where("access_mode = ?", strings.ToLower(strings.TrimSpace(match.AccessMode)))
	}
	var binding Binding
	if err := query.Take(&binding).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("find workflow node resource binding: %w", err)
	}
	return &binding, nil
}

// AllowsWorkflowNodeIntegrationAction enforces the persisted action allowlist
// for one exact Workflow connector node. Missing evidence denies by default.
func (r *Repository) AllowsWorkflowNodeIntegrationAction(
	ctx context.Context,
	ref WorkflowScopeRef,
	nodeID string,
	connectionID string,
	integrationID string,
	actionID string,
) (bool, error) {
	binding, err := r.FindWorkflowNodeBinding(ctx, ref, nodeID, Match{
		BindingType:      BindingTypeIntegrationConnection,
		ResourceID:       strings.TrimSpace(connectionID),
		ParentResourceID: strings.TrimSpace(integrationID),
	})
	if err != nil || binding == nil {
		return false, err
	}
	return binding.AllowsIntegrationAction(actionID), nil
}

// AllowsIntegrationAction enforces the action allowlist persisted with an
// Agent's selected Integration Connection. An empty or absent allowlist denies
// every action.
func (r *Repository) AllowsIntegrationAction(
	ctx context.Context,
	ref ScopeRef,
	connectionID string,
	integrationID string,
	actionID string,
) (bool, error) {
	binding, err := r.FindBinding(ctx, ref, Match{
		BindingType:      BindingTypeIntegrationConnection,
		ResourceID:       strings.TrimSpace(connectionID),
		ParentResourceID: strings.TrimSpace(integrationID),
	})
	if err != nil || binding == nil {
		return false, err
	}
	return binding.AllowsIntegrationAction(actionID), nil
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
	// A connection UUID is already globally unique within an organization.
	// Ignore its provider/integration parent so save, detach, and delete paths
	// contend on the same lock even when one caller only has the connection ID.
	if bindingType == BindingTypeIntegrationConnection {
		parentResourceID = ""
	}
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
		deleteQuery := current.WithContext(ctx).Where(
			"agent_id = ? AND consumer_type = ? AND binding_scope = ?",
			ref.AgentID, ConsumerTypeAgent, ref.Scope,
		)
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
			binding.ConsumerType = ConsumerTypeAgent
			binding.ConsumerID = ref.AgentID
			binding.BindingInstanceID = ""
			binding.BindingScope = ref.Scope
			binding.PublishedVersionUUID = cloneUUID(ref.PublishedVersionUUID)
			binding.ResourceID = strings.TrimSpace(binding.ResourceID)
			binding.ParentResourceID = strings.TrimSpace(binding.ParentResourceID)
			binding.DisplayName = strings.TrimSpace(binding.DisplayName)
			binding.AccessMode = strings.ToLower(strings.TrimSpace(binding.AccessMode))
			binding.Metadata = normalizeBindingMetadata(binding)
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

// ReplaceWorkflowScope atomically replaces all binding rows extracted from
// one concrete Workflow graph scope. Passing a transaction joins the caller's
// graph-write transaction; otherwise this method creates its own transaction.
func (r *Repository) ReplaceWorkflowScope(
	ctx context.Context,
	tx *gorm.DB,
	ref WorkflowScopeRef,
	bindings []Binding,
) error {
	if err := validateWorkflowScopeRef(ref); err != nil {
		return err
	}
	rows, err := prepareWorkflowBindings(ref, bindings, time.Now())
	if err != nil {
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
		oldQuery := current.WithContext(ctx).Where(
			"agent_id = ? AND consumer_type = ? AND consumer_id = ? AND binding_scope = ?",
			ref.AgentID, ConsumerTypeWorkflow, ref.WorkflowID, ref.Scope,
		)
		oldQuery = applyVersionScope(oldQuery, ref.PublishedVersionUUID)
		var oldBindings []Binding
		if err := oldQuery.Find(&oldBindings).Error; err != nil {
			return fmt.Errorf("list prior workflow resource bindings: %w", err)
		}
		if err := r.lockBindingRows(ctx, current, oldBindings, rows); err != nil {
			return err
		}

		deleteQuery := current.WithContext(ctx).Where(
			"agent_id = ? AND consumer_type = ? AND consumer_id = ? AND binding_scope = ?",
			ref.AgentID, ConsumerTypeWorkflow, ref.WorkflowID, ref.Scope,
		)
		deleteQuery = applyVersionScope(deleteQuery, ref.PublishedVersionUUID)
		if err := deleteQuery.Delete(&Binding{}).Error; err != nil {
			return fmt.Errorf("clear workflow resource binding scope: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := current.WithContext(ctx).Create(&rows).Error; err != nil {
			return fmt.Errorf("create workflow resource bindings: %w", err)
		}
		return nil
	}
	if tx != nil {
		return replace(db)
	}
	return db.WithContext(ctx).Transaction(replace)
}

func prepareWorkflowBindings(ref WorkflowScopeRef, bindings []Binding, now time.Time) ([]Binding, error) {
	rows := make([]Binding, 0, len(bindings))
	for _, binding := range bindings {
		nodeID := strings.TrimSpace(binding.BindingInstanceID)
		if nodeID == "" {
			return nil, fmt.Errorf("workflow resource binding node id is required")
		}
		if binding.OrganizationID == uuid.Nil || binding.WorkspaceID == uuid.Nil || binding.BindingType == "" || strings.TrimSpace(binding.ResourceID) == "" {
			return nil, fmt.Errorf("workflow resource binding organization, workspace, type, and resource id are required")
		}
		binding.ID = uuid.Nil
		binding.AgentID = ref.AgentID
		binding.ConsumerType = ConsumerTypeWorkflow
		binding.ConsumerID = ref.WorkflowID
		binding.BindingInstanceID = nodeID
		binding.BindingScope = ref.Scope
		binding.PublishedVersionUUID = cloneUUID(ref.PublishedVersionUUID)
		binding.ResourceID = strings.TrimSpace(binding.ResourceID)
		binding.ParentResourceID = strings.TrimSpace(binding.ParentResourceID)
		binding.DisplayName = strings.TrimSpace(binding.DisplayName)
		binding.AccessMode = strings.ToLower(strings.TrimSpace(binding.AccessMode))
		binding.Metadata = normalizeBindingMetadata(binding)
		binding.CreatedAt = now
		binding.UpdatedAt = now
		rows = append(rows, binding)
	}
	return rows, nil
}

func (r *Repository) lockBindingRows(ctx context.Context, tx *gorm.DB, groups ...[]Binding) error {
	refs := make([]ResourceRef, 0)
	for _, bindings := range groups {
		for _, binding := range bindings {
			refs = append(refs, ResourceRef{
				OrganizationID:   binding.OrganizationID,
				WorkspaceID:      &binding.WorkspaceID,
				BindingType:      binding.BindingType,
				ResourceID:       binding.ResourceID,
				ParentResourceID: binding.ParentResourceID,
			})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	if err := r.LockResources(ctx, tx, refs); err != nil {
		return fmt.Errorf("lock workflow binding resources: %w", err)
	}
	return nil
}

func normalizeBindingMetadata(binding Binding) map[string]interface{} {
	metadata := make(map[string]interface{}, len(binding.Metadata)+1)
	for key, value := range binding.Metadata {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = value
	}
	if binding.BindingType == BindingTypeIntegrationConnection {
		metadata[IntegrationAllowedActionIDsMetadataKey] = IntegrationAllowedActionIDs(binding)
	}
	return metadata
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
			Where("agent_id = ? AND consumer_type = ? AND binding_scope = ?", ref.AgentID, ConsumerTypeAgent, ScopePublished).
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

// ReplaceWorkflowPublishedHead replaces the exact immutable published
// Workflow/version scope. Despite its historical name, it intentionally keeps
// bindings for older published versions because those versions remain directly
// executable by version UUID. Explicit resource revocation is responsible for
// invalidating bindings across every version.
func (r *Repository) ReplaceWorkflowPublishedHead(
	ctx context.Context,
	tx *gorm.DB,
	ref WorkflowScopeRef,
	bindings []Binding,
) error {
	if err := validateWorkflowScopeRef(ref); err != nil {
		return err
	}
	if ref.Scope != ScopePublished {
		return fmt.Errorf("workflow published head replacement requires published scope")
	}
	return r.ReplaceWorkflowScope(ctx, tx, ref, bindings)
}

// RevokeAgentConsumers removes every Agent and Workflow consumer row owned by
// one agents.id. It takes locks in the global resource -> Agent order before
// the final delete. A concurrent save that wins the Agent lock commits first
// and is included by DELETE; a save that loses observes the caller's subsequent
// Agent deletion and aborts. Call this before taking a conflicting DB row lock.
func (r *Repository) RevokeAgentConsumers(ctx context.Context, tx *gorm.DB, agentID uuid.UUID) error {
	if agentID == uuid.Nil {
		return fmt.Errorf("agent id is required")
	}
	db := r.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return fmt.Errorf("agent bindings database is required")
	}
	revoke := func(current *gorm.DB) error {
		var bindings []Binding
		if err := current.WithContext(ctx).Where("agent_id = ?", agentID).Find(&bindings).Error; err != nil {
			return fmt.Errorf("list Agent-owned resource bindings: %w", err)
		}
		if err := r.lockBindingRows(ctx, current, bindings); err != nil {
			return err
		}
		if err := r.LockAgents(ctx, current, []uuid.UUID{agentID}); err != nil {
			return fmt.Errorf("lock Agent before revoking owned resource bindings: %w", err)
		}
		if err := current.WithContext(ctx).Where("agent_id = ?", agentID).Delete(&Binding{}).Error; err != nil {
			return fmt.Errorf("revoke Agent-owned resource bindings: %w", err)
		}
		return nil
	}
	if tx != nil {
		return revoke(db)
	}
	return db.WithContext(ctx).Transaction(revoke)
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
	if ref.ConsumerType != "" {
		if !validConsumerType(ref.ConsumerType) {
			return nil, fmt.Errorf("invalid resource binding consumer type %q", ref.ConsumerType)
		}
		query = query.Where("consumer_type = ?", ref.ConsumerType)
	}
	if ref.ConsumerID != nil {
		if *ref.ConsumerID == uuid.Nil {
			return nil, fmt.Errorf("resource binding consumer id cannot be nil")
		}
		query = query.Where("consumer_id = ?", *ref.ConsumerID)
	}
	if instanceID := strings.TrimSpace(ref.BindingInstanceID); instanceID != "" {
		query = query.Where("binding_instance_id = ?", instanceID)
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

// ListWorkflowConnectionImpact returns node- and version-level Workflow
// references for a connection. Draft and every still-runnable published
// version remain separate records so callers can explain the exact impact.
func (r *Repository) ListWorkflowConnectionImpact(
	ctx context.Context,
	organizationID uuid.UUID,
	connectionID string,
) ([]WorkflowConnectionReference, error) {
	bindings, err := r.ListImpact(ctx, ResourceRef{
		OrganizationID: organizationID,
		ConsumerType:   ConsumerTypeWorkflow,
		BindingType:    BindingTypeIntegrationConnection,
		ResourceID:     strings.TrimSpace(connectionID),
	})
	if err != nil {
		return nil, err
	}
	references := make([]WorkflowConnectionReference, 0, len(bindings))
	for _, binding := range bindings {
		if reference, ok := binding.AsWorkflowConnectionReference(); ok {
			references = append(references, reference)
		}
	}
	sort.Slice(references, func(i, j int) bool {
		left := strings.Join([]string{
			references[i].WorkspaceID.String(), references[i].AgentID.String(),
			string(references[i].Scope), references[i].WorkflowID.String(), references[i].NodeID,
		}, "|")
		right := strings.Join([]string{
			references[j].WorkspaceID.String(), references[j].AgentID.String(),
			string(references[j].Scope), references[j].WorkflowID.String(), references[j].NodeID,
		}, "|")
		return left < right
	})
	return references, nil
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
	if ref.ConsumerType != "" {
		if !validConsumerType(ref.ConsumerType) {
			return fmt.Errorf("invalid resource binding consumer type %q", ref.ConsumerType)
		}
		query = query.Where("consumer_type = ?", ref.ConsumerType)
	}
	if ref.ConsumerID != nil {
		if *ref.ConsumerID == uuid.Nil {
			return fmt.Errorf("resource binding consumer id cannot be nil")
		}
		query = query.Where("consumer_id = ?", *ref.ConsumerID)
	}
	if instanceID := strings.TrimSpace(ref.BindingInstanceID); instanceID != "" {
		query = query.Where("binding_instance_id = ?", instanceID)
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

func validateWorkflowScopeRef(ref WorkflowScopeRef) error {
	if ref.AgentID == uuid.Nil {
		return fmt.Errorf("workflow owning agent id is required")
	}
	if ref.WorkflowID == uuid.Nil {
		return fmt.Errorf("workflow id is required")
	}
	return validateScopeVersion(ref.Scope, ref.PublishedVersionUUID, "workflow")
}

func validateScopeVersion(scope Scope, versionUUID *uuid.UUID, consumer string) error {
	switch scope {
	case ScopeDraft:
		if versionUUID != nil {
			return fmt.Errorf("%s draft binding scope cannot have a published version", consumer)
		}
	case ScopePublished:
		if versionUUID == nil || *versionUUID == uuid.Nil {
			return fmt.Errorf("%s published binding scope requires a version", consumer)
		}
	default:
		return fmt.Errorf("invalid %s binding scope %q", consumer, scope)
	}
	return nil
}

func validConsumerType(value ConsumerType) bool {
	return value == ConsumerTypeAgent || value == ConsumerTypeWorkflow
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
