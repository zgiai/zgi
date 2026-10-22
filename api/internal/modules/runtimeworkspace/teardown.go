package runtimeworkspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RuntimeFileObjectRetention = 30 * 24 * time.Hour

var ErrWorkspaceTeardownRequiresDisabled = errors.New("runtime workspace must be disabled before teardown")

type TeardownResult struct {
	RuntimeWorkspaceID uuid.UUID
	SubjectCount       int
	ConversationCount  int
	FileCount          int
}

func TeardownAttachedWorkspace(ctx context.Context, tx *gorm.DB, workspaceID uuid.UUID, now time.Time) (*TeardownResult, error) {
	if tx == nil || workspaceID == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}
	var runtimeWorkspace RuntimeWorkspace
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ?", workspaceID).Take(&runtimeWorkspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &TeardownResult{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load Runtime Workspace for teardown: %w", err)
	}
	if runtimeWorkspace.Status != StatusDisabled {
		return nil, ErrWorkspaceTeardownRequiresDisabled
	}
	runtimeID := runtimeWorkspace.LogicalID()
	result := &TeardownResult{RuntimeWorkspaceID: runtimeID}

	var subjectIDs []uuid.UUID
	if err := tx.WithContext(ctx).Table("user_subjects").Where("issuer_type = ? AND issuer_id = ?", SubjectIssuerType, runtimeID).Pluck("id", &subjectIDs).Error; err != nil {
		return nil, fmt.Errorf("list Runtime Subjects for teardown: %w", err)
	}
	result.SubjectCount = len(subjectIDs)

	type runtimeFile struct {
		ID  string
		Key string
	}
	var files []runtimeFile
	if err := tx.WithContext(ctx).Table("upload_files AS files").
		Select("files.id, files.key").
		Joins("JOIN runtime_external_files AS runtime_files ON runtime_files.file_id = files.id").
		Where("runtime_files.runtime_workspace_id = ?", runtimeID).
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list Runtime files for teardown: %w", err)
	}
	fileIDs := make([]string, 0, len(files))
	for _, file := range files {
		fileIDs = append(fileIDs, file.ID)
		if file.Key == "" {
			continue
		}
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO file_object_deletions
				(id, object_key, attempt_count, next_attempt_at, last_error, created_at, updated_at)
			VALUES (?, ?, 0, ?, '', ?, ?)
		`, uuid.New(), file.Key, now.Add(RuntimeFileObjectRetention), now, now).Error; err != nil {
			return nil, fmt.Errorf("schedule Runtime file object deletion: %w", err)
		}
	}
	result.FileCount = len(fileIDs)
	if len(fileIDs) > 0 {
		if err := tx.WithContext(ctx).Exec("DELETE FROM upload_files WHERE id IN ?", fileIDs).Error; err != nil {
			return nil, fmt.Errorf("delete Runtime file records: %w", err)
		}
	}

	if len(subjectIDs) > 0 {
		var relationshipIDs []uuid.UUID
		if err := tx.WithContext(ctx).Table("user_memory_relationships").Where("subject_id IN ?", subjectIDs).Pluck("id", &relationshipIDs).Error; err != nil {
			return nil, fmt.Errorf("list Runtime memory relationships: %w", err)
		}
		if len(relationshipIDs) > 0 {
			for _, table := range []string{"user_memory_extraction_jobs", "user_memory_undo_records", "user_memory_events", "user_memory_items"} {
				if err := tx.WithContext(ctx).Exec("DELETE FROM "+table+" WHERE relationship_id IN ?", relationshipIDs).Error; err != nil {
					return nil, fmt.Errorf("delete Runtime memory data from %s: %w", table, err)
				}
			}
			if err := tx.WithContext(ctx).Exec("DELETE FROM user_memory_relationships WHERE id IN ?", relationshipIDs).Error; err != nil {
				return nil, fmt.Errorf("delete Runtime memory relationships: %w", err)
			}
		}

		var conversationIDs []uuid.UUID
		if err := tx.WithContext(ctx).Table("chat_runtime_conversations").Where("account_id IN ?", subjectIDs).Pluck("id", &conversationIDs).Error; err != nil {
			return nil, fmt.Errorf("list Runtime conversations: %w", err)
		}
		result.ConversationCount = len(conversationIDs)
		if len(conversationIDs) > 0 {
			if err := tx.WithContext(ctx).Exec("DELETE FROM chat_runtime_messages WHERE conversation_id IN ?", conversationIDs).Error; err != nil {
				return nil, fmt.Errorf("delete Runtime messages: %w", err)
			}
			if err := tx.WithContext(ctx).Exec("DELETE FROM chat_runtime_conversations WHERE id IN ?", conversationIDs).Error; err != nil {
				return nil, fmt.Errorf("delete Runtime conversations: %w", err)
			}
		}
		if err := tx.WithContext(ctx).Exec("DELETE FROM user_subjects WHERE id IN ?", subjectIDs).Error; err != nil {
			return nil, fmt.Errorf("delete Runtime Subjects: %w", err)
		}
	}

	for _, table := range []string{"runtime_workspace_end_user_events", "runtime_workspace_end_users"} {
		if err := tx.WithContext(ctx).Exec("DELETE FROM "+table+" WHERE runtime_workspace_id = ?", runtimeID).Error; err != nil {
			return nil, fmt.Errorf("delete Runtime end users from %s: %w", table, err)
		}
	}
	if err := tx.WithContext(ctx).Exec("DELETE FROM runtime_workspaces WHERE runtime_id = ?", runtimeID).Error; err != nil {
		return nil, fmt.Errorf("delete Runtime Workspace: %w", err)
	}
	return result, nil
}
