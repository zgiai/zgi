package execution

import (
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// readOnlyAutoInvocation reports whether a High-risk call may run as
// trusted-low under ADR 0026: the target is a DigitalHuman, its binding chose
// read_only_auto, the connector is the exact SSH Server Operations built-in,
// and the call with its exact input is proven read-only. Everything else keeps
// its classification, so a change still asks and an unattended run is still
// governed by task authorization.
func readOnlyAutoInvocation(request ExecutionRequest, binding core.TargetBinding, definition catalog.Definition) bool {
	if request.Target.Type != core.TargetTypeDigitalHuman || !binding.Constraint.ReadOnlyAutoAuthorized() ||
		!definition.IsServerOpsSSHCredentials() {
		return false
	}
	input, ok := invocationInputMap(request.Input)
	return ok && serverops.IsReadOnlyInvocation(string(request.OperationID), input)
}

// invocationInputMap returns the call input as the JSON object the provider
// receives.
func invocationInputMap(input any) (map[string]any, bool) {
	if values, ok := input.(map[string]any); ok {
		return values, true
	}
	if input == nil {
		return map[string]any{}, true
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, false
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values == nil {
		return nil, false
	}
	return values, true
}

// unattendedInvocationAllowed reports whether an unattended call may consume
// the owner's task grant at all (ADR 0027). An SSH Server Operations call must
// be proven read-only: a change always needs a person, whatever was granted.
func unattendedInvocationAllowed(request ExecutionRequest, definition catalog.Definition) bool {
	if !definition.IsServerOpsSSHCredentials() {
		return true
	}
	input, ok := invocationInputMap(request.Input)
	return ok && serverops.IsReadOnlyInvocation(string(request.OperationID), input)
}
