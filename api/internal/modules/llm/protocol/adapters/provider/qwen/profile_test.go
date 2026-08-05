package qwen

import "testing"

func TestResolveEndpointProfilePreservesRegionalHost(t *testing.T) {
	for _, host := range []string{
		"https://dashscope.aliyuncs.com",
		"https://dashscope-us.aliyuncs.com",
		"https://workspace.ap-southeast-1.maas.aliyuncs.com",
	} {
		t.Run(host, func(t *testing.T) {
			profile := ResolveEndpointProfile(host + "/api/v1/")
			if profile.NativeBaseURL != host+"/api/v1" {
				t.Fatalf("native URL = %q", profile.NativeBaseURL)
			}
			if profile.OpenAICompatibleBaseURL != host+"/compatible-mode/v1" {
				t.Fatalf("compatible URL = %q", profile.OpenAICompatibleBaseURL)
			}
			if profile.AnthropicMessagesBaseURL != host+"/apps/anthropic/v1" {
				t.Fatalf("anthropic URL = %q", profile.AnthropicMessagesBaseURL)
			}
			if profile.CompatibleRerankBaseURL != host+"/compatible-api/v1" {
				t.Fatalf("rerank URL = %q", profile.CompatibleRerankBaseURL)
			}
		})
	}
}

func TestResolveEndpointProfileKeepsUnrecognizedCustomPath(t *testing.T) {
	const custom = "https://gateway.example.com/qwen"
	profile := ResolveEndpointProfile(custom)
	if profile.NativeBaseURL != custom || profile.OpenAICompatibleBaseURL != custom {
		t.Fatalf("custom profile = %#v, want unchanged URLs", profile)
	}
}
