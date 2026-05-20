package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	llmGatewayTracerName         = "zgi.llm.gateway"
	defaultOTELPayloadCharacters = 65536
	maxRerankTraceResults        = 5

	otelObservationGeneration = "generation"
	otelObservationDefault    = "DEFAULT"
	otelObservationError      = "ERROR"

	otelLLMCaptureNone    = "none"
	otelLLMCaptureSummary = "summary"
	otelLLMCaptureFull    = "full"
)

type llmTracePayload struct {
	Name            string
	Operation       string
	Input           interface{}
	Output          interface{}
	ModelParameters interface{}
	StartTime       time.Time
	EndTime         time.Time
	Billing         *BillingContext
	Usage           *llmTraceUsage
	Err             error
}

type llmTraceUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (s *llmGatewayServiceImpl) traceChatCompletion(
	ctx context.Context,
	req *adapter.ChatRequest,
	resp *adapter.ChatResponse,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:            "llm.chat",
		Operation:       "chat",
		Input:           chatInput(req),
		Output:          chatCompletionOutput(resp),
		ModelParameters: chatModelParameters(req),
		StartTime:       startTime,
		EndTime:         endTime,
		Billing:         billingCtx,
		Err:             err,
	})
}

func (s *llmGatewayServiceImpl) traceStreamingChatCompletion(
	ctx context.Context,
	req *adapter.ChatRequest,
	fullResponse string,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	promptTokens int,
	completionTokens int,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:      "llm.chat.stream",
		Operation: "chat",
		Input:     chatInput(req),
		Output: map[string]interface{}{
			"role":    "assistant",
			"content": fullResponse,
		},
		ModelParameters: map[string]interface{}{
			"stream": true,
		},
		StartTime: startTime,
		EndTime:   endTime,
		Billing:   billingCtx,
		Usage: &llmTraceUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
		Err: err,
	})
}

func (s *llmGatewayServiceImpl) traceCreateResponse(
	ctx context.Context,
	req *adapter.CreateResponseRequest,
	resp *adapter.CreateResponseResponse,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:            "llm.responses",
		Operation:       "responses",
		Input:           responseInput(req),
		Output:          responseOutput(resp),
		ModelParameters: responseModelParameters(req),
		StartTime:       startTime,
		EndTime:         endTime,
		Billing:         billingCtx,
		Err:             err,
	})
}

func (s *llmGatewayServiceImpl) traceEmbeddings(
	ctx context.Context,
	req *adapter.EmbeddingsRequest,
	resp *adapter.EmbeddingsResponse,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:            "llm.embeddings",
		Operation:       "embeddings",
		Input:           embeddingInputSummary(req),
		Output:          embeddingOutputSummary(resp),
		ModelParameters: embeddingModelParameters(req),
		StartTime:       startTime,
		EndTime:         endTime,
		Billing:         billingCtx,
		Err:             err,
	})
}

func (s *llmGatewayServiceImpl) traceRerank(
	ctx context.Context,
	req *adapter.RerankRequest,
	resp *adapter.RerankResponse,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:            "llm.rerank",
		Operation:       "rerank",
		Input:           rerankInputSummary(req),
		Output:          rerankOutputSummary(resp),
		ModelParameters: rerankModelParameters(req),
		StartTime:       startTime,
		EndTime:         endTime,
		Billing:         billingCtx,
		Err:             err,
	})
}

func (s *llmGatewayServiceImpl) traceImageGeneration(
	ctx context.Context,
	req *adapter.ImageRequest,
	resp *adapter.ImageResponse,
	startTime time.Time,
	endTime time.Time,
	billingCtx *BillingContext,
	err error,
) {
	traceLLMOperation(ctx, llmTracePayload{
		Name:            "llm.images",
		Operation:       "image_generation",
		Input:           imageInputSummary(req),
		Output:          imageOutputSummary(resp),
		ModelParameters: imageModelParameters(req),
		StartTime:       startTime,
		EndTime:         endTime,
		Billing:         billingCtx,
		Err:             err,
	})
}

