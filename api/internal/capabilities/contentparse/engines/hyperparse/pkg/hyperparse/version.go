package hyperparse

// ModulePath matches the module path used by the embedded runtime package.
const ModulePath = "github.com/zgiai/hyperparse"

// Version is the semantic version of the embedded runtime facade.
const Version = "0.1.5"

// SDKVersion returns Version for logs and diagnostics.
func SDKVersion() string { return Version }
