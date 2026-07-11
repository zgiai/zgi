package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type partialAnswerMessageRepo struct {
	repository.MessageRepository
	answer   string
	metadata map[string]interface{}
}

func (r *partialAnswerMessageRepo) UpdatePartialAnswer(_ context.Context, _ uuid.UUID, answer string, metadata map[string]interface{}) error {
	r.answer = answer
	r.metadata = copyStringAnyMap(metadata)
	return nil
}

func TestPersistPartialSkillLoopAnswerBestEffortKeepsFailedStreamContent(t *testing.T) {
	messageRepo := &partialAnswerMessageRepo{}
	svc := &service{repos: &repository.Repositories{Message: messageRepo}}
	message := &runtimemodel.Message{
		ID:       uuid.New(),
		Metadata: map[string]interface{}{"operation_plan": map[string]interface{}{"status": "running"}},
	}
	prepared := &PreparedChat{Message: message}

	svc.persistPartialSkillLoopAnswerBestEffort(context.Background(), prepared, "partial final answer", &adapter.Usage{TotalTokens: 42})

	if messageRepo.answer != "partial final answer" {
		t.Fatalf("persisted answer = %q, want partial final answer", messageRepo.answer)
	}
	if message.Answer != "partial final answer" {
		t.Fatalf("prepared message answer = %q, want partial final answer", message.Answer)
	}
	if got := mapFromOperationContext(messageRepo.metadata["operation_plan"])["status"]; got != "running" {
		t.Fatalf("operation_plan.status = %#v, want running after failed stream", got)
	}
	if _, ok := messageRepo.metadata["usage"]; !ok {
		t.Fatalf("usage metadata missing: %#v", messageRepo.metadata)
	}
}
