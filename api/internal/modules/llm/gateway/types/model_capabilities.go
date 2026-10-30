package types

// ModelCapabilities is the capability declaration read from a model's metadata
// row. It is the request-time counterpart of the catalog: handlers use it to
// reject a request that asks for a capability the model never declared, instead
// of forwarding it and turning the upstream's deterministic refusal into a
// retryable 5xx.
//
// Known is deliberately separate from the individual flags. A missing row (or a
// row whose metadata cannot be read) leaves every flag false, which is
// indistinguishable from "the model really supports nothing"; callers must treat
// Known=false as "unknown" and fail open rather than reject.
type ModelCapabilities struct {
	Known            bool
	StructuredOutput bool
	JSONMode         bool
	ToolCall         bool
	Vision           bool
}
