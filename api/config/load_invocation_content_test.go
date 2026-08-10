package config

import "testing"

func TestLoadLLMInvocationContentConfigBoundsOperationalTuning(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		values := map[string]string{
			envLLMInvocationContentMaxBytes:      "32768",
			envLLMInvocationContentRetentionDays: "90",
			envLLMInvocationContentQueueSize:     "20",
			envLLMInvocationContentBatchSize:     "10",
		}
		value, ok := values[key]
		return value, ok
	}}

	loadLLMInvocationContentConfig(cfg, source)
	if cfg.LLMInvocationContent.MaxBytes != 32768 || cfg.LLMInvocationContent.RetentionDays != 14 || cfg.LLMInvocationContent.QueueSize != 20 || cfg.LLMInvocationContent.BatchSize != 10 {
		t.Fatalf("unexpected invocation content config: %#v", cfg.LLMInvocationContent)
	}
}
