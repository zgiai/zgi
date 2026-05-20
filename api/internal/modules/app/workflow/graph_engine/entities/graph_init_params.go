package entities

import "github.com/zgiai/zgi/api/internal/modules/app/common"

// WorkflowType represents workflow type enum (copied from workflow package to avoid circular import)
type WorkflowType string

const (
	WorkflowTypeWorkflow WorkflowType = "workflow"
	WorkflowTypeChat     WorkflowType = "chat"
)

// UserFrom represents user source enum (copied from workflow package to avoid circular import)
type UserFrom string

const (
	UserFromAccount UserFrom = "account"
	UserFromEndUser UserFrom = "end_user"
)

// InvokeFrom re-exports the app-shared invoke_from enum for graph engine initialization.
// Canonical values live in internal/modules/app/common so graph-engine and workflow code
// do not need to duplicate string literals or create circular imports.
type InvokeFrom = common.InvokeFrom

const (
	InvokeFromDebugger    InvokeFrom = common.InvokeFromDebugger
	InvokeFromExternalAPI InvokeFrom = common.InvokeFromExternalAPI
	InvokeFromServiceAPI  InvokeFrom = common.InvokeFromExternalAPI
	InvokeFromWebApp      InvokeFrom = common.InvokeFromWebApp
	InvokeFromWorkflow    InvokeFrom = common.InvokeFromWorkflow
)

type GraphInitParams struct {
	TenantID           string                 `json:"tenant_id"`       // legacy name, semantically workspace id in workflow
	WorkspaceID        string                 `json:"workspace_id"`    // canonical workspace id for LLM gateway app context
	OrganizationID     string                 `json:"organization_id"` // canonical organization id for billing and routing
	BillingSubjectType string                 `json:"billing_subject_type"`
	AppID              string                 `json:"app_id"`        // app id
	WorkflowType       WorkflowType           `json:"workflow_type"` // workflow type
	WorkflowID         string                 `json:"workflow_id"`   // workflow id
	GraphConfig        map[string]interface{} `json:"graph_config"`  // graph config
	UserID             string                 `json:"user_id"`       // user id
	UserFrom           UserFrom               `json:"user_from"`     // user from, account or end-user
	InvokeFrom         InvokeFrom             `json:"invoke_from"`   // invoke from, service-api, web-app, explore or debugger
	CallDepth          int                    `json:"call_depth"`    // call depth
}
