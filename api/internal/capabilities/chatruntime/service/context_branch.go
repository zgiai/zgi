package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// copyConversationTranscriptForBranch copies the durable facts of a source
// conversation up to a boundary into a branch conversation.
//
// A branch owns its facts: entry ids are re-minted and every preview that
// embeds a ref is rewritten to the new id, so the branch never points back at
// the source conversation (which may later be deleted by the retention policy).
// Positions (seq/cseq) are preserved, which is what makes the branch start from
// exactly the context the source had at that boundary.
//
// messageIDMap maps every copied source message onto its row in the branch; a
// conversation branch flow creates those rows first and then calls this.
func (s *service) copyConversationTranscriptForBranch(
	ctx context.Context,
	sourceConversationID uuid.UUID,
	targetConversationID uuid.UUID,
	messageIDMap map[uuid.UUID]uuid.UUID,
	organizationID uuid.UUID,
	throughCSeq int64,
) (int, error) {
	if s == nil || s.repos == nil || s.repos.TranscriptEntry == nil {
		return 0, nil
	}
	if sourceConversationID == uuid.Nil || targetConversationID == uuid.Nil || len(messageIDMap) == 0 {
		return 0, fmt.Errorf("%w: branch copy requires source, target and message mapping", ErrInvalidInput)
	}
	source, err := s.repos.TranscriptEntry.ListForCopy(ctx, sourceConversationID, throughCSeq)
	if err != nil {
		return 0, err
	}
	if len(source) == 0 {
		return 0, nil
	}
	copies := make([]*runtimemodel.TranscriptEntry, 0, len(source))
	for _, entry := range source {
		if entry == nil {
			continue
		}
		targetMessageID, ok := messageIDMap[entry.MessageID]
		if !ok || targetMessageID == uuid.Nil {
			continue
		}
		newID := uuid.New()
		copies = append(copies, &runtimemodel.TranscriptEntry{
			ID:             newID,
			MessageID:      targetMessageID,
			ConversationID: targetConversationID,
			OrganizationID: organizationID,
			CSeq:           entry.CSeq,
			Seq:            entry.Seq,
			Kind:           entry.Kind,
			Preview:        rewriteTranscriptEntryRef(entry.Preview, entry.ID, newID),
			Payload:        entry.Payload,
			ByteSize:       entry.ByteSize,
		})
	}
	if len(copies) == 0 {
		return 0, nil
	}
	if err := s.repos.TranscriptEntry.Append(ctx, copies); err != nil {
		return 0, err
	}
	return len(copies), nil
}

// rewriteTranscriptEntryRef repoints an archived-result reference at the copied
// entry. Only the preview text carries the ref, so nothing else has to change.
func rewriteTranscriptEntryRef(preview map[string]interface{}, from, to uuid.UUID) map[string]interface{} {
	if len(preview) == 0 || from == uuid.Nil || to == uuid.Nil || from == to {
		return preview
	}
	content, ok := preview["content"].(string)
	if !ok || !strings.Contains(content, from.String()) {
		return preview
	}
	rewritten := make(map[string]interface{}, len(preview))
	for key, value := range preview {
		rewritten[key] = value
	}
	rewritten["content"] = strings.ReplaceAll(content, from.String(), to.String())
	return rewritten
}
