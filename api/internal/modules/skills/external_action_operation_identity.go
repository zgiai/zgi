package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const externalActionOperationItemIDRuntimeParameter = "_server_external_action_operation_item_id"

// externalActionOperationIdentity is deliberately unexported. Runtime values
// reconstructed from JSON or supplied as ordinary strings/maps cannot satisfy
// this type assertion, even if they guess the private parameter key and format.
type externalActionOperationIdentity struct{ value string }

// ProjectedExternalActionOperationItemID derives a stable, server-owned receipt
// identity for one concrete projected Action phase. Every input is taken from
// the canonical runtime ledger or a fixed projection binding, never from the
// model-visible business argument envelope.
func ProjectedExternalActionOperationItemID(
	phaseID string,
	ledgerEpoch string,
	bindingFingerprint string,
	integrationID string,
	actionID string,
	connectionID string,
) string {
	components := []string{
		strings.TrimSpace(phaseID),
		strings.TrimSpace(ledgerEpoch),
		strings.TrimSpace(bindingFingerprint),
		strings.ToLower(strings.TrimSpace(integrationID)),
		strings.ToLower(strings.TrimSpace(actionID)),
		strings.ToLower(strings.TrimSpace(connectionID)),
	}
	for _, component := range components {
		if component == "" {
			return ""
		}
	}
	payload := strings.Join(append([]string{"projected-external-action-phase-v1"}, components...), "\x00")
	digest := sha256.Sum256([]byte(payload))
	return "phase:" + hex.EncodeToString(digest[:])
}

// WithExternalActionOperationItemID stores the server-derived phase identity
// in request-scoped runtime parameters. It is intentionally unavailable in the
// tool schema and therefore cannot be supplied through model arguments.
func WithExternalActionOperationItemID(parameters map[string]interface{}, operationItemID string) map[string]interface{} {
	next := copyStringAnyMap(parameters)
	if next == nil {
		next = map[string]interface{}{}
	}
	delete(next, externalActionOperationItemIDRuntimeParameter)
	if operationItemID = normalizeExternalActionOperationItemID(operationItemID); operationItemID != "" {
		next[externalActionOperationItemIDRuntimeParameter] = externalActionOperationIdentity{value: operationItemID}
	}
	return next
}

// ExternalActionOperationItemIDFromRuntimeParameters returns only identities
// produced in the canonical phase format.
func ExternalActionOperationItemIDFromRuntimeParameters(parameters map[string]interface{}) string {
	if parameters == nil {
		return ""
	}
	identity, _ := parameters[externalActionOperationItemIDRuntimeParameter].(externalActionOperationIdentity)
	return normalizeExternalActionOperationItemID(identity.value)
}

func normalizeExternalActionOperationItemID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != len("phase:")+sha256.Size*2 || !strings.HasPrefix(value, "phase:") {
		return ""
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "phase:")); err != nil {
		return ""
	}
	return value
}
