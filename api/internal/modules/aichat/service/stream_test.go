package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
)

func TestFinalizePreparedErrorSetsFailedMessageAsCurrentLeaf(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
	}, errors.New("tool failed"))

	if messageRepo.updateErrorID != messageID || messageRepo.updateErrorMessage != "tool failed" {
		t.Fatalf("UpdateError(%s, %q), want (%s, %q)", messageRepo.updateErrorID, messageRepo.updateErrorMessage, messageID, "tool failed")
	}
	if conversationRepo.updateAfterConversationID != conversationID || conversationRepo.updateAfterLeafID != messageID {
		t.Fatalf("UpdateAfterMessage(%s, %s), want (%s, %s)", conversationRepo.updateAfterConversationID, conversationRepo.updateAfterLeafID, conversationID, messageID)
	}
	if conversationRepo.finishActiveCalls != 0 {
		t.Fatalf("FinishActiveMessage calls = %d, want 0", conversationRepo.finishActiveCalls)
	}
}

func TestFinalizePreparedErrorCompletesRootReplacement(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
		ReplaceRoot:  true,
	}, errors.New("model failed"))

	if conversationRepo.completeRootConversationID != conversationID || conversationRepo.completeRootMessageID != messageID {
		t.Fatalf("CompleteRootReplacement(%s, %s), want (%s, %s)", conversationRepo.completeRootConversationID, conversationRepo.completeRootMessageID, conversationID, messageID)
	}
	if conversationRepo.updateAfterConversationID != uuid.Nil {
		t.Fatalf("UpdateAfterMessage was called for root replacement")
	}
}

type recordingConversationRepository struct {
	updateAfterConversationID  uuid.UUID
	updateAfterLeafID          uuid.UUID
	completeRootConversationID uuid.UUID
	completeRootMessageID      uuid.UUID
	finishActiveCalls          int
}

func (r *recordingConversationRepository) Create(ctx context.Context, conversation *aichatmodel.Conversation) error {
	return nil
}
func (r *recordingConversationRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) GetBySourceConversation(ctx context.Context, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) ListScoped(ctx context.Context, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Conversation, int64, error) {
	return nil, 0, nil
}
func (r *recordingConversationRepository) UpdateScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) DeleteScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) StartStreaming(ctx context.Context, id, organizationID, accountID, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) FinishActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	r.finishActiveCalls++
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessages(ctx context.Context, messageIDs []uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) CompleteRootReplacement(ctx context.Context, id, messageID uuid.UUID) error {
	r.completeRootConversationID = id
	r.completeRootMessageID = messageID
	return nil
}
func (r *recordingConversationRepository) UpdateAfterMessage(ctx context.Context, id uuid.UUID, leafMessageID uuid.UUID) error {
	r.updateAfterConversationID = id
	r.updateAfterLeafID = leafMessageID
	return nil
}
func (r *recordingConversationRepository) RefreshAfterMessageDelete(ctx context.Context, id uuid.UUID) error {
	return nil
}

type recordingMessageRepository struct {
	updateErrorID      uuid.UUID
	updateErrorMessage string
}

func (r *recordingMessageRepository) Create(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) GetBySourceMessage(ctx context.Context, sourceMessageID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListByConversationScoped(ctx context.Context, conversationID, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Message, int64, error) {
	return nil, 0, nil
}
func (r *recordingMessageRepository) ListBranch(ctx context.Context, leafID uuid.UUID, maxDepth int) ([]*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) CountByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *recordingMessageRepository) ReplaceRootForStreaming(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) UpdateCompleted(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateError(ctx context.Context, id uuid.UUID, message string) error {
	r.updateErrorID = id
	r.updateErrorMessage = message
	return nil
}
func (r *recordingMessageRepository) MarkStopped(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (r *recordingMessageRepository) UpdateStoppedAnswer(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) DeleteSubtreeScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*repository.MessageDeleteResult, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListStaleActiveIDs(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *recordingMessageRepository) MarkStaleActiveAsError(ctx context.Context, cutoff time.Time, message string) (int64, error) {
	return 0, nil
}