func traceLLMOperation(ctx context.Context, payload llmTracePayload) {
	if payload.Billing == nil {
		return
	}
	if payload.StartTime.IsZero() {
		payload.StartTime = time.Now()
	}
	if payload.EndTime.IsZero() || payload.EndTime.Before(payload.StartTime) {
		payload.EndTime = time.Now()
	}

	tracer := otel.Tracer(llmGatewayTracerName)
	_, span := tracer.Start(
		ctx,
		payload.Name,
		oteltrace.WithTimestamp(payload.StartTime),
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
	)
	defer span.End(oteltrace.WithTimestamp(payload.EndTime))

	span.SetAttributes(observability.SanitizeAttributes(baseLLMAttributes(payload))...)
	span.SetAttributes(observability.SanitizeAttributes(payloadAttributes(payload))...)

	traceErr := payload.Err
	if traceErr == nil && payload.Billing.Status == "error" && payload.Billing.ErrorMessage != "" {
		traceErr = fmt.Errorf("%s", payload.Billing.ErrorMessage)
	}
	if traceErr != nil {
		errMessage := observability.SanitizeString(traceErr.Error())
		span.RecordError(observability.SanitizeError(traceErr))
		span.SetStatus(codes.Error, errMessage)
		attrs := []attribute.KeyValue{attribute.String("error.message", errMessage)}
		if llmLangfuseAttributesEnabled() {
			attrs = append(attrs,
				attribute.String("langfuse.observation.level", otelObservationError),
				attribute.String("langfuse.observation.status_message", errMessage),
			)
		}
		span.SetAttributes(observability.SanitizeAttributes(attrs)...)
		return
	}

	span.SetStatus(codes.Ok, "")
	if llmLangfuseAttributesEnabled() {
		span.SetAttributes(observability.SanitizeAttributes([]attribute.KeyValue{
			attribute.String("langfuse.observation.level", otelObservationDefault),
		})...)
	}
}

func baseLLMAttributes(payload llmTracePayload) []attribute.KeyValue {
	bc := payload.Billing
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", payload.Operation),
		attribute.String("gen_ai.system", bc.ProviderName),
		attribute.String("gen_ai.request.model", bc.ModelName),
		attribute.String("gen_ai.response.model", bc.ModelName),
		attribute.String("zgi.request_id", bc.RequestID),
		attribute.String("zgi.gateway_request_id", bc.RequestID),
		attribute.String("zgi.attempt_id", bc.AttemptID),
		attribute.String("zgi.organization_id", bc.OrganizationID),
		attribute.String("zgi.api_key_id", bc.APIKeyID),
		attribute.String("zgi.provider", bc.ProviderName),
		attribute.String("zgi.provider_id", bc.ProviderID.String()),
		attribute.String("zgi.model", bc.ModelName),
		attribute.String("zgi.model_id", bc.ModelID.String()),
		attribute.Bool("zgi.use_system_provider", bc.UseSystemProvider),
		attribute.Int64("zgi.response_time_ms", bc.ResponseTime),
		attribute.Int64("zgi.estimated_credits", bc.EstimatedCredits),
		attribute.Int64("zgi.actual_credits", bc.ActualCredits),
	}

	if llmLangfuseAttributesEnabled() {
		attrs = append(attrs, llmLangfuseAttributes(bc, payload.Name)...)
		attrs = append(attrs,
			attribute.String("langfuse.observation.type", otelObservationGeneration),
			attribute.String("langfuse.observation.model.name", bc.ModelName),
		)
	}

	if bc.RouteID != nil {
		attrs = append(attrs, attribute.String("zgi.route_id", bc.RouteID.String()))
	}
	if bc.ChannelID != nil {
		attrs = append(attrs, attribute.String("zgi.channel_id", bc.ChannelID.String()))
	}
	if bc.GroupID != nil {
		attrs = append(attrs, attribute.String("zgi.group_id", bc.GroupID.String()))
	}
	if bc.WorkspaceID != "" {
		attrs = append(attrs, attribute.String("zgi.workspace_id", bc.WorkspaceID))
	}
	if bc.AppID != nil {
		attrs = append(attrs, attribute.String("zgi.app_id", bc.AppID.String()))
	}
	if bc.AppType != nil {
		attrs = append(attrs, attribute.String("zgi.app_type", *bc.AppType))
	}
	if bc.AccountID != nil {
		attrs = append(attrs, attribute.String("zgi.account_id", bc.AccountID.String()))
	}
	if bc.SessionID != "" {
		attrs = append(attrs, attribute.String("zgi.session_id", bc.SessionID))
	}
	if bc.ConversationID != "" {
		attrs = append(attrs, attribute.String("zgi.conversation_id", bc.ConversationID))
	}
	if bc.WorkflowID != "" {
		attrs = append(attrs, attribute.String("zgi.workflow_id", bc.WorkflowID))
	}
	if bc.WorkflowRunID != "" {
		attrs = append(attrs, attribute.String("zgi.workflow_run_id", bc.WorkflowRunID))
	}
	if bc.NodeID != "" {
		attrs = append(attrs, attribute.String("zgi.node_id", bc.NodeID))
	}
	if bc.NodeType != "" {
		attrs = append(attrs, attribute.String("zgi.node_type", bc.NodeType))
	}

	return attrs
}

