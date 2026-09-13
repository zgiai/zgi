package digitalhumantask

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestDeliveryConfigPersistsSourceConversationAndFallback(t *testing.T) {
	reply := &invocation.ReplyAddress{
		Provider: "feishu", InstallationID: uuid.NewString(), ConversationID: "oc_source", ThreadID: "omt_thread",
	}
	config, err := deliveryConfig(digitalhuman.TaskDeliverySourceConversation, digitalhuman.TaskMutationContext{ReplyTo: reply})
	if err != nil {
		t.Fatal(err)
	}
	if deliveryMode(config) != digitalhuman.TaskDeliverySourceConversation {
		t.Fatalf("delivery mode = %q", deliveryMode(config))
	}
	if deliveryTarget(config) != digitalhuman.TaskDeliveryTargetSourceConversation {
		t.Fatalf("delivery target = %q", deliveryTarget(config))
	}
	if deliveryFallback(config) != digitalhuman.TaskDeliveryFallbackTaskCenter {
		t.Fatalf("delivery fallback = %q", deliveryFallback(config))
	}
	got := replyAddress(config)
	if got == nil || got.Provider != reply.Provider || got.InstallationID != reply.InstallationID || got.ConversationID != reply.ConversationID || got.ThreadID != reply.ThreadID {
		t.Fatalf("reply address = %#v, want %#v", got, reply)
	}
}

func TestDeliveryTargetKeepsLegacyRowsReadable(t *testing.T) {
	if got := deliveryTarget(map[string]any{"mode": string(digitalhuman.TaskDeliverySourceConversation)}); got != digitalhuman.TaskDeliveryTargetSourceConversation {
		t.Fatalf("legacy source target = %q", got)
	}
	if got := deliveryFallback(map[string]any{"mode": string(digitalhuman.TaskDeliverySourceConversation)}); got != digitalhuman.TaskDeliveryFallbackTaskCenter {
		t.Fatalf("legacy source fallback = %q", got)
	}
	if got := deliveryTarget(map[string]any{}); got != digitalhuman.TaskDeliveryTargetTaskCenter {
		t.Fatalf("legacy task target = %q", got)
	}
}
