package digitalhuman

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestOnlyATurnStoppedWithItsProcessAndNotTriedIsResumed(t *testing.T) {
	expired, other := staleRuntimeLeaseError, "model_invocation_failed"
	for name, tt := range map[string]struct {
		message *runtimemodel.Message
		want    bool
	}{
		"stopped with its process": {&runtimemodel.Message{Status: runtimemodel.MessageStatusError, Error: &expired}, true},
		"tried before": {&runtimemodel.Message{Status: runtimemodel.MessageStatusError, Error: &expired,
			Metadata: map[string]interface{}{"interrupted_turn_resume": map[string]interface{}{"status": "not_resumed"}}}, false},
		"failed for another reason": {&runtimemodel.Message{Status: runtimemodel.MessageStatusError, Error: &other}, false},
		"still streaming":           {&runtimemodel.Message{Status: runtimemodel.MessageStatusStreaming}, false},
		"missing":                   {nil, false},
	} {
		if got := interruptedTurnMessage(tt.message); got != tt.want {
			t.Fatalf("%s: interruptedTurnMessage() = %v, want %v", name, got, tt.want)
		}
	}
}
