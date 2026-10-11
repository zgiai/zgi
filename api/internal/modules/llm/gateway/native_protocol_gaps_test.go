package gateway

import (
	"testing"

	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

// TestKnownProviderModelProtocolUnsupportedMirrorsConsole pins the two
// provider/model/protocol counterexamples Console withholds from its published
// catalog. Pre must not re-advertise them through its own discovery or routing.
func TestKnownProviderModelProtocolUnsupportedMirrorsConsole(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		protocol string
		want     bool
	}{
		{"agicto", "claude-sonnet-4-6", protocolOpenAIResponses, true},
		{"Agicto 供应商", "CLAUDE-HAIKU-4-5", protocolOpenAIResponses, false}, // display name is not the routing provider key
		{"agicto", "gpt-4o", protocolOpenAIResponses, false},
		{"agicto", "claude-sonnet-4-6", protocolAnthropicMessages, false},
		{"deepseek", "deepseek-v4-pro", protocolAnthropicMessages, true},
		{"deepseek", "deepseek-v4-flash", protocolAnthropicMessages, true},
		{"deepseek", "deepseek-chat", protocolAnthropicMessages, false},
		{"deepseek", "deepseek-v4-pro", protocolOpenAIResponses, false},
	}
	for _, testCase := range cases {
		if got := knownProviderModelProtocolUnsupported(testCase.provider, testCase.model, testCase.protocol); got != testCase.want {
			t.Fatalf("knownProviderModelProtocolUnsupported(%q, %q, %q) = %v, want %v",
				testCase.provider, testCase.model, testCase.protocol, got, testCase.want)
		}
	}
}

// TestRouteSupportWithholdsTheKnownGaps is the offline acceptance check: a
// provider whose capability default is optimistic must still refuse the
// counterexample combination, while every other model on the same provider
// keeps being advertised.
func TestRouteSupportWithholdsTheKnownGaps(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		model     string
		route     *channelmodel.LLMRoute
		responses bool
		messages  bool
	}{
		{
			// Console withholds only `responses` for agicto+claude; its
			// Anthropic Messages capability is unaffected.
			name:      "agicto claude is not advertised for responses",
			provider:  "agicto",
			model:     "claude-sonnet-4-6",
			route:     &channelmodel.LLMRoute{ChannelProvider: "agicto"},
			responses: false,
			messages:  true,
		},
		{
			name:      "agicto gpt keeps responses",
			provider:  "agicto",
			model:     "gpt-4o",
			route:     &channelmodel.LLMRoute{ChannelProvider: "agicto"},
			responses: true,
			messages:  true,
		},
		{
			name:      "deepseek v4 is not advertised for anthropic messages",
			provider:  "deepseek",
			model:     "deepseek-v4-pro",
			route:     &channelmodel.LLMRoute{ChannelProvider: "deepseek"},
			messages:  false,
			responses: false,
		},
		{
			name:      "deepseek chat keeps anthropic messages",
			provider:  "deepseek",
			model:     "deepseek-chat",
			route:     &channelmodel.LLMRoute{ChannelProvider: "deepseek"},
			messages:  true,
			responses: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := routeSupportsOpenAIResponses(testCase.route, testCase.model); got != testCase.responses {
				t.Fatalf("routeSupportsOpenAIResponses(%s) = %v, want %v", testCase.model, got, testCase.responses)
			}
			if got := routeSupportsAnthropicMessages(testCase.route, testCase.model); got != testCase.messages {
				t.Fatalf("routeSupportsAnthropicMessages(%s) = %v, want %v", testCase.model, got, testCase.messages)
			}
		})
	}
}

// TestExplicitModelDeclarationStaysAuthoritative guards the precedence: an
// explicit per-model declaration for the counterexample combination keeps
// winning, so a provider that really does serve it can still be configured.
func TestExplicitModelDeclarationStaysAuthoritative(t *testing.T) {
	route := &channelmodel.LLMRoute{
		ChannelProvider: "agicto",
		NativeProtocols: channelmodel.NativeProtocolConfig{
			ModelProtocols: map[string][]string{
				"claude-sonnet-4-6": {protocolOpenAIResponses},
			},
		},
	}
	if !routeSupportsOpenAIResponses(route, "claude-sonnet-4-6") {
		t.Fatal("an explicit per-model declaration must remain authoritative")
	}
}
