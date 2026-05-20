package llm

import "testing"

func TestConvertToModelConfig_TrimsProviderAndModel(t *testing.T) {
	node := &Node{}

	cfg, err := node.convertToModelConfig(map[string]any{
		"provider": " siliconflow ",
		"model":    " ByteDance-Seed/Seed-OSS-36B-Instruct ",
	})
	if err != nil {
		t.Fatalf("convertToModelConfig() error = %v", err)
	}
	if cfg.Provider != "siliconflow" {
		t.Fatalf("cfg.Provider = %q, want %q", cfg.Provider, "siliconflow")
	}
	if cfg.Name != "ByteDance-Seed/Seed-OSS-36B-Instruct" {
		t.Fatalf("cfg.Name = %q, want %q", cfg.Name, "ByteDance-Seed/Seed-OSS-36B-Instruct")
	}
}
