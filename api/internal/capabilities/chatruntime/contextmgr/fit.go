package contextmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// maxCompactionsPerRun caps semantic compactions in one run. Each one is
	// a full model call over the context; past the cap the oldest history is
	// left out instead.
	maxCompactionsPerRun = 2
	minEstimateScale     = 0.25
	maxEstimateScale     = 2.0
)

// droppedHistoryNotice tells the model that the request leaves out the oldest
// history.
const droppedHistoryNotice = "Earlier parts of this conversation were left out of this request to fit the model's context window. Ask the user if you need something from them."

func clampEstimateScale(scale float64) float64 {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 1
	}
	return min(max(scale, minEstimateScale), maxEstimateScale)
}

// scaledBudget converts a budget in the provider's tokens into estimated
// tokens. The estimator is deliberately conservative, often by a factor of two
// for Chinese; the provider's usage shows by how much (ADR 0038).
func scaledBudget(budget Budget, scale float64) Budget {
	if scale == 1 {
		return budget
	}
	toEstimate := func(tokens int) int {
		if tokens <= 0 {
			return tokens
		}
		return int(float64(tokens) / scale)
	}
	budget.PromptBudget = toEstimate(budget.PromptBudget)
	budget.CompactInputLimit = toEstimate(budget.CompactInputLimit)
	budget.SoftLimit = toEstimate(budget.SoftLimit)
	budget.HardLimit = toEstimate(budget.HardLimit)
	budget.TargetTokens = toEstimate(budget.TargetTokens)
	return budget
}

// fitByDroppingHistory makes a request fit the budget when compaction could
// not, instead of failing the turn (ADR 0038). System messages stay where they
// are. In order, it leaves out:
//
//   - whole earlier turns, oldest first, each a user message and everything
//     up to the next one, so tool calls keep their results;
//   - the oldest rounds of the current turn, keeping its user message and the
//     newest round;
//   - the middle of the current user message, which can be read back with
//     read_context_artifact.
//
// It reports false when even that does not fit, such as when the system prompt
// and tools alone exceed the budget.
func (m *Manager) fitByDroppingHistory(request *adapter.ChatRequest, budget int) (*adapter.ChatRequest, bool) {
	if request == nil || budget <= 0 {
		return request, false
	}
	current := cloneRequest(request)
	fits := func() bool { return m.estimator.EstimateChatRequest(current).Tokens < budget }

	userIndex := lastUserTurnIndex(current.Messages)
	if userIndex < 0 {
		return request, false
	}
	dropped := false
	// Earlier turns, oldest first.
	for !fits() {
		start, end := oldestDroppableUnit(current.Messages, userIndex)
		if start < 0 {
			break
		}
		current.Messages = append(current.Messages[:start], current.Messages[end:]...)
		userIndex -= end - start
		dropped = true
	}
	// Rounds of the current turn, oldest first, keeping the newest.
	for !fits() {
		start, end := oldestCurrentTurnRound(current.Messages, userIndex)
		if start < 0 {
			break
		}
		current.Messages = append(current.Messages[:start], current.Messages[end:]...)
		dropped = true
	}
	if dropped {
		notice := adapter.Message{Role: "system", Content: droppedHistoryNotice}
		current.Messages = append(current.Messages[:userIndex], append([]adapter.Message{notice}, current.Messages[userIndex:]...)...)
		userIndex++
	}
	if !fits() {
		m.projectUserMessage(current, userIndex, budget)
	}
	return current, fits()
}

// lastUserTurnIndex finds the user message that opens the current turn.
func lastUserTurnIndex(messages []adapter.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			return index
		}
	}
	return -1
}

// oldestDroppableUnit returns the oldest earlier turn before limit: from a
// user (or summary) message up to the next user message, skipping system
// messages that open the request.
func oldestDroppableUnit(messages []adapter.Message, limit int) (int, int) {
	start := -1
	for index := 0; index < limit; index++ {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "system") {
			start = index
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := start + 1
	for end < limit && !strings.EqualFold(strings.TrimSpace(messages[end].Role), "user") {
		end++
	}
	// Keep system messages inside the unit, such as turn context, in place.
	for end > start && strings.EqualFold(strings.TrimSpace(messages[end-1].Role), "system") {
		end--
	}
	if end <= start {
		return -1, -1
	}
	return start, end
}

// oldestCurrentTurnRound returns the oldest assistant response of the
// current turn with its tool results, if a newer response follows it.
func oldestCurrentTurnRound(messages []adapter.Message, userIndex int) (int, int) {
	start := -1
	for index := userIndex + 1; index < len(messages); index++ {
		if strings.EqualFold(strings.TrimSpace(messages[index].Role), "assistant") {
			start = index
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := start + 1
	for end < len(messages) && strings.EqualFold(strings.TrimSpace(messages[end].Role), "tool") {
		end++
	}
	for index := end; index < len(messages); index++ {
		if strings.EqualFold(strings.TrimSpace(messages[index].Role), "assistant") {
			return start, end
		}
	}
	return -1, -1
}

// projectUserMessage keeps the beginning and end of an oversized current
// user message, so that a first message larger than the window still runs.
// The original stays readable through read_context_artifact.
func (m *Manager) projectUserMessage(request *adapter.ChatRequest, index int, budget int) {
	content, ok := request.Messages[index].Content.(string)
	if !ok || content == "" {
		return
	}
	request.Messages[index].Content = ""
	rest := m.estimator.EstimateChatRequest(request).Tokens
	request.Messages[index].Content = content
	available := budget - rest - 512
	tokens := m.estimator.EstimateText(content, request.Model).Tokens
	if available <= 0 || tokens <= available {
		return
	}
	runes := len([]rune(content))
	keep := int(float64(runes) * float64(available) / float64(tokens) * 0.9)
	if keep < 200 {
		return
	}
	digest := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(digest[:])
	ref, err := m.rememberOriginal(hash, content)
	if err != nil {
		return
	}
	request.Messages[index].Content = fmt.Sprintf(
		"[This message is %d characters, more than fits the model's context window. Its beginning and end follow; read the rest with read_context_artifact using artifact_ref %s.]\n\n%s",
		runes, ref, headAndTail(content, keep))
}