func withLLMLangfuseTraceContext(ctx context.Context, bc *BillingContext, traceName string) context.Context {
	if bc == nil {
		return ctx
	}
	return observability.WithLangfuseTraceAttributes(ctx, llmLangfuseAttributes(bc, traceName)...)
}

func llmLangfuseAttributes(bc *BillingContext, traceName string) []attribute.KeyValue {
	if bc == nil {
		return nil
	}

	attrs := observability.LangfuseRuntimeAttributes()
	attrs = append(attrs,
		attribute.String("langfuse.trace.name", traceName),
		attribute.String("langfuse.user.id", traceUserID(bc)),
		attribute.String("langfuse.trace.metadata.request_id", bc.RequestID),
		attribute.String("langfuse.trace.metadata.attempt_id", bc.AttemptID),
		attribute.String("langfuse.trace.metadata.api_key_id", bc.APIKeyID),
		attribute.String("langfuse.trace.metadata.organization_id", bc.OrganizationID),
		attribute.String("langfuse.trace.metadata.provider", bc.ProviderName),
	)

	if sessionID := traceSessionID(bc); sessionID != "" {
		attrs = append(attrs, attribute.String("langfuse.session.id", sessionID))
	}
	if bc.RouteID != nil {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.route_id", bc.RouteID.String()))
	}
	if bc.ChannelID != nil {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.channel_id", bc.ChannelID.String()))
	}
	if bc.WorkspaceID != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.workspace_id", bc.WorkspaceID))
	}
	if bc.AppID != nil {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.app_id", bc.AppID.String()))
	}
	if bc.AppType != nil {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.app_type", *bc.AppType))
	}
	if bc.ConversationID != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.conversation_id", bc.ConversationID))
	}
	if bc.WorkflowID != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.workflow_id", bc.WorkflowID))
	}
	if bc.WorkflowRunID != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.workflow_run_id", bc.WorkflowRunID))
	}
	if bc.NodeID != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.node_id", bc.NodeID))
	}
	if bc.NodeType != "" {
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.node_type", bc.NodeType))
	}

	tags := llmLangfuseTags(bc)
	if len(tags) > 0 {
		attrs = append(attrs, attribute.StringSlice("langfuse.trace.tags", tags))
	}
	return attrs
}

func llmLangfuseTags(bc *BillingContext) []string {
	tags := []string{"zgi", "llm"}
	if bc == nil {
		return tags
	}
	if bc.ProviderName != "" {
		tags = append(tags, "provider:"+bc.ProviderName)
	}
	if bc.ModelName != "" {
		tags = append(tags, "model:"+bc.ModelName)
	}
	if bc.AppType != nil && *bc.AppType != "" {
		tags = append(tags, "app_type:"+*bc.AppType)
	}
	if bc.NodeType != "" {
		tags = append(tags, "node_type:"+bc.NodeType)
	}
	if bc.IsStreaming {
		tags = append(tags, "streaming")
	}
	return tags
}

func payloadAttributes(payload llmTracePayload) []attribute.KeyValue {
	usage := traceUsage(payload)
	attrs := []attribute.KeyValue{
		attribute.Int("gen_ai.usage.input_tokens", usage.PromptTokens),
		attribute.Int("gen_ai.usage.output_tokens", usage.CompletionTokens),
		attribute.Int("gen_ai.usage.total_tokens", usage.TotalTokens),
	}

	if !llmLangfuseAttributesEnabled() {
		return attrs
	}

	if input := traceContentJSONString(payload.Input); input != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.input", input))
	}
	if output := traceContentJSONString(payload.Output); output != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.output", output))
	}
	if params := safeJSONString(payload.ModelParameters); params != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.model.parameters", params))
	}
	if details := usageDetails(usage); details != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.usage_details", details))
	}
	if cost := costDetails(payload.Billing); cost != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.cost_details", cost))
	}

	return attrs
}

