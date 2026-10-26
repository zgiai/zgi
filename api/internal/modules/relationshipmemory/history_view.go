package relationshipmemory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The user's view of their memory history (ADR 0013): commits newest first,
// each with the files it changed, the content before and after, the user's
// words it was based on, and why. The latest change to a file can be
// reverted, which writes the content before it back as a new change.

const (
	defaultHistoryPageSize = 20
	maxHistoryPageSize     = 50
)

type HistoryRequest struct {
	// Key limits the history to one file.
	Key   string
	Limit int
	// Before continues a page: only commits created before it are returned.
	Before *time.Time
}

type HistoryVersion struct {
	ID             uuid.UUID  `json:"id"`
	Key            string     `json:"key"`
	DefinitionID   *uuid.UUID `json:"definition_id,omitempty"`
	Action         string     `json:"action"`
	BeforeRevision *int64     `json:"before_revision,omitempty"`
	AfterRevision  *int64     `json:"after_revision,omitempty"`
	Before         string     `json:"before"`
	After          string     `json:"after"`
	Evidence       string     `json:"evidence,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	ContentPurged  bool       `json:"content_purged"`
	// Revertible is true for the latest change to a file whose content is
	// still kept and still current.
	Revertible bool `json:"revertible"`
}

type HistoryCommit struct {
	ID             uuid.UUID        `json:"id"`
	SourceKind     string           `json:"source_kind"`
	ActorType      string           `json:"actor_type"`
	Source         string           `json:"source"`
	ConversationID *uuid.UUID       `json:"conversation_id,omitempty"`
	MessageID      *uuid.UUID       `json:"message_id,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	Versions       []HistoryVersion `json:"versions"`
}

type HistoryPage struct {
	Commits []HistoryCommit `json:"commits"`
	// NextBefore continues the history when more commits exist.
	NextBefore *time.Time `json:"next_before,omitempty"`
}

// History returns a relationship's commits, newest first.
func (s *Service) History(ctx context.Context, relationshipID uuid.UUID, req HistoryRequest) (*HistoryPage, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultHistoryPageSize
	}
	if limit > maxHistoryPageSize {
		limit = maxHistoryPageSize
	}
	db := s.db.WithContext(ctx)
	key := strings.TrimSpace(req.Key)
	commitQuery := db.Model(&MemoryCommit{}).Where("relationship_id = ?", relationshipID)
	if key != "" {
		commitQuery = commitQuery.Where("id IN (?)", db.Model(&MemoryFileVersion{}).Select("commit_id").Where("relationship_id = ? AND memory_key = ?", relationshipID, key))
	}
	if req.Before != nil {
		commitQuery = commitQuery.Where("created_at < ?", *req.Before)
	}
	var commits []MemoryCommit
	if err := commitQuery.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&commits).Error; err != nil {
		return nil, fmt.Errorf("list memory commits: %w", err)
	}
	page := &HistoryPage{Commits: make([]HistoryCommit, 0, len(commits))}
	if len(commits) > limit {
		commits = commits[:limit]
		next := commits[len(commits)-1].CreatedAt
		page.NextBefore = &next
	}
	if len(commits) == 0 {
		return page, nil
	}
	commitIDs := make([]uuid.UUID, 0, len(commits))
	for _, commit := range commits {
		commitIDs = append(commitIDs, commit.ID)
	}
	versionQuery := db.Where("commit_id IN ?", commitIDs)
	if key != "" {
		versionQuery = versionQuery.Where("memory_key = ?", key)
	}
	var versions []MemoryFileVersion
	if err := versionQuery.Order("created_at ASC, id ASC").Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("list memory versions: %w", err)
	}
	latest, current, err := s.latestVersions(ctx, relationshipID)
	if err != nil {
		return nil, err
	}
	var relationship UserMemoryRelationship
	if err := db.Select("id", "consumer_type").First(&relationship, "id = ?", relationshipID).Error; err != nil {
		return nil, fmt.Errorf("read memory relationship: %w", err)
	}
	byCommit := map[uuid.UUID][]HistoryVersion{}
	for _, version := range versions {
		view, err := s.historyVersion(ctx, version)
		if err != nil {
			return nil, err
		}
		view.Revertible = latest[version.MemoryKey] == version.ID && version.ContentPurgedAt == nil && versionIsCurrent(version, current) &&
			versionRestorable(version, current, relationship.ConsumerType == ConsumerDigitalHuman)
		byCommit[version.CommitID] = append(byCommit[version.CommitID], view)
	}
	for _, commit := range commits {
		page.Commits = append(page.Commits, HistoryCommit{
			ID: commit.ID, SourceKind: commit.SourceKind, ActorType: commit.ActorType, Source: commit.Source,
			ConversationID: commit.ConversationID, MessageID: commit.MessageID, CreatedAt: commit.CreatedAt,
			Versions: byCommit[commit.ID],
		})
	}
	return page, nil
}

