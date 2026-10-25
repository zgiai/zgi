package relationshipmemory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/memoryblock"
	"gorm.io/gorm"
)

// Version history (ADR 0013): every change to a memory value belongs to a
// commit, and each changed value gets a version row holding its content
// before and after. Commits and versions are removed with their relationship,
// so permanent deletion and erasure never leave history behind.

const (
	VersionCreate = "create"
	VersionUpdate = "update"
	VersionDelete = "delete"
	VersionClear  = "clear"
	VersionUndo   = "undo"

	// versionContentRetention bounds how long version content is kept; the
	// commit and version records themselves follow the audit window.
	versionContentRetention = 30 * 24 * time.Hour
	historyRetention        = 180 * 24 * time.Hour
)

type MemoryCommit struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RelationshipID uuid.UUID  `gorm:"type:uuid;not null;index" json:"relationship_id"`
	Owner          string     `gorm:"type:varchar(16);not null" json:"owner"`
	SourceKind     string     `gorm:"type:varchar(32);not null" json:"source_kind"`
	ActorType      string     `gorm:"type:varchar(32);not null" json:"actor_type"`
	ActorID        *uuid.UUID `gorm:"type:uuid" json:"actor_id,omitempty"`
	Source         string     `gorm:"type:varchar(32);not null" json:"source"`
	OperationID    *uuid.UUID `gorm:"type:uuid" json:"operation_id,omitempty"`
	ConversationID *uuid.UUID `gorm:"type:uuid" json:"conversation_id,omitempty"`
	MessageID      *uuid.UUID `gorm:"type:uuid" json:"message_id,omitempty"`
	Message        string     `gorm:"type:text;not null;default:''" json:"message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (MemoryCommit) TableName() string { return "memory_commits" }

type MemoryFileVersion struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	CommitID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"commit_id"`
	RelationshipID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"relationship_id"`
	DefinitionID    *uuid.UUID `gorm:"type:uuid" json:"definition_id,omitempty"`
	MemoryKey       string     `gorm:"type:varchar(64);not null" json:"key"`
	Action          string     `gorm:"type:varchar(16);not null" json:"action"`
	BeforeRevision  *int64     `json:"before_revision,omitempty"`
	AfterRevision   *int64     `json:"after_revision,omitempty"`
	BeforePayload   string     `gorm:"type:text;not null;default:''" json:"-"`
	BeforeEncoding  string     `gorm:"type:varchar(24);not null;default:'plain'" json:"-"`
	BeforeKeyID     *string    `gorm:"type:varchar(128)" json:"-"`
	AfterPayload    string     `gorm:"type:text;not null;default:''" json:"-"`
	AfterEncoding   string     `gorm:"type:varchar(24);not null;default:'plain'" json:"-"`
	AfterKeyID      *string    `gorm:"type:varchar(128)" json:"-"`
	ContentPurgedAt *time.Time `json:"content_purged_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (MemoryFileVersion) TableName() string { return "memory_file_versions" }

type commitScopeKey struct{}

type commitScope struct{ commits map[uuid.UUID]uuid.UUID }

// WithCommitScope makes the value writes done with the returned context share
// one commit per relationship, as one model tool call or one background
// extraction pass should.
func WithCommitScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, commitScopeKey{}, &commitScope{commits: map[uuid.UUID]uuid.UUID{}})
}

