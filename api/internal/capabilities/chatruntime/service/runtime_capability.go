package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// BoundResourceGrant captures the account and time that authorized a bound runtime resource.
type BoundResourceGrant struct {
	BoundByAccountID string
	BoundAtUnix      int64
	GrantEnabled     bool
}

// RuntimeCapabilityConfig groups runtime-managed capabilities derived from RunConfig.
type RuntimeCapabilityConfig struct {
	AgentID   string
	Knowledge RuntimeKnowledgeCapability
	Database  RuntimeDatabaseCapability
	Workflow  RuntimeWorkflowCapability
	Memory    RuntimeMemoryCapability
}

type RuntimeKnowledgeCapability struct {
	DatasetIDs      []string
	Grant           BoundResourceGrant
	RetrievalConfig map[string]interface{}
}

type RuntimeDatabaseCapability struct {
	Bindings []AgentDatabaseBinding
	Grant    BoundResourceGrant
}

type RuntimeWorkflowCapability struct {
	Bindings []AgentWorkflowBinding
	Grant    BoundResourceGrant
}

type RuntimeMemoryCapability struct {
	AgentEnabled bool
	AgentSlots   []AgentMemorySlotConfig
	UserScope    string
}

func runtimeCapabilityConfigFromRunConfig(config RunConfig) RuntimeCapabilityConfig {
	return RuntimeCapabilityConfig{
		AgentID: strings.TrimSpace(config.BillingAppID),
		Knowledge: RuntimeKnowledgeCapability{
			DatasetIDs:      append([]string(nil), config.KnowledgeDatasetIDs...),
			Grant:           NewBoundResourceGrant(config.KnowledgeBoundByAccountID, config.KnowledgeBoundAtUnix),
			RetrievalConfig: copyStringAnyMap(config.KnowledgeRetrievalConfig),
		},
		Database: RuntimeDatabaseCapability{
			Bindings: copyAgentDatabaseBindings(config.DatabaseBindings),
			Grant:    NewBoundResourceGrant(config.DatabaseBoundByAccountID, config.DatabaseBoundAtUnix),
		},
		Workflow: RuntimeWorkflowCapability{
			Bindings: copyAgentWorkflowBindings(config.WorkflowBindings),
			Grant:    NewBoundResourceGrant(config.WorkflowBoundByAccountID, config.WorkflowBoundAtUnix),
		},
		Memory: RuntimeMemoryCapability{
			AgentEnabled: config.AgentMemoryEnabled,
			AgentSlots:   enabledAgentMemorySlots(config.AgentMemorySlots),
			UserScope:    strings.TrimSpace(config.AgentMemoryUserScope),
		},
	}
}

func (c RuntimeCapabilityConfig) RuntimeParameters(scope Scope, billingAppType string) map[string]interface{} {
	params := map[string]interface{}{
		"organization_id": scope.OrganizationID.String(),
	}
	if scope.WorkspaceID != nil {
		params["workspace_id"] = scope.WorkspaceID.String()
	}
	c.Knowledge.applyRuntimeParameters(params)
	c.Database.applyRuntimeParameters(params)
	c.Workflow.applyRuntimeParameters(params)
	c.Memory.applyRuntimeParameters(params)
	if strings.EqualFold(strings.TrimSpace(billingAppType), runtimemodel.ConversationCallerAgent) && strings.TrimSpace(c.AgentID) != "" {
		params["agent_id"] = strings.TrimSpace(c.AgentID)
	}
	return params
}

func (c RuntimeKnowledgeCapability) applyRuntimeParameters(params map[string]interface{}) {
	if len(c.DatasetIDs) > 0 {
		params["knowledge_dataset_ids"] = append([]string(nil), c.DatasetIDs...)
	}
	c.Grant.applyRuntimeParameters(params, "knowledge")
	if len(c.RetrievalConfig) > 0 {
		params["knowledge_retrieval_config"] = copyStringAnyMap(c.RetrievalConfig)
	}
}

func (c RuntimeDatabaseCapability) applyRuntimeParameters(params map[string]interface{}) {
	if len(c.Bindings) > 0 {
		params["database_bindings"] = copyAgentDatabaseBindings(c.Bindings)
	}
	c.Grant.applyRuntimeParameters(params, "database")
}

func (c RuntimeWorkflowCapability) applyRuntimeParameters(params map[string]interface{}) {
	if len(c.Bindings) > 0 {
		params["workflow_bindings"] = copyAgentWorkflowBindings(c.Bindings)
	}
	c.Grant.applyRuntimeParameters(params, "workflow")
}

func (c RuntimeMemoryCapability) applyRuntimeParameters(params map[string]interface{}) {
	if !c.AgentEnabled {
		return
	}
	params["agent_memory_enabled"] = true
	params["agent_memory_slots"] = append([]AgentMemorySlotConfig(nil), c.AgentSlots...)
	if c.UserScope != "" {
		params["user_scope"] = c.UserScope
	}
}

func (g BoundResourceGrant) applyRuntimeParameters(params map[string]interface{}, prefix string) {
	if strings.TrimSpace(g.BoundByAccountID) != "" {
		params[prefix+"_bound_by_account_id"] = strings.TrimSpace(g.BoundByAccountID)
	}
	if g.BoundAtUnix > 0 {
		params[prefix+"_binding_grant"] = g.GrantEnabled
		params[prefix+"_bound_at_unix"] = g.BoundAtUnix
	}
}

// NewBoundResourceGrant normalizes persisted binding grant fields.
func NewBoundResourceGrant(boundByAccountID string, boundAtUnix int64) BoundResourceGrant {
	boundByAccountID = strings.TrimSpace(boundByAccountID)
	return BoundResourceGrant{
		BoundByAccountID: boundByAccountID,
		BoundAtUnix:      boundAtUnix,
		GrantEnabled:     boundByAccountID != "" && boundAtUnix > 0,
	}
}

// ResolveBoundResourceGrant returns the grant to persist for a resource binding.
func ResolveBoundResourceGrant(previous BoundResourceGrant, currentConfigured bool, unchanged bool, actorAccountID string, nowUnix int64) BoundResourceGrant {
	if !currentConfigured {
		return BoundResourceGrant{}
	}
	if unchanged && previous.GrantEnabled {
		return previous
	}
	actorAccountID = strings.TrimSpace(actorAccountID)
	if actorAccountID == "" {
		return previous
	}
	return BoundResourceGrant{
		BoundByAccountID: actorAccountID,
		BoundAtUnix:      nowUnix,
		GrantEnabled:     nowUnix > 0,
	}
}
