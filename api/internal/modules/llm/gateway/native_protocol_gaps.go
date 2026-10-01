package gateway

import "strings"

// knownProviderModelProtocolUnsupported captures validated provider gaps so the
// catalog never advertises a protocol/model combination that deterministically
// fails. Console owns the same two counterexamples
// (llm/proxy/native_capabilities.go::knownProviderModelProtocolUnsupported) and
// withholds them from the published capability; Pre mirrors them so its own
// discovery and routing do not re-advertise a route Console already refuses.
//
// An explicit per-model declaration stays authoritative: callers consult this
// only after routeProtocolDeclaration reports no declaration for the model.
func knownProviderModelProtocolUnsupported(provider, model, protocol string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	// Agicto returns a 200 error envelope for Claude models on /responses, so
	// advertising it makes Codex finish with an empty turn.
	if provider == "agicto" && protocol == protocolOpenAIResponses && strings.HasPrefix(model, "claude-") {
		return true
	}
	// DeepSeek's native Anthropic endpoint does not serve the v4 model family;
	// advertising it makes discovery claim a route that returns 404.
	if provider == "deepseek" && protocol == protocolAnthropicMessages && strings.HasPrefix(model, "deepseek-v4") {
		return true
	}
	return false
}