// commitFor returns the commit the current write belongs to, creating it when
// the scope has none yet or its commit was rolled back with a failed write.
func (s *Service) commitFor(ctx context.Context, tx *gorm.DB, relationshipID uuid.UUID, meta MutationMetadata) (uuid.UUID, error) {
	scope, _ := ctx.Value(commitScopeKey{}).(*commitScope)
	if scope != nil {
		if id, ok := scope.commits[relationshipID]; ok {
			var count int64
			if err := tx.Model(&MemoryCommit{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return uuid.Nil, fmt.Errorf("find memory commit: %w", err)
			}
			if count == 1 {
				return id, nil
			}
		}
	}
	commit := MemoryCommit{
		ID: uuid.New(), RelationshipID: relationshipID, Owner: memoryblock.OwnerUser,
		SourceKind: normalizedSourceKind(meta.SourceKind), ActorType: fallback(meta.ActorType, "user"), ActorID: meta.ActorID,
		Source: fallback(meta.Source, "api"), ConversationID: meta.SourceConversationID, MessageID: meta.SourceMessageID,
		CreatedAt: s.now(),
	}
	if meta.OperationID != uuid.Nil {
		operationID := meta.OperationID
		commit.OperationID = &operationID
	}
	if err := tx.Create(&commit).Error; err != nil {
		return uuid.Nil, fmt.Errorf("create memory commit: %w", err)
	}
	if scope != nil {
		scope.commits[relationshipID] = commit.ID
	}
	return commit.ID, nil
}

// recordVersion writes one value change into the current commit. before or
// after is nil when the value did not exist on that side of the change.
func (s *Service) recordVersion(ctx context.Context, tx *gorm.DB, relationship *UserMemoryRelationship, meta MutationMetadata, action, key string, definitionID *uuid.UUID, before, after *UserMemoryItem) error {
	commitID, err := s.commitFor(ctx, tx, relationship.ID, meta)
	if err != nil {
		return err
	}
	version := MemoryFileVersion{ID: uuid.New(), CommitID: commitID, RelationshipID: relationship.ID, DefinitionID: definitionID, MemoryKey: key, Action: action, CreatedAt: s.now()}
	if before != nil {
		revision := before.Revision
		version.BeforeRevision, version.BeforePayload, version.BeforeEncoding, version.BeforeKeyID = &revision, before.ContentPayload, before.ContentEncoding, before.ContentKeyID
	}
	if after != nil {
		revision := after.Revision
		version.AfterRevision, version.AfterPayload, version.AfterEncoding, version.AfterKeyID = &revision, after.ContentPayload, after.ContentEncoding, after.ContentKeyID
	}
	if err := tx.Create(&version).Error; err != nil {
		return fmt.Errorf("record memory version: %w", err)
	}
	return nil
}

// definitionIDFor resolves the block definition a value belongs to. Only Agent
// values have definitions in this phase.
func definitionIDFor(ctx context.Context, tx *gorm.DB, relationship *UserMemoryRelationship, key string) (*uuid.UUID, error) {
	if relationship.ConsumerType != ConsumerAgent {
		return nil, nil
	}
	return memoryblock.DefinitionID(ctx, tx, memoryblock.ConsumerAgent, relationship.ConsumerID, key)
}

// purgeVersionContent removes the content of a relationship's history, keeping
// who changed what and when. A user clearing their memory expects its old
// values gone too.
func (s *Service) purgeVersionContent(tx *gorm.DB, relationshipID uuid.UUID) error {
	return tx.Model(&MemoryFileVersion{}).Where("relationship_id = ? AND content_purged_at IS NULL", relationshipID).
		Updates(map[string]any{"before_payload": "", "after_payload": "", "before_key_id": nil, "after_key_id": nil, "content_purged_at": s.now()}).Error
}

// CleanupExpiredHistory purges version content older than its retention and
// deletes commits, with their versions, older than the audit window.
func (s *Service) CleanupExpiredHistory(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("cleanup memory history: service is not configured")
	}
	now := s.now()
	db := s.db.WithContext(ctx)
	if err := db.Model(&MemoryFileVersion{}).Where("content_purged_at IS NULL AND created_at < ?", now.Add(-versionContentRetention)).
		Updates(map[string]any{"before_payload": "", "after_payload": "", "before_key_id": nil, "after_key_id": nil, "content_purged_at": now}).Error; err != nil {
		return fmt.Errorf("purge expired memory version content: %w", err)
	}
	if err := db.Where("created_at < ?", now.Add(-historyRetention)).Delete(&MemoryCommit{}).Error; err != nil {
		return fmt.Errorf("delete expired memory commits: %w", err)
	}
	return nil
}