func traceUsage(payload llmTracePayload) llmTraceUsage {
	if payload.Usage != nil {
		return *payload.Usage
	}
	return llmTraceUsage{
		PromptTokens:     payload.Billing.PromptTokens,
		CompletionTokens: payload.Billing.CompletionTokens,
		TotalTokens:      payload.Billing.TotalTokens,
	}
}

func traceUserID(bc *BillingContext) string {
	if bc.AccountID != nil {
		return bc.AccountID.String()
	}
	if bc.APIKeyID != "" {
		return bc.APIKeyID
	}
	return bc.OrganizationID
}

func traceSessionID(bc *BillingContext) string {
	if bc == nil {
		return ""
	}
	if strings.TrimSpace(bc.SessionID) != "" {
		return strings.TrimSpace(bc.SessionID)
	}
	return strings.TrimSpace(bc.ConversationID)
}

func usageDetails(usage llmTraceUsage) string {
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return ""
	}
	return safeJSONString(map[string]int{
		"input":  usage.PromptTokens,
		"output": usage.CompletionTokens,
		"total":  usage.TotalTokens,
	})
}

func costDetails(bc *BillingContext) string {
	if !bc.TotalUSD.IsPositive() {
		return ""
	}
	return safeJSONString(map[string]float64{
		"input":  bc.InputUSD.InexactFloat64(),
		"output": bc.OutputUSD.InexactFloat64(),
		"total":  bc.TotalUSD.InexactFloat64(),
	})
}

func llmLangfuseAttributesEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.LLMLangfuseAttributes
}

func otelConfig() config.OpenTelemetryConfig {
	if config.GlobalConfig == nil {
		return config.OpenTelemetryConfig{}
	}
	return config.GlobalConfig.OpenTelemetry
}

func traceContentJSONString(value interface{}) string {
	switch llmCaptureContentMode() {
	case otelLLMCaptureNone:
		return ""
	case otelLLMCaptureFull:
		return safeJSONString(value)
	default:
		return safeJSONString(contentSummary(value))
	}
}

func llmCaptureContentMode() string {
	mode := strings.ToLower(strings.TrimSpace(otelConfig().LLMCaptureContent))
	switch mode {
	case otelLLMCaptureNone, otelLLMCaptureSummary, otelLLMCaptureFull:
		return mode
	default:
		return otelLLMCaptureSummary
	}
}

func otelPayloadMaxCharacters() int {
	maxChars := otelConfig().LLMCaptureMaxChars
	if maxChars < 0 {
		return defaultOTELPayloadCharacters
	}
	return maxChars
}

func safeJSONString(value interface{}) string {
	if value == nil {
		return ""
	}

	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(fmt.Sprintf("%v", value))
	}
	return truncateString(observability.SanitizeString(string(data)), otelPayloadMaxCharacters())
}

func truncateString(value string, maxLen int) string {
	value = observability.SanitizeString(value)
	if maxLen == 0 {
		return value
	}
	if len(value) <= maxLen {
		return value
	}

	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen]) + "...[truncated]"
}

func contentSummary(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []adapter.Message:
		return messageSummary(typed)
	case adapter.Message:
		return singleMessageSummary(typed)
	case map[string]interface{}:
		return mapContentSummary(typed)
	default:
		return genericContentSummary(value)
	}
}

func chatInput(req *adapter.ChatRequest) interface{} {
	if req == nil {
		return nil
	}
	return req.Messages
}

func messageSummary(messages []adapter.Message) map[string]interface{} {
	roles := make(map[string]int)
	contentChars := 0
	toolCallCount := 0
	for _, message := range messages {
		if message.Role != "" {
			roles[message.Role]++
		}
		contentChars += estimatedJSONLength(message.Content)
		toolCallCount += len(message.ToolCalls)
	}
	return map[string]interface{}{
		"type":          "chat_messages",
		"message_count": len(messages),
		"roles":         roles,
		"content_chars": contentChars,
		"tool_calls":    toolCallCount,
	}
}

