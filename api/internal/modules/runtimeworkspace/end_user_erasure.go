package runtimeworkspace

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// End-user event actions of erasure and export.
const (
	EndUserEventErased   = "erased"
	EndUserEventExported = "exported"
)

// Export caps keep one export a bounded response.
const (
	endUserExportConversationLimit = 500
	endUserExportMessageLimit      = 2000
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
			"runtime_workspace_end_user_notices", "runtime_workspace_webhook_deliveries"} {
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
	Truncated     bool                   `json:"truncated,omitempty"`
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

// ExportEndUser collects an end user's data in every bound app and records
// the export.
func (s *Service) ExportEndUser(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor) (*EndUserExport, error) {
	view, err := s.GetEndUser(ctx, workspaceID, subjectID)
	if err != nil {
		return nil, err
	}
	if view.Status == EndUserErased {
		return nil, ErrEndUserNotFound
	}
	runtimeWorkspace, _, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	export := &EndUserExport{ExportedAt: s.now(), EndUser: *view, Conversations: []ExportedConversation{}, Memory: []any{}, Files: []ExportedFile{}}

	var conversations []struct {
		ID         uuid.UUID
		CallerType string
		CallerID   *uuid.UUID
		Title      string
		CreatedAt  time.Time
	}
	if err := db.Table("chat_runtime_conversations").Select("id, caller_type, caller_id, title, created_at").
		Where("account_id = ? AND deleted_at IS NULL", subjectID).Order("created_at").Limit(endUserExportConversationLimit + 1).Scan(&conversations).Error; err != nil {
		return nil, fmt.Errorf("export end-user conversations: %w", err)
	}
	if len(conversations) > endUserExportConversationLimit {
		conversations, export.Truncated = conversations[:endUserExportConversationLimit], true
	}
	index := make(map[uuid.UUID]int, len(conversations))
	ids := make([]uuid.UUID, 0, len(conversations))
	for _, conversation := range conversations {
		index[conversation.ID] = len(export.Conversations)
		ids = append(ids, conversation.ID)
		export.Conversations = append(export.Conversations, ExportedConversation{ID: conversation.ID, CallerType: conversation.CallerType,
			CallerID: conversation.CallerID, Title: conversation.Title, CreatedAt: conversation.CreatedAt, Messages: []ExportedMessage{}})
	}
	if len(ids) > 0 {
		var messages []struct {
			ExportedMessage
			ConversationID uuid.UUID
		}
		if err := db.Table("chat_runtime_messages").Select("id, conversation_id, query, answer, status, created_at").
			Where("conversation_id IN ? AND deleted_at IS NULL", ids).Order("created_at").Limit(endUserExportMessageLimit + 1).Scan(&messages).Error; err != nil {
			return nil, fmt.Errorf("export end-user messages: %w", err)
		}
		if len(messages) > endUserExportMessageLimit {
			messages, export.Truncated = messages[:endUserExportMessageLimit], true
		}
		for _, message := range messages {
			position := index[message.ConversationID]
			export.Conversations[position].Messages = append(export.Conversations[position].Messages, message.ExportedMessage)
		}
	}

	var relationshipIDs []uuid.UUID
	if err := db.Table("user_memory_relationships").Where("subject_id = ?", subjectID).Pluck("id", &relationshipIDs).Error; err != nil {
		return nil, fmt.Errorf("export end-user memory: %w", err)
	}
	memory := relationshipmemory.NewService(s.db)
	for _, relationshipID := range relationshipIDs {
		exported, err := memory.Export(ctx, relationshipID)
		if err != nil {
			return nil, fmt.Errorf("export end-user memory: %w", err)
		}
		export.Memory = append(export.Memory, exported)
	}

	if err := db.Table("upload_files AS files").Select("files.id, files.name, files.size, files.mime_type, files.created_at").
		Joins("JOIN runtime_external_files AS bindings ON bindings.file_id = files.id").
		Where("bindings.subject_id = ?", subjectID).Order("files.created_at").Scan(&export.Files).Error; err != nil {
		return nil, fmt.Errorf("export end-user files: %w", err)
	}
	for _, owner := range registeredEndUserDataOwners() {
		key, data, err := owner.ExportEndUser(ctx, s.db, subjectID)
		if err != nil {
			return nil, err
		}
		if key != "" && data != nil {
			if export.Apps == nil {
				export.Apps = map[string]any{}
			}
			export.Apps[key] = data
		}
	}
	if err := recordEndUserEvent(s.db.WithContext(ctx), runtimeWorkspace.LogicalID(), subjectID, EndUserEventExported, actor, s.now()); err != nil {
		return nil, err
	}
	export.History = []EndUserEvent{}
	for page := 1; ; page++ {
		events, err := s.ListEndUserEvents(ctx, workspaceID, subjectID, page, 200)
		if err != nil {
			return nil, err
		}
		export.History = append(export.History, events.Items...)
		if !events.HasMore {
			break
		}
	}
	return export, nil
}
