package runtimeworkspace

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// End-user event actions of erasure and export.
const (
	EndUserEventErased   = "erased"
	EndUserEventExported = "exported"
)

// EndUserErasure is what an application domain leaves for after the erase
// commits: storage objects to delete, and cleanup outside the database.
type EndUserErasure struct {
	ObjectKeys []string
	After      func(context.Context)
}

// EndUserDataOwner is an application domain that keeps data of Runtime
// Workspace end users beyond the chat runtime, such as DigitalHumans. It
// erases inside the erase transaction; purge also removes the tombstones a
// plain erase keeps, for a Runtime Workspace teardown.
type EndUserDataOwner interface {
	EraseEndUser(ctx context.Context, tx *gorm.DB, subjectID uuid.UUID, purge bool) (EndUserErasure, error)
	ExportEndUser(ctx context.Context, db *gorm.DB, subjectID uuid.UUID) (key string, data any, err error)
}

// EndUserExportPager is an EndUserDataOwner whose export is read page by
// page, so the export job holds every record of it (ADR 0012). Sections are
// the keys of the owner's export object, each a list of records.
type EndUserExportPager interface {
	EndUserExportSections() (key string, sections []string)
	ExportEndUserPage(ctx context.Context, db *gorm.DB, subjectID uuid.UUID, section string, offset, limit int) ([]map[string]any, error)
}

var endUserDataOwners struct {
	sync.RWMutex
	items []EndUserDataOwner
}

// UseEndUserDataOwner registers an application domain for end-user erasure
// and export. Registering the same owner twice has no effect.
func UseEndUserDataOwner(owner EndUserDataOwner) {
	if owner == nil {
		return
	}
	endUserDataOwners.Lock()
	defer endUserDataOwners.Unlock()
	for _, existing := range endUserDataOwners.items {
		if existing == owner {
			return
		}
	}
	endUserDataOwners.items = append(endUserDataOwners.items, owner)
}

func registeredEndUserDataOwners() []EndUserDataOwner {
	endUserDataOwners.RLock()
	defer endUserDataOwners.RUnlock()
	return append([]EndUserDataOwner(nil), endUserDataOwners.items...)
}

// EraseEndUser deletes everything an end user left in every bound app:
// conversations, files, memory, DigitalHuman instances, runs, tasks,
// reports, notifications, and artifacts. The registry keeps a tombstone
// with identifiers only, and the erasure is recorded. Usage bills and the
// access audit keep their identifiers.
func (s *Service) EraseEndUser(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor) error {
	var after []func(context.Context)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runtimeWorkspace, user, err := s.loadEndUser(ctx, tx, workspaceID, subjectID, true)
		if err != nil {
			return err
		}
		if user.Status == EndUserErased {
			return nil
		}
		now := s.now()
		erasures, err := eraseEndUserData(ctx, tx, subjectID, false, now)
		if err != nil {
			return err
		}
		for _, erasure := range erasures {
			if erasure.After != nil {
				after = append(after, erasure.After)
			}
		}
		for _, table := range []string{"runtime_workspace_end_user_credit_entries", "runtime_workspace_end_user_credit_lots", "runtime_workspace_end_user_credit_bills",
			"runtime_workspace_end_user_notices", "runtime_workspace_webhook_deliveries", "runtime_workspace_end_user_exports"} {
			if err := tx.Exec("DELETE FROM "+table+" WHERE subject_id = ?", subjectID).Error; err != nil {
				return fmt.Errorf("delete end-user credits from %s: %w", table, err)
			}
		}
		if err := tx.Model(&EndUser{}).Where("subject_id = ?", subjectID).Updates(map[string]any{
			"status": EndUserErased, "external_user_id": "", "quota_json": datatypes.JSON("{}"),
			"plan_key": "", "plan_expires_at": nil, "credits_since": nil, "credit_debt": 0,
			"suspended_at": nil, "erased_at": now, "updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("mark end user erased: %w", err)
		}
		return recordEndUserEvent(tx, runtimeWorkspace.LogicalID(), subjectID, EndUserEventErased, actor, now)
	})
	if err != nil {
		return err
	}
	for _, cleanup := range after {
		cleanup(context.WithoutCancel(ctx))
	}
	return nil
}

