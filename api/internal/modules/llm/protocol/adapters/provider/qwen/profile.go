package qwen

import "strings"

// EndpointProfile resolves the protocol endpoints exposed by one Model Studio
// region. The host is preserved, so the same rules support China,
// international, and private gateway base URLs.
type EndpointProfile struct {
	NativeBaseURL            string
	OpenAICompatibleBaseURL  string
	AnthropicMessagesBaseURL string
	CompatibleRerankBaseURL  string
}

// ResolveEndpointProfile normalizes any supported Qwen base URL once, instead
// of scattering path rewrites across individual capabilities.
func ResolveEndpointProfile(raw string) EndpointProfile {
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	return EndpointProfile{
		NativeBaseURL:            replaceQwenProtocolPath(baseURL, "/api/v1"),
		OpenAICompatibleBaseURL:  replaceQwenProtocolPath(baseURL, "/compatible-mode/v1"),
		AnthropicMessagesBaseURL: replaceQwenProtocolPath(baseURL, "/apps/anthropic/v1"),
		CompatibleRerankBaseURL:  replaceQwenProtocolPath(baseURL, "/compatible-api/v1"),
	}
}

func replaceQwenProtocolPath(baseURL, target string) string {
	for _, current := range []string{
		"/compatible-mode/v1",
		"/compatible-api/v1",
		"/apps/anthropic/v1",
		"/api/v1",
	} {
		if strings.Contains(baseURL, current) {
			return strings.Replace(baseURL, current, target, 1)
		}
	}
	return baseURL
}
