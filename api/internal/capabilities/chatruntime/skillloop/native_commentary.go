package skillloop

import (
	"context"
	"regexp"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	nativeCommentaryDispositionProcess = "process"
	nativeCommentaryDispositionDiscard = "discard"

	nativeCommentaryMaxTokens      = 384
	nativeCommentaryMaxCount       = 8
	nativeCommentaryTurnTokenLimit = 1920

	nativeCommentaryRejectNoBusinessTool   = "no_executable_business_tool"
	nativeCommentaryRejectTokenLimit       = "token_limit_exceeded"
	nativeCommentaryRejectTurnBudget       = "turn_budget_exceeded"
	nativeCommentaryRejectCountLimit       = "commentary_count_exceeded"
	nativeCommentaryRejectInternalIdentity = "internal_identifier_detected"
)

var nativeCommentaryUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)

type nativeCommentaryState struct {
	model          string
	count          int
	totalTokens    int
	startedAt      time.Time
	firstVisibleAt time.Time
}

type nativeCommentaryDecision struct {
	disposition string
	reason      string
	tokens      int
	count       int
	totalTokens int
	firstDelay  time.Duration
}

func newNativeCommentaryState(model string, metadata map[string]interface{}) *nativeCommentaryState {
	state := &nativeCommentaryState{model: strings.TrimSpace(model), startedAt: time.Now()}
	state.restorePresentationBudget(metadata)
	return state
}

func (s *nativeCommentaryState) classify(content string, calls []adapter.ToolCall, toolSet *skills.NativeToolSet) nativeCommentaryDecision {
	if s == nil {
		s = newNativeCommentaryState("", nil)
	}
	text := strings.TrimSpace(content)
	tokens := modelInvocationTokenEstimator.EstimateText(text, s.model).Tokens
	decision := nativeCommentaryDecision{
		disposition: nativeCommentaryDispositionDiscard,
		tokens:      tokens,
		count:       s.count,
		totalTokens: s.totalTokens,
	}

	business, controlsOnly := nativeCommentaryCallKinds(calls, toolSet)
	switch {
	case !business && !controlsOnly:
		decision.reason = nativeCommentaryRejectNoBusinessTool
	case tokens > nativeCommentaryMaxTokens:
		decision.reason = nativeCommentaryRejectTokenLimit
	case s.count >= nativeCommentaryMaxCount:
		decision.reason = nativeCommentaryRejectCountLimit
	case s.totalTokens+tokens > nativeCommentaryTurnTokenLimit:
		decision.reason = nativeCommentaryRejectTurnBudget
	case nativeCommentaryContainsInternalIdentifier(text, calls, toolSet):
		decision.reason = nativeCommentaryRejectInternalIdentity
	default:
		decision.disposition = nativeCommentaryDispositionProcess
		s.count++
		s.totalTokens += tokens
		if s.firstVisibleAt.IsZero() {
			s.firstVisibleAt = time.Now()
			decision.firstDelay = s.firstVisibleAt.Sub(s.startedAt)
		}
		decision.count = s.count
		decision.totalTokens = s.totalTokens
	}
	return decision
}

func nativeCommentaryCallKinds(calls []adapter.ToolCall, toolSet *skills.NativeToolSet) (business bool, controlsOnly bool) {
	if len(calls) == 0 {
		return false, false
	}
	bindings := map[string]struct{}{}
	if toolSet != nil {
		for alias := range toolSet.ToolBindings {
			bindings[strings.ToLower(strings.TrimSpace(alias))] = struct{}{}
		}
	}
	controlsOnly = true
	for _, call := range calls {
		name := strings.ToLower(strings.TrimSpace(call.Function.Name))
		if _, ok := bindings[name]; ok {
			controlsOnly = false
			if _, err := skills.ParseArguments(call.Function.Arguments); err == nil {
				business = true
			}
			continue
		}
		if !nativeCommentaryControlTool(name) {
			controlsOnly = false
		}
	}
	return business, controlsOnly
}

func nativeCommentaryControlTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case skills.MetaToolActivateSkills,
		skills.MetaToolSearchSkills,
		skills.MetaToolUpdatePlan,
		skills.MetaToolTurnState,
		skills.MetaToolRequestUserInput,
		skills.MetaToolReadSkillReference,
		contextArtifactToolName:
		return true
	default:
		return false
	}
}

func nativeCommentaryContainsInternalIdentifier(content string, calls []adapter.ToolCall, toolSet *skills.NativeToolSet) bool {
	if nativeCommentaryUUIDPattern.MatchString(content) {
		return true
	}
	identifiers := make([]string, 0, len(calls)*2+16)
	for _, name := range []string{
		skills.MetaToolActivateSkills,
		skills.MetaToolSearchSkills,
		skills.MetaToolUpdatePlan,
		skills.MetaToolTurnState,
		skills.MetaToolRequestUserInput,
		skills.MetaToolReadSkillReference,
		skills.MetaToolLoadSkill,
		skills.MetaToolCallSkillTool,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolFinalAnswer,
		contextArtifactToolName,
	} {
		identifiers = append(identifiers, name)
	}
	for _, call := range calls {
		identifiers = append(identifiers, call.ID, call.Function.Name)
	}
	if toolSet != nil {
		identifiers = append(identifiers, toolSet.ActiveSkillIDs...)
		for alias, binding := range toolSet.ToolBindings {
			identifiers = append(identifiers, alias, binding.SkillID, binding.ToolName)
		}
	}
	for _, identifier := range identifiers {
		if nativeCommentaryContainsIdentifier(content, identifier) {
			return true
		}
	}
	return false
}

func nativeCommentaryContainsIdentifier(content string, identifier string) bool {
	content = strings.ToLower(content)
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" {
		return false
	}
	for offset := 0; ; {
		index := strings.Index(content[offset:], identifier)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !nativeCommentaryIdentifierRune(rune(content[index-1]))
		afterIndex := index + len(identifier)
		afterOK := afterIndex >= len(content) || !nativeCommentaryIdentifierRune(rune(content[afterIndex]))
		if beforeOK && afterOK {
			return true
		}
		offset = index + len(identifier)
		if offset >= len(content) {
			return false
		}
	}
}

func nativeCommentaryIdentifierRune(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}

func (s *nativeCommentaryState) restorePresentationBudget(metadata map[string]interface{}) {
	if s == nil || len(metadata) == 0 {
		return
	}
	presentation, _ := metadata["presentation"].(map[string]interface{})
	if len(presentation) == 0 {
		return
	}
	for _, raw := range nativeCommentarySlice(presentation["items"]) {
		item, _ := raw.(map[string]interface{})
		if !strings.EqualFold(strings.TrimSpace(nativeCommentaryString(item["kind"])), "text") ||
			!strings.EqualFold(strings.TrimSpace(nativeCommentaryString(item["content_phase"])), "process") {
			continue
		}
		content := nativeCommentaryString(item["content"])
		if strings.TrimSpace(content) == "" {
			continue
		}
		s.count++
		s.totalTokens += modelInvocationTokenEstimator.EstimateText(content, s.model).Tokens
	}
}

func nativeCommentarySlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func nativeCommentaryString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func (r *Runner) logNativeCommentaryDecision(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	decision nativeCommentaryDecision,
	reasoningObserved bool,
) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return
	}
	logger.InfoContext(ctx, "aichat native commentary classified",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"round", round+1,
		"estimated_tokens", decision.tokens,
		"disposition", decision.disposition,
		"rejection_reason", decision.reason,
		"commentary_count", decision.count,
		"commentary_total_tokens", decision.totalTokens,
		"first_commentary_delay_ms", decision.firstDelay.Milliseconds(),
		"reasoning_observed", reasoningObserved,
	)
}