// eraseEndUserData removes an end user's data in tx and schedules their
// storage objects for deletion.
func eraseEndUserData(ctx context.Context, tx *gorm.DB, subjectID uuid.UUID, purge bool, now time.Time) ([]EndUserErasure, error) {
	db := tx.WithContext(ctx)
	var erasures []EndUserErasure
	var objectKeys []string
	// Application domains go first: their runs and jobs point at the
	// conversations removed below.
	for _, owner := range registeredEndUserDataOwners() {
		erasure, err := owner.EraseEndUser(ctx, tx, subjectID, purge)
		if err != nil {
			return nil, err
		}
		objectKeys = append(objectKeys, erasure.ObjectKeys...)
		erasures = append(erasures, erasure)
	}

	var conversationIDs []uuid.UUID
	if err := db.Table("chat_runtime_conversations").Where("account_id = ?", subjectID).Pluck("id", &conversationIDs).Error; err != nil {
		return nil, fmt.Errorf("list end-user conversations: %w", err)
	}
	toolFiles := db.Table("tool_files").Where("user_id = ?", subjectID)
	if len(conversationIDs) > 0 {
		toolFiles = db.Table("tool_files").Where("user_id = ? OR conversation_id IN ?", subjectID, conversationIDs)
	}
	var toolFileKeys []string
	if err := toolFiles.Where("file_key <> ''").Pluck("file_key", &toolFileKeys).Error; err != nil {
		return nil, fmt.Errorf("list end-user tool files: %w", err)
	}
	objectKeys = append(objectKeys, toolFileKeys...)
	deleteToolFiles := db.Where("user_id = ?", subjectID)
	if len(conversationIDs) > 0 {
		deleteToolFiles = db.Where("user_id = ? OR conversation_id IN ?", subjectID, conversationIDs)
	}
	if err := deleteToolFiles.Table("tool_files").Delete(nil).Error; err != nil {
		return nil, fmt.Errorf("delete end-user tool files: %w", err)
	}
	if len(conversationIDs) > 0 {
		if err := db.Exec("DELETE FROM chat_runtime_messages WHERE conversation_id IN ?", conversationIDs).Error; err != nil {
			return nil, fmt.Errorf("delete end-user messages: %w", err)
		}
		if err := db.Exec("DELETE FROM chat_runtime_conversations WHERE id IN ?", conversationIDs).Error; err != nil {
			return nil, fmt.Errorf("delete end-user conversations: %w", err)
		}
	}

	var relationshipIDs []uuid.UUID
	if err := db.Table("user_memory_relationships").Where("subject_id = ?", subjectID).Pluck("id", &relationshipIDs).Error; err != nil {
		return nil, fmt.Errorf("list end-user memory relationships: %w", err)
	}
	if len(relationshipIDs) > 0 {
		for _, table := range []string{"user_memory_extraction_jobs", "user_memory_undo_records", "user_memory_events", "user_memory_items"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE relationship_id IN ?", relationshipIDs).Error; err != nil {
				return nil, fmt.Errorf("delete end-user memory from %s: %w", table, err)
			}
		}
		if err := db.Exec("DELETE FROM user_memory_relationships WHERE id IN ?", relationshipIDs).Error; err != nil {
			return nil, fmt.Errorf("delete end-user memory relationships: %w", err)
		}
	}
	for _, table := range []string{"agent_memory_values", "agent_memory_undo_records", "agent_memory_events", "agent_memory_extraction_jobs", "agent_memory_subject_states"} {
		if err := db.Exec("DELETE FROM "+table+" WHERE user_scope = 'end_user' AND user_id = ?", subjectID).Error; err != nil {
			return nil, fmt.Errorf("delete end-user agent memory from %s: %w", table, err)
		}
	}

	var rawFileIDs []string
	if err := db.Raw(`
		SELECT file_id::text FROM runtime_external_files WHERE subject_id = ?
		UNION SELECT file_id::text FROM agent_execution_uploads WHERE subject_id = ?
	`, subjectID, subjectID).Scan(&rawFileIDs).Error; err != nil {
		return nil, fmt.Errorf("list end-user files: %w", err)
	}
	fileIDs := make([]uuid.UUID, 0, len(rawFileIDs))
	for _, raw := range rawFileIDs {
		if id, err := uuid.Parse(raw); err == nil {
			fileIDs = append(fileIDs, id)
		}
	}
	if err := db.Exec("DELETE FROM agent_execution_uploads WHERE subject_id = ?", subjectID).Error; err != nil {
		return nil, fmt.Errorf("delete end-user execution uploads: %w", err)
	}
	if len(fileIDs) > 0 {
		var uploadKeys []string
		if err := db.Table("upload_files").Where("id IN ? AND key <> ''", fileIDs).Pluck("key", &uploadKeys).Error; err != nil {
			return nil, fmt.Errorf("list end-user upload objects: %w", err)
		}
		objectKeys = append(objectKeys, uploadKeys...)
		if err := db.Exec("DELETE FROM upload_files WHERE id IN ?", fileIDs).Error; err != nil {
			return nil, fmt.Errorf("delete end-user uploads: %w", err)
		}
	}
	if err := db.Exec("DELETE FROM runtime_external_files WHERE subject_id = ?", subjectID).Error; err != nil {
		return nil, fmt.Errorf("delete end-user file bindings: %w", err)
	}
	if err := scheduleObjectDeletions(ctx, tx, objectKeys, now); err != nil {
		return nil, err
	}
	return erasures, nil
}

