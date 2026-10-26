package service

import (
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"testing"
)

func TestScheduledEmployeeTokenBudgetRequiresExactServerRunCorrelation(t *testing.T) {
	id := uuid.New()
	request := runtimedto.ChatRequest{MaxTaskTokens: 60000, OperationContext: map[string]any{"digital_human_run_id": id.String()}}
	for _, source := range []string{runtimemodel.ConversationSourceConsole, runtimemodel.ConversationSourceTask} {
		caller := Caller{Type: runtimemodel.ConversationCallerDigitalHuman, RunID: &id, Source: source}
		if !taskTokenBudgetCallerAllowed(caller, request) {
			t.Fatal("scheduled employee rejected", source)
		}
		wrong := uuid.New()
		caller.RunID = &wrong
		if taskTokenBudgetCallerAllowed(caller, request) {
			t.Fatal("borrowed run budget admitted")
		}
	}
	if taskTokenBudgetCallerAllowed(Caller{Type: runtimemodel.ConversationCallerAIChat, Source: runtimemodel.ConversationSourceConsole}, request) {
		t.Fatal("console caller admitted")
	}
}