// RevertVersion undoes the latest change to a file by writing back the
// content it had before, as a new change. It refuses a change that is no
// longer the file's latest, or whose content has been purged.
func (s *Service) RevertVersion(ctx context.Context, relationshipID, versionID uuid.UUID, contract ConsumerMemoryContract, metadata MutationMetadata) (*UserMemoryItem, error) {
	var version MemoryFileVersion
	err := s.db.WithContext(ctx).First(&version, "id = ? AND relationship_id = ?", versionID, relationshipID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUndoUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("read memory version: %w", err)
	}
	if version.ContentPurgedAt != nil {
		return nil, ErrUndoUnavailable
	}
	latest, current, err := s.latestVersions(ctx, relationshipID)
	if err != nil {
		return nil, err
	}
	if latest[version.MemoryKey] != version.ID || !versionIsCurrent(version, current) {
		return nil, ErrRevisionConflict
	}
	if !versionRestorable(version, current, contract.DefinitionMode == "dynamic") {
		return nil, ErrUndoUnavailable
	}
	view, err := s.historyVersion(ctx, version)
	if err != nil {
		return nil, err
	}
	metadata.Reason = "reverted a change from " + version.CreatedAt.UTC().Format(time.RFC3339)
	metadata.Evidence = ""
	item, exists := current[version.MemoryKey]
	if version.BeforeRevision == nil || strings.TrimSpace(view.Before) == "" {
		if !exists {
			return nil, nil
		}
		if _, err := s.DeleteItem(ctx, relationshipID, version.MemoryKey, item.Revision, metadata); err != nil {
			return nil, err
		}
		return nil, nil
	}
	// A deleted value comes back with the name and category it had.
	expected, displayName, category := int64(0), version.BeforeDisplayName, version.BeforeCategory
	if exists {
		expected, displayName, category = item.Revision, item.DisplayName, item.Category
	}
	return s.PutItem(ctx, relationshipID, PutItemRequest{
		Key: version.MemoryKey, DisplayName: displayName, Category: category, Content: view.Before,
		ExpectedRevision: expected, Contract: contract, Metadata: metadata,
	})
}

// versionRestorable reports whether reverting version can write its value
// back. A deleted Digital Human memory needs its category, which history
// recorded before it kept categories cannot supply.
func versionRestorable(version MemoryFileVersion, current map[string]UserMemoryItem, needsCategory bool) bool {
	if _, exists := current[version.MemoryKey]; exists || version.BeforeRevision == nil || !needsCategory {
		return true
	}
	return version.BeforeCategory != ""
}

// latestVersions returns, per file, the ID of its newest version and its
// current item.
func (s *Service) latestVersions(ctx context.Context, relationshipID uuid.UUID) (map[string]uuid.UUID, map[string]UserMemoryItem, error) {
	db := s.db.WithContext(ctx)
	var rows []struct {
		ID        uuid.UUID
		MemoryKey string
	}
	err := db.Raw(`SELECT DISTINCT ON (memory_key) id, memory_key FROM memory_file_versions
		WHERE relationship_id = ? ORDER BY memory_key, created_at DESC, id DESC`, relationshipID).Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("find latest memory versions: %w", err)
	}
	latest := make(map[string]uuid.UUID, len(rows))
	for _, row := range rows {
		latest[row.MemoryKey] = row.ID
	}
	var items []UserMemoryItem
	if err := db.Where("relationship_id = ?", relationshipID).Find(&items).Error; err != nil {
		return nil, nil, fmt.Errorf("read memory items: %w", err)
	}
	current := make(map[string]UserMemoryItem, len(items))
	for _, item := range items {
		current[item.MemoryKey] = item
	}
	return latest, current, nil
}

// versionIsCurrent reports whether the file is still as the version left it.
func versionIsCurrent(version MemoryFileVersion, current map[string]UserMemoryItem) bool {
	item, exists := current[version.MemoryKey]
	if version.AfterRevision == nil {
		return !exists
	}
	return exists && item.Revision == *version.AfterRevision
}

func (s *Service) historyVersion(ctx context.Context, version MemoryFileVersion) (HistoryVersion, error) {
	view := HistoryVersion{
		ID: version.ID, Key: version.MemoryKey, DefinitionID: version.DefinitionID, Action: version.Action,
		BeforeRevision: version.BeforeRevision, AfterRevision: version.AfterRevision,
		Evidence: version.Evidence, Reason: version.Reason, ContentPurged: version.ContentPurgedAt != nil,
	}
	if view.ContentPurged {
		return view, nil
	}
	var err error
	if version.BeforeRevision != nil {
		if view.Before, err = s.codec.Decode(ctx, EncodedContent{Payload: version.BeforePayload, Encoding: version.BeforeEncoding, KeyID: version.BeforeKeyID}); err != nil {
			return view, fmt.Errorf("decode memory version: %w", err)
		}
	}
	if version.AfterRevision != nil {
		if view.After, err = s.codec.Decode(ctx, EncodedContent{Payload: version.AfterPayload, Encoding: version.AfterEncoding, KeyID: version.AfterKeyID}); err != nil {
			return view, fmt.Errorf("decode memory version: %w", err)
		}
	}
	return view, nil
}

func truncateHistoryText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// Item returns one decoded item of a relationship, or nil when the key has
// none. File-style editors read the current value before changing it.
func (s *Service) Item(ctx context.Context, relationshipID uuid.UUID, key string) (*UserMemoryItem, error) {
	var item UserMemoryItem
	err := s.db.WithContext(ctx).First(&item, "relationship_id = ? AND memory_key = ?", relationshipID, strings.TrimSpace(key)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read memory item: %w", err)
	}
	content, err := s.codec.Decode(ctx, EncodedContent{Payload: item.ContentPayload, Encoding: item.ContentEncoding, KeyID: item.ContentKeyID})
	if err != nil {
		return nil, fmt.Errorf("decode memory item: %w", err)
	}
	item.ContentPayload = content
	return &item, nil
}