// scheduleObjectDeletions queues storage objects for deferred deletion.
func scheduleObjectDeletions(ctx context.Context, tx *gorm.DB, keys []string, now time.Time) error {
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO file_object_deletions (id, object_key, attempt_count, next_attempt_at, last_error, created_at, updated_at)
			VALUES (?, ?, 0, ?, '', ?, ?)
		`, uuid.New(), key, now.Add(RuntimeFileObjectRetention), now, now).Error; err != nil {
			return fmt.Errorf("schedule end-user object deletion: %w", err)
		}
	}
	return nil
}

func recordEndUserEvent(tx *gorm.DB, runtimeWorkspaceID, subjectID uuid.UUID, action string, actor EndUserActor, now time.Time) error {
	var actorID *uuid.UUID
	if actor.ID != uuid.Nil {
		id := actor.ID
		actorID = &id
	}
	if err := tx.Create(&EndUserEvent{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspaceID, SubjectID: subjectID,
		Action: action, ActorKind: actor.Kind, ActorID: actorID, Details: datatypes.JSON("{}"), CreatedAt: now}).Error; err != nil {
		return fmt.Errorf("record end-user event: %w", err)
	}
	return nil
}

// EndUserExport is everything kept for one end user, as one JSON document.
type EndUserExport struct {
	ExportedAt    time.Time              `json:"exported_at"`
	EndUser       EndUserView            `json:"end_user"`
	Conversations []ExportedConversation `json:"conversations"`
	Memory        []any                  `json:"memory"`
	Files         []ExportedFile         `json:"files"`
	Apps          map[string]any         `json:"apps,omitempty"`
	History       []EndUserEvent         `json:"history"`
}

// ExportedConversation is a conversation with its messages.
type ExportedConversation struct {
	ID         uuid.UUID         `json:"id"`
	CallerType string            `json:"caller_type"`
	CallerID   *uuid.UUID        `json:"caller_id,omitempty"`
	Title      string            `json:"title"`
	CreatedAt  time.Time         `json:"created_at"`
	Messages   []ExportedMessage `json:"messages"`
}

// ExportedMessage is one turn of a conversation.
type ExportedMessage struct {
	ID        uuid.UUID `json:"id"`
	Query     string    `json:"query"`
	Answer    string    `json:"answer"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportedFile describes a file the end user uploaded.
type ExportedFile struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}
