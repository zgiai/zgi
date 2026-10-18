package executionruntime

import (
	"encoding/json"
	"testing"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAgentExecutionShellReportsWorkerJobDuration(t *testing.T) {
	start := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	end := start.Add(2500 * time.Millisecond)
	before := start.Add(-time.Second)
	zero := time.Time{}
	for _, tc := range []struct {
		name       string
		start, end *time.Time
		want       int64
		present    bool
	}{
		{"measured", &start, &end, 2500, true},
		{"sub-millisecond", &start, &start, 0, true},
		{"old-worker", nil, nil, 0, false},
		{"missing-finish", &start, nil, 0, false},
		{"invalid-order", &start, &before, 0, false},
		{"zero-start", &zero, &end, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := 0
			job := agentExecutionJob{ID: "job-duration", Status: "succeeded", ExitCode: &code, StartedAt: tc.start, FinishedAt: tc.end}
			// Exercise the same JSON boundary as a Worker HTTP response.
			encoded, err := json.Marshal(job)
			if err != nil {
				t.Fatal(err)
			}
			var decoded agentExecutionJob
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			gateway := &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding-duration", Status: "active", Generation: 1}, job: decoded}
			tool := newAgentExecutionShellTool(gateway, agentExecutionScope{OrganizationID: "org", SubjectID: "user"}, "agent", "conversation", "message", "standard", "local-gvisor", nil, nil)
			result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-duration", Function: adapter.FunctionCall{Arguments: `{"command":"true"}`}})
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			for _, output := range []map[string]interface{}{result.Result, result.ModelResult} {
				actual, ok := output["execution_duration_ms"]
				if ok != tc.present || (ok && actual != tc.want) {
					t.Fatalf("execution duration=%v present=%v, want %d present=%v", actual, ok, tc.want, tc.present)
				}
			}
		})
	}
}
