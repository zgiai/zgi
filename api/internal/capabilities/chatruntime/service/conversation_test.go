package service

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestNormalizeConversationSourcePreservesExternalAPI(t *testing.T) {
	if got := normalizeConversationSource(runtimemodel.ConversationSourceExternalAPI); got != runtimemodel.ConversationSourceExternalAPI {
		t.Fatalf("normalizeConversationSource(external-api) = %q, want %q", got, runtimemodel.ConversationSourceExternalAPI)
	}
}

func TestNormalizeConversationSourcePreservesScheduledTask(t *testing.T) {
	if got := normalizeConversationSource(runtimemodel.ConversationSourceTask); got != runtimemodel.ConversationSourceTask {
		t.Fatalf("scheduled task source became %q", got)
	}
}
