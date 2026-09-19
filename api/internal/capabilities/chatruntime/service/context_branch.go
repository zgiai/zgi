package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/gorm"
)

// copyConversationTranscriptForBranch copies the durable facts reachable from
// a source head into a branch conversation.
//
// A branch owns its facts: entry ids are re-minted and every preview that
// embeds a ref is rewritten to the new id, so the branch never points back at
// the source conversation (which may later be deleted by the retention policy).
// Parent-chain and summary-boundary references are remapped so the target owns
// an independent log; no source entry id is retained.
//
// messageIDMap maps every copied source message onto its row in the branch; a
// conversation branch flow creates those rows first and then calls this.
func (s *service) copyConversationTranscriptForBranch(
	ctx context.Context,
	sourceHeadEntryID uuid.UUID,
	targetConversationID uuid.UUID,
	messageIDMap map[uuid.UUID]uuid.UUID,
	organizationID uuid.UUID,
) (int, error) {
	if s == nil || s.repos == nil || s.repos.TranscriptEntry == nil {
		return 0, nil
	}
	if sourceHeadEntryID == uuid.Nil || targetConversationID == uuid.Nil || len(messageIDMap) == 0 {
		return 0, fmt.Errorf("%w: branch copy requires source, target and message mapping", ErrInvalidInput)
	}
	source, err := s.repos.TranscriptEntry.ListForCopyByHead(ctx, sourceHeadEntryID, nil)
	if err != nil {
		return 0, err
	}
	if len(source) == 0 {
		return 0, nil
	}
	idMap := make(map[uuid.UUID]uuid.UUID, len(source))
	for _, entry := range source {
		if entry == nil {
			continue
		}
		if targetMessageID, ok := messageIDMap[entry.MessageID]; !ok || targetMessageID == uuid.Nil {
			return 0, fmt.Errorf("%w: missing target message mapping for source message %s", ErrInvalidInput, entry.MessageID)
		}
		idMap[entry.ID] = uuid.New()
	}
	copies := make([]*runtimemodel.TranscriptEntry, 0, len(source))
	lastByMessage := make(map[uuid.UUID]uuid.UUID)
	for _, entry := range source {
		if entry == nil {
			continue
		}
		targetMessageID, ok := messageIDMap[entry.MessageID]
		if !ok || targetMessageID == uuid.Nil {
			continue
		}
		newID := idMap[entry.ID]
		parentID := uuid.Nil
		if entry.ParentEntryID != nil {
			parentID = idMap[*entry.ParentEntryID]
		}
		if entry.ParentEntryID != nil && parentID == uuid.Nil {
			return 0, fmt.Errorf("%w: source parent %s is outside the copied chain", ErrInvalidInput, entry.ParentEntryID)
		}
		var parentPtr *uuid.UUID
		if parentID != uuid.Nil {
			parentPtr = &parentID
		}
		var boundaryPtr *uuid.UUID
		if entry.CoversThroughEntryID != nil {
			boundaryID := idMap[*entry.CoversThroughEntryID]
			if boundaryID == uuid.Nil {
				return 0, fmt.Errorf("%w: source summary boundary %s is outside the copied chain", ErrInvalidInput, entry.CoversThroughEntryID)
			}
			boundaryPtr = &boundaryID
		}
		copies = append(copies, &runtimemodel.TranscriptEntry{
			ID:                   newID,
			MessageID:            targetMessageID,
			ConversationID:       targetConversationID,
			OrganizationID:       organizationID,
			Kind:                 entry.Kind,
			ParentEntryID:        parentPtr,
			CoversThroughEntryID: boundaryPtr,
			EventID:              "branch:" + newID.String(),
			Preview:              rewriteTranscriptEntryRef(entry.Preview, entry.ID, newID),
			Payload:              cloneTranscriptEntryMap(entry.Payload),
			ByteSize:             entry.ByteSize,
		})
		lastByMessage[targetMessageID] = newID
	}
	if len(copies) == 0 {
		return 0, nil
	}
	appendAndSet := func(entries []*runtimemodel.TranscriptEntry, heads map[uuid.UUID]uuid.UUID, repo repository.TranscriptEntryRepository) error {
		if err := repo.Append(ctx, entries); err != nil {
			return err
		}
		for messageID, head := range heads {
			if err := repo.SetMessageHead(ctx, messageID, &head); err != nil {
				return err
			}
		}
		return nil
	}
	if s.repos.DB != nil {
		if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return appendAndSet(copies, lastByMessage, repository.NewRepositories(tx).TranscriptEntry)
		}); err != nil {
			return 0, err
		}
	} else if err := appendAndSet(copies, lastByMessage, s.repos.TranscriptEntry); err != nil {
		return 0, err
	}
	return len(copies), nil
}

// rewriteTranscriptEntryRef repoints an archived-result reference at the copied
// entry. Only the preview text carries the ref, so nothing else has to change.
func rewriteTranscriptEntryRef(preview map[string]interface{}, from, to uuid.UUID) map[string]interface{} {
	rewritten := cloneTranscriptEntryMap(preview)
	if len(rewritten) == 0 || from == uuid.Nil || to == uuid.Nil || from == to {
		return rewritten
	}
	content, ok := rewritten["content"].(string)
	if !ok || !strings.Contains(content, from.String()) {
		return rewritten
	}
	rewritten["content"] = strings.ReplaceAll(content, from.String(), to.String())
	return rewritten
}

// cloneTranscriptEntryMap makes branch copies independent even before GORM
// serializes them. Payload and preview are nested JSON maps; a shallow copy
// would leave slices and child maps shared with the source object.
func cloneTranscriptEntryMap(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	cloned, ok := cloneTranscriptEntryValue(source).(map[string]interface{})
	if !ok {
		return nil
	}
	return cloned
}

func cloneTranscriptEntryValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			cloned[key] = cloneTranscriptEntryValue(child)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneTranscriptEntryValue(child)
		}
		return cloned
	default:
		return value
	}
}
