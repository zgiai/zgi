package knowledge

import (
	"fmt"
	"strings"
	"unicode/utf8"

	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	defaultAgentKnowledgeRetrievalBudget = 20
	maxAgentKnowledgeQueryRunes          = 500
	maxAgentKnowledgeNoNewEvidence       = 2
	knowledgeRetrievalHistoryParam       = "knowledge_retrieval_history"
	knowledgeRetrievalRoundParam         = "knowledge_retrieval_round"
	knowledgeRetrievalBudgetParam        = "knowledge_retrieval_budget"
)

// agentKnowledgeExecutionState contains the request-scoped budget and successful
// retrieval history supplied by the skill loop. It is deliberately runtime-local;
// it is not persisted as conversation state.
type agentKnowledgeExecutionState struct {
	round   int
	budget  int
	history []map[string]interface{}
}

func (r knowledgeRuntime) agentKnowledgeExecutionState() agentKnowledgeExecutionState {
	state := agentKnowledgeExecutionState{budget: defaultAgentKnowledgeRetrievalBudget}
	if r.runtime == nil {
		state.round = 1
		return state
	}
	state.round = intValue(r.runtime.RuntimeParameters, knowledgeRetrievalRoundParam, 1)
	if state.round <= 0 {
		state.round = 1
	}
	state.budget = intValue(r.runtime.RuntimeParameters, knowledgeRetrievalBudgetParam, defaultAgentKnowledgeRetrievalBudget)
	if state.budget <= 0 {
		state.budget = defaultAgentKnowledgeRetrievalBudget
	}
	state.history = retrievalHistoryValue(r.runtime.RuntimeParameters[knowledgeRetrievalHistoryParam])
	return state
}

func (r knowledgeRuntime) isAgentInvocation() bool {
	return r.runtime != nil && r.runtime.InvokeFrom == tools.ToolInvokeFromAgent
}

func blockedAgentKnowledgeResponse(query string, state agentKnowledgeExecutionState, reason string, warning string) *dataset_service.KnowledgeRetrieveResponse {
	remaining := state.budget - state.round
	if remaining < 0 {
		remaining = 0
	}
	stopReason := strings.TrimSpace(reason)
	continuationAllowed := remaining > 0 && (stopReason == "duplicate_query" || stopReason == "query_too_long" || stopReason == "query_required")
	warnings := []string{}
	if strings.TrimSpace(warning) != "" {
		warnings = append(warnings, strings.TrimSpace(warning))
	}
	return &dataset_service.KnowledgeRetrieveResponse{
		Query:       strings.TrimSpace(query),
		Status:      dataset_service.KnowledgeRetrieveStatusBlocked,
		ResultCount: 0,
		Warnings:    warnings,
		Resources:   []dataset_service.KnowledgeRetrieverResource{},
		Control: &dataset_service.KnowledgeRetrievalControl{
			ContinuationAllowed: continuationAllowed,
			RemainingRounds:     remaining,
			Round:               state.round,
			NewEvidenceCount:    0,
			StopReason:          &stopReason,
		},
	}
}

func preflightAgentKnowledgeRetrieval(query string, state agentKnowledgeExecutionState) *dataset_service.KnowledgeRetrieveResponse {
	if utf8.RuneCountInString(strings.TrimSpace(query)) > maxAgentKnowledgeQueryRunes {
		return blockedAgentKnowledgeResponse(
			query,
			state,
			"query_too_long",
			fmt.Sprintf("query exceeds the %d-character retrieval limit; shorten it while preserving the key evidence need", maxAgentKnowledgeQueryRunes),
		)
	}
	if state.round > state.budget {
		return blockedAgentKnowledgeResponse(
			query,
			state,
			"retrieval_round_limit",
			"retrieval execution budget reached; answer from the evidence already retrieved",
		)
	}
	normalizedQuery := normalizeRetrievalQuery(query)
	if normalizedQuery == "" {
		return blockedAgentKnowledgeResponse(query, state, "query_required", "a non-empty retrieval query is required")
	}
	for _, invocation := range state.history {
		if normalizeRetrievalQuery(retrievalHistoryQuery(invocation)) == normalizedQuery {
			return blockedAgentKnowledgeResponse(
				query,
				state,
				"duplicate_query",
				"this query was already used in the current retrieval execution; use a materially different evidence query or answer from the existing results",
			)
		}
	}
	return nil
}

func attachAgentKnowledgeControl(response *dataset_service.KnowledgeRetrieveResponse, state agentKnowledgeExecutionState) {
	if response == nil {
		return
	}
	newEvidenceCount := newKnowledgeEvidenceCount(response.Resources, state.history)
	stopReason := ""
	continuationAllowed := true
	if response.Status == dataset_service.KnowledgeRetrieveStatusNoConfig {
		continuationAllowed = false
		stopReason = "no_config"
	}
	if state.round >= state.budget {
		continuationAllowed = false
		stopReason = "retrieval_round_limit"
	}
	if newEvidenceCount == 0 && priorNoNewEvidenceCount(state.history)+1 >= maxAgentKnowledgeNoNewEvidence {
		continuationAllowed = false
		stopReason = "no_new_evidence"
	}
	control := &dataset_service.KnowledgeRetrievalControl{
		ContinuationAllowed: continuationAllowed,
		RemainingRounds:     maxInt(state.budget-state.round, 0),
		Round:               state.round,
		NewEvidenceCount:    newEvidenceCount,
	}
	if stopReason != "" {
		control.StopReason = &stopReason
		appendKnowledgeWarning(response, knowledgeStopWarning(stopReason))
	}
	response.Control = control
}

