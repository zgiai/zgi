package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"gorm.io/gorm"
)

// A failed turn whose cause has no product code used to be persisted without
// any error_code, so a reloaded conversation could only show the stored text
// and rendered it as a server error. The stable catalog code the stream frame
// publishes is now stored on the message too.
func TestRecordPreparedErrorIdentityStoresStableCodeForUnclassifiedErrors(t *testing.T) {
	repo := &countingTimelineMessageRepo{}
	svc := &service{repos: &repository.Repositories{Message: repo}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{"error_params": map[string]interface{}{"stale": true}}},
	}

	cause := fmt.Errorf("failed to select provider: %w", llmerrors.NewModelNotFoundErrorWithName("zzz-unknown-qa"))
	svc.recordPreparedErrorIdentity(context.Background(), prepared, cause)

	if repo.updates != 1 || repo.metadata["error_code"] != "llm.model.not_found" {
		t.Fatalf("persisted metadata = %#v after %d update(s), want error_code llm.model.not_found", repo.metadata, repo.updates)
	}
	if _, ok := repo.metadata["error_params"]; ok {
		t.Fatalf("stale error_params kept: %#v", repo.metadata)
	}
	if prepared.Message.Metadata["error_code"] != "llm.model.not_found" {
		t.Fatalf("prepared metadata = %#v, want the persisted identity", prepared.Message.Metadata)
	}

	// A product error keeps its own code, as before.
	repo = &countingTimelineMessageRepo{}
	svc.repos.Message = repo
	svc.recordPreparedErrorIdentity(context.Background(), prepared, ErrModelIdleTimeout)
	if repo.metadata["error_code"] != aichatErrorCodeModelServiceTimeout {
		t.Fatalf("product error_code = %#v, want %s", repo.metadata["error_code"], aichatErrorCodeModelServiceTimeout)
	}
}

func TestRecordPreparedErrorIdentityToleratesFinishedMessage(t *testing.T) {
	repo := &finishedMessageRepo{}
	svc := &service{repos: &repository.Repositories{Message: repo}}
	prepared := &PreparedChat{Message: &runtimemodel.Message{ID: uuid.New()}}
	svc.recordPreparedErrorIdentity(context.Background(), prepared, errors.New("boom"))
	if prepared.Message.Metadata != nil {
		t.Fatalf("metadata = %#v, want unchanged when the message no longer accepts writes", prepared.Message.Metadata)
	}
}

type finishedMessageRepo struct{ repository.MessageRepository }

func (finishedMessageRepo) UpdateMetadata(context.Context, uuid.UUID, map[string]interface{}) error {
	return gorm.ErrRecordNotFound
}
