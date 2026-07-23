package service

import adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"

// beginPreparedUsageExecution snapshots the usage already persisted for a
// same-message continuation. The snapshot stays in memory so partial metadata
// writes during this execution cannot make the final aggregation double count.
func beginPreparedUsageExecution(prepared *PreparedChat) {
	if prepared == nil || prepared.usageExecutionStarted {
		return
	}
	prepared.usageExecutionStarted = true
	if (!prepared.Continuation && !prepared.usageContinuation) || prepared.Message == nil {
		return
	}
	prepared.usageExecutionBaseline = usageFromMetadata(prepared.Message.Metadata)
}

func preparedResultMetadataForPrepared(prepared *PreparedChat, source map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata := preparedResultMetadata(source, usage)
	return applyPreparedUsageMetadata(prepared, metadata, usage)
}

func applyPreparedUsageMetadata(prepared *PreparedChat, metadata map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if prepared == nil || (!prepared.Continuation && !prepared.usageContinuation) {
		metadata["usage"] = usageMetadata(usage)
		return metadata
	}
	beginPreparedUsageExecution(prepared)
	cumulative := addUsageCloned(prepared.usageExecutionBaseline, usage)
	metadata["usage"] = usageMetadata(cumulative)
	metadata["latest_execution_usage"] = usageMetadata(usage)
	metadata["usage_scope"] = "message"
	return metadata
}

func usageFromMetadata(metadata map[string]interface{}) *adapter.Usage {
	if len(metadata) == 0 {
		return nil
	}
	usageMap, ok := metadata["usage"].(map[string]interface{})
	if !ok || len(usageMap) == 0 {
		return nil
	}
	usage := &adapter.Usage{
		PromptTokens:     intValueFromAny(usageMap["prompt_tokens"]),
		CompletionTokens: intValueFromAny(usageMap["completion_tokens"]),
		TotalTokens:      intValueFromAny(usageMap["total_tokens"]),
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func addUsageCloned(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if current == nil && next == nil {
		return nil
	}
	result := &adapter.Usage{}
	if current != nil {
		*result = *current
	}
	if next != nil {
		result.PromptTokens += next.PromptTokens
		result.CompletionTokens += next.CompletionTokens
		result.TotalTokens += next.TotalTokens
	}
	return result
}