func singleMessageSummary(message adapter.Message) map[string]interface{} {
	return map[string]interface{}{
		"type":          "chat_message",
		"role":          message.Role,
		"content_chars": estimatedJSONLength(message.Content),
		"tool_calls":    len(message.ToolCalls),
	}
}

func mapContentSummary(value map[string]interface{}) map[string]interface{} {
	summary := make(map[string]interface{}, len(value)+2)
	for key, item := range value {
		if shouldKeepSummaryValue(key, item) {
			summary[key] = item
			continue
		}
		summary[key+"_chars"] = estimatedJSONLength(item)
	}
	summary["json_chars"] = estimatedJSONLength(value)
	return summary
}

func shouldKeepSummaryValue(key string, value interface{}) bool {
	switch key {
	case "role", "status", "object", "model", "type":
	default:
		return false
	}
	switch value.(type) {
	case string, bool, int, int64, float64:
		return true
	default:
		return false
	}
}

func genericContentSummary(value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":       reflect.TypeOf(value).String(),
		"json_chars": estimatedJSONLength(value),
	}
}

func estimatedJSONLength(value interface{}) int {
	if value == nil {
		return 0
	}
	data, err := json.Marshal(value)
	if err != nil {
		return len(fmt.Sprintf("%v", value))
	}
	return len(data)
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func chatCompletionOutput(resp *adapter.ChatResponse) interface{} {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	return resp.Choices[0].Message
}

func chatModelParameters(req *adapter.ChatRequest) map[string]interface{} {
	params := map[string]interface{}{}
	if req == nil {
		return params
	}
	if req.Temperature != nil {
		params["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		params["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		params["max_tokens"] = *req.MaxTokens
	}
	if req.PresencePenalty != nil {
		params["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		params["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.Stream {
		params["stream"] = true
	}
	if req.StreamOptions != nil {
		params["stream_options.include_usage"] = req.StreamOptions.IncludeUsage
	}
	if len(req.Stop) > 0 {
		params["stop_count"] = len(req.Stop)
	}
	if len(req.Functions) > 0 {
		params["function_count"] = len(req.Functions)
	}
	if req.FunctionCall != nil {
		params["function_call"] = req.FunctionCall
	}
	if len(req.Tools) > 0 {
		params["tool_count"] = len(req.Tools)
	}
	if req.ToolChoice != nil {
		params["tool_choice"] = req.ToolChoice
	}
	if req.ResponseFormat != nil {
		params["response_format"] = req.ResponseFormat
	}
	if req.Seed != nil {
		params["seed"] = *req.Seed
	}
	if req.N != nil {
		params["n"] = *req.N
	}
	if len(req.LogitBias) > 0 {
		params["logit_bias_count"] = len(req.LogitBias)
	}
	if len(req.AdditionalParameters) > 0 {
		params["additional_parameters"] = req.AdditionalParameters
	}
	return params
}

func responseInput(req *adapter.CreateResponseRequest) interface{} {
	if req == nil {
		return nil
	}
	return map[string]interface{}{
		"input":        req.Input,
		"messages":     req.Messages,
		"instructions": req.Instructions,
	}
}

func responseOutput(resp *adapter.CreateResponseResponse) interface{} {
	if resp == nil {
		return nil
	}
	return map[string]interface{}{
		"output":  resp.Output,
		"choices": resp.Choices,
		"status":  resp.Status,
	}
}

func responseModelParameters(req *adapter.CreateResponseRequest) map[string]interface{} {
	params := map[string]interface{}{}
	if req == nil {
		return params
	}
	if req.Temperature != nil {
		params["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		params["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		params["max_tokens"] = *req.MaxTokens
	}
	if req.MaxOutputTokens != nil {
		params["max_output_tokens"] = *req.MaxOutputTokens
	}
	if req.Stream {
		params["stream"] = true
	}
	if len(req.Tools) > 0 {
		params["tool_count"] = len(req.Tools)
	}
	if req.ToolChoice != nil {
		params["tool_choice"] = req.ToolChoice
	}
	if req.ResponseFormat != nil {
		params["response_format"] = req.ResponseFormat
	}
	if len(req.Metadata) > 0 {
		params["metadata_keys"] = sortedMapKeys(req.Metadata)
	}
	if len(req.Modalities) > 0 {
		params["modalities"] = req.Modalities
	}
	if len(req.AdditionalParameters) > 0 {
		params["additional_parameters"] = req.AdditionalParameters
	}
	return params
}

func embeddingInputSummary(req *adapter.EmbeddingsRequest) interface{} {
	if req == nil {
		return nil
	}
	return map[string]interface{}{
		"input_count": countCollection(req.Input),
		"input_type":  req.InputType,
	}
}

func embeddingOutputSummary(resp *adapter.EmbeddingsResponse) interface{} {
	if resp == nil {
		return nil
	}
	dimensions := 0
	if len(resp.Data) > 0 {
		dimensions = len(resp.Data[0].Embedding)
	}
	return map[string]interface{}{
		"object":          resp.Object,
		"embedding_count": len(resp.Data),
		"dimensions":      dimensions,
		"model":           resp.Model,
	}
}

func embeddingModelParameters(req *adapter.EmbeddingsRequest) map[string]interface{} {
	params := map[string]interface{}{}
	if req == nil {
		return params
	}
	if req.EncodingFormat != "" {
		params["encoding_format"] = req.EncodingFormat
	}
	if req.Dimensions > 0 {
		params["dimensions"] = req.Dimensions
	}
	if req.Truncate != "" {
		params["truncate"] = req.Truncate
	}
	if req.MaxTokens > 0 {
		params["max_tokens"] = req.MaxTokens
	}
	return params
}

func rerankInputSummary(req *adapter.RerankRequest) interface{} {
	if req == nil {
		return nil
	}
	return map[string]interface{}{
		"query":          req.Query,
		"document_count": countCollection(req.Documents),
	}
}

func rerankOutputSummary(resp *adapter.RerankResponse) interface{} {
	if resp == nil {
		return nil
	}
	limit := len(resp.Results)
	if limit > maxRerankTraceResults {
		limit = maxRerankTraceResults
	}

	results := make([]map[string]interface{}, 0, limit)
	for _, result := range resp.Results[:limit] {
		results = append(results, map[string]interface{}{
			"index":           result.Index,
			"relevance_score": result.RelevanceScore,
		})
	}

	return map[string]interface{}{
		"id":           resp.ID,
		"object":       resp.Object,
		"model":        resp.Model,
		"result_count": len(resp.Results),
		"top_results":  results,
	}
}

func rerankModelParameters(req *adapter.RerankRequest) map[string]interface{} {
	params := map[string]interface{}{}
	if req == nil {
		return params
	}
	if req.TopN != nil {
		params["top_n"] = *req.TopN
	}
	if req.ScoreThreshold != nil {
		params["score_threshold"] = *req.ScoreThreshold
	}
	if req.MaxTokensPerDoc != nil {
		params["max_tokens_per_doc"] = *req.MaxTokensPerDoc
	}
	if req.ReturnDocuments != nil {
		params["return_documents"] = *req.ReturnDocuments
	}
	if len(req.RankFields) > 0 {
		params["rank_fields"] = req.RankFields
	}
	return params
}

func imageInputSummary(req *adapter.ImageRequest) interface{} {
	if req == nil {
		return nil
	}
	return map[string]interface{}{
		"prompt": req.Prompt,
		"model":  req.Model,
	}
}

func imageOutputSummary(resp *adapter.ImageResponse) interface{} {
	if resp == nil {
		return nil
	}

	revisedPrompts := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		if item.RevisedPrompt != "" {
			revisedPrompts = append(revisedPrompts, item.RevisedPrompt)
		}
	}

	return map[string]interface{}{
		"created":              resp.Created,
		"image_count":          len(resp.Data),
		"revised_prompt_count": len(revisedPrompts),
		"revised_prompts":      revisedPrompts,
	}
}

func imageModelParameters(req *adapter.ImageRequest) map[string]interface{} {
	params := map[string]interface{}{}
	if req == nil {
		return params
	}
	if req.N != nil {
		params["n"] = *req.N
	}
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	if req.Style != "" {
		params["style"] = req.Style
	}
	if req.ResponseFormat != "" {
		params["response_format"] = req.ResponseFormat
	}
	return params
}

func countCollection(value interface{}) int {
	if value == nil {
		return 0
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		return v.Len()
	}
	return 1
}
