package workflow

import "testing"

func TestRunTokensCountEveryNodeThatReportsUsage(t *testing.T) {
	log := func(outputs string) WorkflowNodeRuntimeLog { return WorkflowNodeRuntimeLog{Outputs: &outputs} }
	for name, tc := range map[string]struct {
		outputs string
		want    int64
	}{
		"llm node":            {outputs: `{"text":"ok","usage":{"TotalTokens":255}}`, want: 255},
		"sql generation":      {outputs: `{"sql":"SELECT 1","usage":{"prompt_tokens":145,"completion_tokens":351,"total_tokens":496}}`, want: 496},
		"parameter extractor": {outputs: `{"name":"x","__usage":{"total_tokens":88}}`, want: 88},
		"no model call":       {outputs: `{"rows":[]}`, want: 0},
	} {
		if got := workflowNodeRuntimeLogTotalTokens(log(tc.outputs)); got != tc.want {
			t.Errorf("%s: tokens = %d, want %d", name, got, tc.want)
		}
	}
}
