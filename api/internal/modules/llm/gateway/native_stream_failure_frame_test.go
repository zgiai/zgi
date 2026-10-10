package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

// TestHandleNativeStreamBillingNeverRelaysUpstreamFailureFrame covers E2E-6: an
// upstream failure frame can carry provider-internal fields (for example a
// gateway's x-ms-fe-error header echo), so it must reach the handler as an
// error to be projected, never as relayed data.
func TestHandleNativeStreamBillingNeverRelaysUpstreamFailureFrame(t *testing.T) {
	const leakedHeaders = `"headers":{"x-ms-fe-error":"true"}`
	tests := []struct {
		name    string
		format  nativeUsageBodyFormat
		prelude adapter.RawStreamEvent
		failure adapter.RawStreamEvent
	}{
		{
			name:    "responses",
			format:  nativeUsageBodyFormatResponses,
			prelude: adapter.RawStreamEvent{Event: "response.created", Data: json.RawMessage(`{"type":"response.created","response":{"id":"resp_1","model":"upstream-model","status":"in_progress"}}`)},
			failure: adapter.RawStreamEvent{Event: "error", Data: json.RawMessage(`{"type":"error","sequence_number":1,"error":{"code":"rate_limit_exceeded",` + leakedHeaders + `,"message":"Service load is too high, please try again later","param":null,"type":"too_many_requests"}}`)},
		},
		{
			name:    "anthropic",
			format:  nativeUsageBodyFormatAnthropic,
			prelude: adapter.RawStreamEvent{Event: "message_start", Data: json.RawMessage(`{"type":"message_start","message":{"id":"msg_1","model":"upstream-model","content":[]}}`)},
			failure: adapter.RawStreamEvent{Event: "error", Data: json.RawMessage(`{"type":"error","error":{"type":"overloaded_error",` + leakedHeaders + `,"message":"Service load is too high, please try again later"}}`)},
		},
		{
			name:    "gemini",
			format:  nativeUsageBodyFormatGemini,
			prelude: adapter.RawStreamEvent{Data: json.RawMessage(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"modelVersion":"upstream-model"}`)},
			failure: adapter.RawStreamEvent{Data: json.RawMessage(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED",` + leakedHeaders + `,"message":"Service load is too high, please try again later"}}`)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := &fakeBillingProvider{checkBalanceResult: true}
			s := &llmGatewayServiceImpl{
				billing:       &fakeBillingProvider{checkBalanceResult: true},
				localBilling:  local,
				healthTracker: NewChannelHealthTracker(nil),
			}
			in := make(chan adapter.RawStreamEvent, 3)
			out := make(chan adapter.RawStreamEvent, 4)
			in <- tt.prelude
			in <- tt.failure
			in <- adapter.RawStreamEvent{Done: true}
			close(in)
			bc := &BillingContext{OrganizationID: "org-1", RequestID: "req-" + tt.name, ModelName: "logical-model", ProviderName: "openai"}
			ps := &ProviderSelection{
				Model:    llmmodel.LLMModel{ID: uuid.New(), Model: "logical-model"},
				Provider: providermodel.LLMProvider{Provider: "openai"},
			}

			s.handleNativeStreamBilling(context.Background(), nil, in, out, bc, ps, nil, 0, time.Now(), "logical-model", nil, "llm.native.stream", tt.format)

			var got []adapter.RawStreamEvent
			for event := range out {
				got = append(got, event)
			}
			if len(got) != 2 {
				t.Fatalf("events = %+v, want relayed prelude then one terminal error", got)
			}
			if strings.Contains(string(got[0].Data), "upstream-model") {
				t.Fatalf("prelude leaked the upstream model: %s", got[0].Data)
			}
			last := got[1]
			if !last.Done || !errors.Is(last.Error, adapter.ErrUpstreamError) {
				t.Fatalf("terminal event = %+v, want upstream error for the handler to project", last)
			}
			if len(last.Data) != 0 {
				t.Fatalf("upstream failure frame was relayed: %s", last.Data)
			}
			for _, event := range got {
				if strings.Contains(string(event.Data), "x-ms-fe-error") || strings.Contains(string(event.Data), "Service load is too high") {
					t.Fatalf("upstream failure details reached the client stream: %s", event.Data)
				}
			}
			if local.lastSettle == nil || local.lastSettle.Status == "success" {
				t.Fatalf("local settlement = %+v, must follow failure path", local.lastSettle)
			}
		})
	}
}
