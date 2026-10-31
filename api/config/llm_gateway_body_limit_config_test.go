package config

import "testing"

func TestLoadLLMConfigKeepsRawGatewayRequestBodyLimit(t *testing.T) {
	cases := []struct {
		name  string
		value string
		set   bool
		want  string
	}{
		{name: "unset", want: ""},
		{name: "trimmed", value: " 256 ", set: true, want: "256"},
		{name: "invalid kept for handler fallback", value: "abc", set: true, want: "abc"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if key == envLLMGatewayRequestBodyLimitMB && test.set {
					return test.value, true
				}
				return "", false
			}}
			cfg := &Config{}
			loadLLMConfig(cfg, source)
			if cfg.LLM.GatewayRequestBodyLimitMB != test.want {
				t.Fatalf("GatewayRequestBodyLimitMB = %q, want %q", cfg.LLM.GatewayRequestBodyLimitMB, test.want)
			}
		})
	}
}
