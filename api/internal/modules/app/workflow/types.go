package workflow

import "github.com/zgiai/zgi/api/internal/modules/app/common"

type WorkflowType string

const (
	WorkflowTypeWorkflow WorkflowType = "workflow"
	WorkflowTypeChat     WorkflowType = "chat"
)

type UserFrom string

const (
	UserFromAccount UserFrom = "account"
	UserFromEndUser UserFrom = "end_user"
)

// InvokeFrom re-exports the app-shared invoke_from enum for workflow code.
// Canonical values are defined in internal/modules/app/common to avoid drift across
// workflow, graph engine, conversation, and external route packages.
type InvokeFrom = common.InvokeFrom

const (
	InvokeFromDebugger    InvokeFrom = common.InvokeFromDebugger
	InvokeFromExternalAPI InvokeFrom = common.InvokeFromExternalAPI
	InvokeFromServiceAPI  InvokeFrom = common.InvokeFromExternalAPI
	InvokeFromWebApp      InvokeFrom = common.InvokeFromWebApp
	InvokeFromWorkflow    InvokeFrom = common.InvokeFromWorkflow
	InvokeFromAutomation  InvokeFrom = common.InvokeFromAutomation
)

type WorkflowNodeExecutionStatus string

const (
	WorkflowNodeExecutionStatusRunning   WorkflowNodeExecutionStatus = "running"
	WorkflowNodeExecutionStatusSucceeded WorkflowNodeExecutionStatus = "succeeded"
	WorkflowNodeExecutionStatusFailed    WorkflowNodeExecutionStatus = "failed"
	WorkflowNodeExecutionStatusException WorkflowNodeExecutionStatus = "exception"
)

type WorkflowNodeExecutionTriggeredFrom string

const (
	WorkflowNodeExecutionTriggeredFromSingleStep WorkflowNodeExecutionTriggeredFrom = "single_step"
	WorkflowNodeExecutionTriggeredFromWorkflow   WorkflowNodeExecutionTriggeredFrom = "workflow"
)
