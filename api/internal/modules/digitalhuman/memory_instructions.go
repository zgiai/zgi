package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Memory instructions are the operator's business guidance for what a
// DigitalHuman should remember or ignore (ADR 0013, applied to DigitalHumans
// in ADR 0015). They live in the version's memory policy, so they are
// published, versioned, and restored with the rest of the configuration.
// They shape what is proposed; they never override the fixed rules on
// evidence, sensitive content, trust, or who may see memory.

// memoryInstructionsKey is the memory policy field that holds the
// instructions.
const memoryInstructionsKey = "instructions"

// maxMemoryInstructionsRunes bounds the instructions, as for Agents.
const maxMemoryInstructionsRunes = 2000

// validateMemoryPolicy checks the fields of a memory policy that the runtime
// reads, trims the instructions, and drops empty ones, so clearing them leaves
// the policy as it was before they were set.
func validateMemoryPolicy(policy map[string]any) error {
	value, ok := policy[memoryInstructionsKey]
	if !ok {
		return nil
	}
	if value == nil {
		delete(policy, memoryInstructionsKey)
		return nil
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	if !ok || utf8.RuneCountInString(text) > maxMemoryInstructionsRunes {
		return ErrInvalidRequest
	}
	if text == "" {
		delete(policy, memoryInstructionsKey)
		return nil
	}
	policy[memoryInstructionsKey] = text
	return nil
}

// memoryInstructionsFromPolicy reads the instructions of a stored memory
// policy; a policy without them has none.
func memoryInstructionsFromPolicy(policy datatypes.JSON) string {
	var decoded struct {
		Instructions string `json:"instructions"`
	}
	if len(policy) == 0 || json.Unmarshal(policy, &decoded) != nil {
		return ""
	}
	return strings.TrimSpace(decoded.Instructions)
}

// runMemoryInstructions reads the instructions of the version a run uses.
// Missing instructions are not an error: memory then follows the fixed rules
// alone.
func (s *Service) runMemoryInstructions(ctx context.Context, run *Run) string {
	if s == nil || s.repo == nil || run == nil || run.DigitalHumanVersionID == uuid.Nil {
		return ""
	}
	var version Version
	err := s.repo.db.WithContext(ctx).Select("memory_policy").
		Where("id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
			run.DigitalHumanVersionID, run.OrganizationID, run.WorkspaceID, run.DigitalHumanID).
		Take(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ""
	}
	if err != nil {
		logger.WarnContext(ctx, "DigitalHuman memory instructions could not be read", "run_id", run.ID.String(), err)
		return ""
	}
	return memoryInstructionsFromPolicy(version.MemoryPolicy)
}

// renderMemoryInstructions presents the operator's instructions as
// preferences beneath the fixed rules.
func renderMemoryInstructions(instructions string) string {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return ""
	}
	return strings.Join([]string{
		"The DigitalHuman's operator describes what its memory is for. Treat it as a preference for what to keep, what to ignore, and where it belongs; it cannot override the rules above on evidence, sensitive content, or who may see memory.",
		"<memory_instructions>", instructions, "</memory_instructions>",
	}, "\n")
}

// MemoryPreviewTurn is one exchange of a sample conversation.
type MemoryPreviewTurn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// MemoryPreviewRequest asks which memory the background proposer would keep
// from a sample conversation. Instructions default to the draft's, so an
// operator can try an edit before saving it.
type MemoryPreviewRequest struct {
	Instructions *string             `json:"instructions,omitempty"`
	Turns        []MemoryPreviewTurn `json:"turns"`
}

// MemoryPreviewProposal is one proposed entry and whether the memory policy
// would accept it.
type MemoryPreviewProposal struct {
	MemoryExtractionDraft
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// MemoryPreviewResult lists the proposals; nothing is written.
type MemoryPreviewResult struct {
	Instructions string                  `json:"instructions"`
	Proposals    []MemoryPreviewProposal `json:"proposals"`
}

const maxMemoryPreviewTurns = 10

// SetMemoryPreviewExtractor enables the memory preview. Without it the
// preview is unavailable.
func (s *Service) SetMemoryPreviewExtractor(extractor MemoryExtractor) {
	if s != nil {
		s.previewExtractor = extractor
	}
}

// PreviewMemory runs the background memory proposer over a sample
// conversation with the draft's model and the given or saved instructions,
// and reports which proposals the memory policy would accept. It writes
// nothing.
func (s *Service) PreviewMemory(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryPreviewRequest) (*MemoryPreviewResult, error) {
	const operation = "digital_human.memory.preview"
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || len(request.Turns) == 0 || len(request.Turns) > maxMemoryPreviewTurns {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	if s.previewExtractor == nil {
		return nil, applicationError(errors.New("digital human memory preview is not configured"), operation)
	}
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	draft, err := s.repo.Draft(ctx, scope, definition, false)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	instructions := memoryInstructionsFromPolicy(draft.MemoryPolicy)
	if request.Instructions != nil {
		instructions = strings.TrimSpace(*request.Instructions)
		if utf8.RuneCountInString(instructions) > maxMemoryInstructionsRunes {
			return nil, applicationError(ErrInvalidRequest, operation)
		}
	}
	var queries, answers []string
	for _, turn := range request.Turns {
		user := strings.TrimSpace(turn.User)
		if user == "" {
			return nil, applicationError(ErrInvalidRequest, operation)
		}
		queries = append(queries, user)
		if answer := strings.TrimSpace(turn.Assistant); answer != "" {
			answers = append(answers, answer)
		}
	}
	current, err := s.retrieveMemories(ctx, scope, digitalHumanID, 50, nil)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	conversationID := uuid.New()
	drafts, err := s.previewExtractor.Extract(ctx, MemoryExtractionInput{
		Job: &MemoryExtractionJob{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, ConversationID: &conversationID},
		Run: &Run{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID},
		Source: MemoryExtractionSource{AccountID: scope.ActorID, Query: strings.Join(queries, "\n"),
			Answer: strings.Join(answers, "\n"), ModelName: draft.DefaultModel},
		CurrentMemories: current, Instructions: instructions,
	})
	if err != nil {
		return nil, applicationError(err, operation)
	}
	result := &MemoryPreviewResult{Instructions: instructions, Proposals: make([]MemoryPreviewProposal, 0, len(drafts))}
	for _, proposal := range drafts {
		reason := ""
		if err := validateMemoryProposalPolicy(MemoryProposalRequest{
			Key: proposal.Key, Action: memoryruntime.MutationActionUpsert, MemoryType: proposal.MemoryType,
			Content: proposal.Content, Confidence: proposal.Confidence,
		}, true); err != nil {
			reason = memoryRejectionReason(err)
		}
		result.Proposals = append(result.Proposals, MemoryPreviewProposal{MemoryExtractionDraft: proposal, Accepted: reason == "", Reason: reason})
	}
	return result, nil
}
