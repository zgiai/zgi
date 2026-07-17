package service

import "testing"

func TestPlanningOutputTokenLimit(t *testing.T) {
	tests := []struct {
		name    string
		control map[string]interface{}
		want    int
	}{
		{
			name: "uses reserved output for large context models",
			control: map[string]interface{}{
				"reserved_output_tokens":  8192,
				"model_max_output_tokens": 131072,
				"safe_context_limit":      235929,
				"estimated_prompt_tokens": 4096,
			},
			want: 8192,
		},
		{
			name: "clamps reserved output to model limit",
			control: map[string]interface{}{
				"reserved_output_tokens":  65536,
				"model_max_output_tokens": 32768,
				"safe_context_limit":      120000,
				"estimated_prompt_tokens": 2000,
			},
			want: 32768,
		},
		{
			name: "clamps reserved output to available context",
			control: map[string]interface{}{
				"reserved_output_tokens":  8192,
				"model_max_output_tokens": 65536,
				"safe_context_limit":      10000,
				"estimated_prompt_tokens": 4000,
			},
			want: 6000,
		},
		{
			name: "keeps legacy fallback without reserved output",
			control: map[string]interface{}{
				"model_max_output_tokens": 4096,
				"safe_context_limit":      16000,
				"estimated_prompt_tokens": 2000,
			},
			want: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := &PreparedChat{parts: &chatRequestParts{ContextControl: tt.control}}
			if got := planningOutputTokenLimit(prepared); got != tt.want {
				t.Fatalf("planningOutputTokenLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}