func appendKnowledgeWarning(response *dataset_service.KnowledgeRetrieveResponse, warning string) {
	if response == nil || strings.TrimSpace(warning) == "" {
		return
	}
	response.Warnings = append(response.Warnings, warning)
}

func knowledgeStopWarning(reason string) string {
	switch strings.TrimSpace(reason) {
	case "retrieval_round_limit":
		return "retrieval execution budget reached; answer from the evidence already retrieved"
	case "no_new_evidence":
		return "recent retrievals added no new evidence; stop retrieving and report any remaining uncertainty"
	case "no_config":
		return "the Agent has no configured knowledge bases; do not retry retrieval"
	default:
		return "retrieval cannot continue; answer from the evidence already retrieved"
	}
}

func normalizeRetrievalQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}

func retrievalHistoryValue(raw interface{}) []map[string]interface{} {
	switch typed := raw.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(map[string]interface{}); ok && len(value) > 0 {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func retrievalHistoryQuery(invocation map[string]interface{}) string {
	arguments := mapValue(invocation, "arguments")
	if query := stringValue(arguments, "query"); query != "" {
		return query
	}
	if query := stringValue(invocation, "query"); query != "" {
		return query
	}
	return stringValue(mapValue(invocation, "result"), "query")
}

func priorNoNewEvidenceCount(history []map[string]interface{}) int {
	count := 0
	for index := len(history) - 1; index >= 0; index-- {
		result := mapValue(history[index], "result")
		newEvidence, ok := retrievalHistoryNewEvidenceCount(result)
		if !ok || newEvidence != 0 {
			break
		}
		count++
	}
	return count
}

func retrievalHistoryNewEvidenceCount(result map[string]interface{}) (int, bool) {
	if result == nil {
		return 0, false
	}
	switch control := result["control"].(type) {
	case *dataset_service.KnowledgeRetrievalControl:
		if control == nil {
			return 0, false
		}
		return control.NewEvidenceCount, true
	case dataset_service.KnowledgeRetrievalControl:
		return control.NewEvidenceCount, true
	default:
		return intValueOK(mapValue(result, "control"), "new_evidence_count")
	}
}

func newKnowledgeEvidenceCount(resources []dataset_service.KnowledgeRetrieverResource, history []map[string]interface{}) int {
	seen := map[string]struct{}{}
	for _, invocation := range history {
		result := mapValue(invocation, "result")
		for _, resource := range retrievalHistoryResources(result) {
			if key := knowledgeResourceKey(resource); key != "" {
				seen[key] = struct{}{}
			}
		}
	}
	newCount := 0
	current := map[string]struct{}{}
	for _, resource := range resources {
		key := knowledgeResourceKey(resource)
		if key == "" {
			continue
		}
		if _, duplicate := current[key]; duplicate {
			continue
		}
		current[key] = struct{}{}
		if _, existed := seen[key]; !existed {
			newCount++
		}
	}
	return newCount
}

func retrievalHistoryResources(result map[string]interface{}) []dataset_service.KnowledgeRetrieverResource {
	raw, ok := result["retriever_resources"]
	if !ok {
		return nil
	}
	resources := make([]dataset_service.KnowledgeRetrieverResource, 0)
	switch typed := raw.(type) {
	case []dataset_service.KnowledgeRetrieverResource:
		return typed
	case []interface{}:
		for _, item := range typed {
			if resource, ok := item.(map[string]interface{}); ok {
				resources = append(resources, knowledgeResourceFromMap(resource))
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			resources = append(resources, knowledgeResourceFromMap(item))
		}
	}
	return resources
}

func knowledgeResourceFromMap(value map[string]interface{}) dataset_service.KnowledgeRetrieverResource {
	return dataset_service.KnowledgeRetrieverResource{
		SegmentID:    stringValue(value, "segment_id"),
		Content:      stringValue(value, "content"),
		DocumentID:   stringValue(value, "document_id"),
		DatasetID:    stringValue(value, "dataset_id"),
		DocumentName: stringValue(value, "document_name"),
		DatasetName:  stringValue(value, "dataset_name"),
	}
}

func knowledgeResourceKey(resource dataset_service.KnowledgeRetrieverResource) string {
	if segmentID := strings.TrimSpace(resource.SegmentID); segmentID != "" {
		return "segment:" + segmentID
	}
	content := normalizeRetrievalQuery(resource.Content)
	if content == "" {
		return ""
	}
	return "content:" + content
}

func intValueOK(values map[string]interface{}, key string) (int, bool) {
	if values == nil {
		return 0, false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return 0, false
	}
	return intValue(values, key, 0), true
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}
