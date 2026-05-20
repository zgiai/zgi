package common

// InvokeFrom is the shared source enum for app-module runtime invocation semantics.
//
// Keep cross-cutting app runtime values here when:
// - the values are used by multiple app subdomains such as workflow, conversation, or external routes
// - the values need one canonical definition to avoid drift and string-based redefinition
// - moving them into a single feature package would create import cycles
//
// This file is also the preferred home for future app-scoped legacy semantic cleanup constants
// that need to be shared across multiple subpackages during incremental refactors.
type InvokeFrom string

const (
	InvokeFromDebugger    InvokeFrom = "debugger"
	InvokeFromExternalAPI InvokeFrom = "external-api"
	InvokeFromWebApp      InvokeFrom = "web-app"
	InvokeFromWorkflow    InvokeFrom = "workflow"
	InvokeFromAutomation  InvokeFrom = "automation"
)
