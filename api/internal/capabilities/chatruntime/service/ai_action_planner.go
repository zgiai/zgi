package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	aiChatActionPlannerMinConfidence = 0.65
	aiChatActionPlannerMaxTokens     = 600
)

type aiChatActionPlannerLLM interface {
	AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error)
}

// AIChatActionPlanner asks an LLM to decide whether an AIChat turn should run a runtime action.
type AIChatActionPlanner struct {
	llm aiChatActionPlannerLLM
}

// AIChatActionPlanRequest is the bounded state the planner may use for a turn.
type AIChatActionPlanRequest struct {
	Query            string
	RuntimeContext   string
	OperationContext map[string]interface{}
	Capabilities     []AIChatActionCapability
	ModelName        string
	Provider         string
	AppContext       *llmclient.AppContext
}

// AIChatActionCapability describes an action available to the current AIChat turn.
type AIChatActionCapability struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name,omitempty"`
	Description          string   `json:"description,omitempty"`
	Domain               string   `json:"domain,omitempty"`
	Action               string   `json:"action,omitempty"`
	RiskLevel            string   `json:"risk_level,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	ResourceTypes        []string `json:"resource_types,omitempty"`
}

// AIChatActionDecision is the structured action decision returned by the planner.
type AIChatActionDecision struct {
	Matched      bool                      `json:"matched"`
	Confidence   *float64                  `json:"confidence,omitempty"`
	CapabilityID string                    `json:"capability_id,omitempty"`
	Intent       string                    `json:"intent,omitempty"`
	ResourceRefs []AIChatActionResourceRef `json:"resource_refs,omitempty"`
	Postprocess  []AIChatActionPostprocess `json:"postprocess,omitempty"`
	Reason       string                    `json:"reason,omitempty"`
}

// AIChatActionResourceRef is the planner-facing alias for the resolver contract.
type AIChatActionResourceRef = PlannerResourceRef

// AIChatActionPostprocess describes a post-action transformation requested by the user.
type AIChatActionPostprocess struct {
	Type           string                 `json:"type"`
	TargetLanguage string                 `json:"target_language,omitempty"`
	Arguments      map[string]interface{} `json:"arguments,omitempty"`
}

func newAIChatActionPlanner(llm aiChatActionPlannerLLM) *AIChatActionPlanner {
	return &AIChatActionPlanner{llm: llm}
}

func (p *AIChatActionPlanner) Plan(ctx context.Context, req AIChatActionPlanRequest) AIChatActionDecision {
	if p == nil || p.llm == nil {
		return noAIChatActionMatch("planner llm unavailable")
	}
	req = normalizeAIChatActionPlanRequest(req)
	if strings.TrimSpace(req.Query) == "" {
		return noAIChatActionMatch("query is empty")
	}
	if strings.TrimSpace(req.ModelName) == "" {
		return noAIChatActionMatch("planner model is empty")
	}
	if len(req.Capabilities) == 0 {
		return noAIChatActionMatch("no action capabilities available")
	}
	llmReq, err := newAIChatActionPlannerRequest(req)
	if err != nil {
		return noAIChatActionMatch("planner request invalid: " + err.Error())
	}
	resp, err := p.llm.AppChat(ctx, req.AppContext, llmReq)
	if err != nil {
		return noAIChatActionMatch("planner llm failed: " + err.Error())
	}
	decision, err := parseAIChatActionDecision(aiChatActionPlannerResponseText(resp), req.Capabilities)
	if err != nil {
		return noAIChatActionMatch("planner decision invalid: " + err.Error())
	}
	return decision
}

func (s *service) planAIChatActionDecision(ctx context.Context, prepared *PreparedChat) AIChatActionDecision {
	if s == nil || prepared == nil || prepared.parts == nil || prepared.Message == nil || prepared.Conversation == nil {
		return noAIChatActionMatch("prepared chat is incomplete")
	}
	return newAIChatActionPlanner(s.llmClient).Plan(ctx, aiChatActionPlanRequestFromPrepared(prepared))
}

func aiChatActionPlanRequestFromPrepared(prepared *PreparedChat) AIChatActionPlanRequest {
	if prepared == nil || prepared.parts == nil {
		return AIChatActionPlanRequest{}
	}
	operationContext := copyStringAnyMap(prepared.parts.RawOperationContext)
	if operationContext == nil {
		operationContext = copyStringAnyMap(prepared.parts.OperationContext)
	}
	return AIChatActionPlanRequest{
		Query:            prepared.parts.Query,
		RuntimeContext:   prepared.parts.RuntimeContext,
		OperationContext: operationContext,
		Capabilities:     aiChatActionCapabilitiesFromParts(prepared.parts),
		ModelName:        prepared.parts.ModelName,
		Provider:         prepared.parts.Provider,
		AppContext:       newBillingAppContext(prepared),
	}
}

func normalizeAIChatActionPlanRequest(req AIChatActionPlanRequest) AIChatActionPlanRequest {
	req.Query = strings.TrimSpace(req.Query)
	req.RuntimeContext = strings.TrimSpace(req.RuntimeContext)
	req.ModelName = strings.TrimSpace(req.ModelName)
	req.Provider = strings.TrimSpace(req.Provider)
	req.OperationContext = copyStringAnyMap(req.OperationContext)
	req.Capabilities = normalizeAIChatActionCapabilities(req.Capabilities)
	return req
}

func newAIChatActionPlannerRequest(req AIChatActionPlanRequest) (*adapter.ChatRequest, error) {
	payload := map[string]interface{}{
		"query":             req.Query,
		"runtime_context":   req.RuntimeContext,
		"operation_context": req.OperationContext,
		"capabilities":      req.Capabilities,
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal planner payload: %w", err)
	}
	temperature := 0.0
	maxTokens := aiChatActionPlannerMaxTokens
	return &adapter.ChatRequest{
		Provider: req.Provider,
		Model:    req.ModelName,
		Messages: []adapter.Message{
			{Role: "system", Content: aiChatActionPlannerSystemPrompt()},
			{Role: "user", Content: "Plan one AIChat action decision from this JSON payload:\n" + string(rawPayload)},
		},
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}, nil
}

func aiChatActionPlannerSystemPrompt() string {
	return strings.Join([]string{
		"You are the internal AIChat action planner.",
		"Decide whether the latest user query should run exactly one available action capability before the final assistant answer.",
		"Use the query, runtime_context, operation_context, and capability descriptions semantically. Do not rely on keyword matching.",
		"Return exactly one JSON object and no prose.",
		`Schema: {"matched":true|false,"confidence":0.0,"capability_id":"available capability id or empty","intent":"short stable intent","resource_refs":[{"type":"file","id":"exact resource id when known","file_id":"exact file id when known","selector":"visible_files[4] when referring to visible order","visible_index":4,"ordinal":2,"ordinal_text":"last","name":"optional exact/fuzzy name","title_contains":"optional contained title text","extension":"pdf","file_type":"excel"}],"postprocess":[{"type":"translate","target_language":"optional language"},{"type":"summarize"}],"reason":"short internal reason"}`,
		"Set matched=true only when one listed capability clearly satisfies the user's requested action.",
		"Never invent capability IDs or resources. If the capability or resource is unclear, set matched=false.",
		"For file.read, plan only reading content from files represented in operation_context.",
		`For file.read references on a visible files page: "the fourth file" => {"type":"file","visible_index":4}; "the second Excel file" => {"type":"file","visible_index":2,"file_type":"excel"} when the second visible row is Excel; "the last PDF" => {"type":"file","extension":"pdf","ordinal_text":"last"}.`,
		"For Chinese file.read references on a visible files page: \"\u7b2c\u56db\u4e2a\u6587\u4ef6\" => {\"type\":\"file\",\"visible_index\":4}; \"\u7b2c\u4e8c\u4e2a Excel \u6587\u4ef6\" => {\"type\":\"file\",\"visible_index\":2,\"file_type\":\"excel\"} when the second visible row is Excel; \"\u6700\u540e\u4e00\u4e2a PDF\" => {\"type\":\"file\",\"extension\":\"pdf\",\"ordinal_text\":\"last\"}.",
		"If the user asks to translate, summarize, explain, or extract the read content after reading, keep file.read as the capability and preserve those operations as postprocess.",
		"Use confidence from 0 to 1. A confident executable decision should be at least 0.65.",
	}, "\n")
}

func parseAIChatActionDecision(raw string, capabilities []AIChatActionCapability) (AIChatActionDecision, error) {
	raw = strings.TrimSpace(stripAIChatActionJSONCodeFence(raw))
	if raw == "" {
		return AIChatActionDecision{}, fmt.Errorf("empty decision")
	}
	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}
	var payload struct {
		Matched      bool                      `json:"matched"`
		Confidence   *float64                  `json:"confidence"`
		CapabilityID string                    `json:"capability_id"`
		Intent       string                    `json:"intent"`
		ResourceRefs []AIChatActionResourceRef `json:"resource_refs"`
		Postprocess  json.RawMessage           `json:"postprocess"`
		Reason       string                    `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return AIChatActionDecision{}, fmt.Errorf("parse decision json: %w", err)
	}
	postprocess, err := parseAIChatActionPostprocess(payload.Postprocess)
	if err != nil {
		return AIChatActionDecision{}, err
	}
	decision := AIChatActionDecision{
		Matched:      payload.Matched,
		Confidence:   copyFloat64Ptr(payload.Confidence),
		CapabilityID: strings.TrimSpace(payload.CapabilityID),
		Intent:       strings.TrimSpace(payload.Intent),
		ResourceRefs: normalizeAIChatActionResourceRefs(payload.ResourceRefs),
		Postprocess:  postprocess,
		Reason:       strings.TrimSpace(payload.Reason),
	}
	if !decision.Matched {
		return decision, nil
	}
	if decision.Confidence == nil {
		return noAIChatActionMatch("planner confidence missing"), nil
	}
	if *decision.Confidence < 0 || *decision.Confidence > 1 {
		return noAIChatActionMatch("planner confidence out of range"), nil
	}
	if *decision.Confidence < aiChatActionPlannerMinConfidence {
		return noAIChatActionMatch("planner confidence below threshold"), nil
	}
	capabilityID, ok := knownAIChatActionCapabilityID(decision.CapabilityID, capabilities)
	if !ok {
		return noAIChatActionMatch("planner capability unavailable"), nil
	}
	decision.CapabilityID = capabilityID
	return decision, nil
}

func stripAIChatActionJSONCodeFence(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func parseAIChatActionPostprocess(raw json.RawMessage) ([]AIChatActionPostprocess, error) {
	if len(raw) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return nil, nil
	}
	var items []AIChatActionPostprocess
	if err := json.Unmarshal(raw, &items); err == nil {
		return normalizeAIChatActionPostprocess(items), nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		items = make([]AIChatActionPostprocess, 0, len(names))
		for _, name := range names {
			items = append(items, AIChatActionPostprocess{Type: name})
		}
		return normalizeAIChatActionPostprocess(items), nil
	}
	var single AIChatActionPostprocess
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, fmt.Errorf("parse postprocess json: %w", err)
	}
	return normalizeAIChatActionPostprocess([]AIChatActionPostprocess{single}), nil
}

func normalizeAIChatActionPostprocess(input []AIChatActionPostprocess) []AIChatActionPostprocess {
	if len(input) == 0 {
		return nil
	}
	out := make([]AIChatActionPostprocess, 0, len(input))
	for _, item := range input {
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		item.TargetLanguage = strings.TrimSpace(item.TargetLanguage)
		if item.Arguments != nil {
			item.Arguments = copyStringAnyMap(item.Arguments)
		}
		if item.Type == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeAIChatActionResourceRefs(input []AIChatActionResourceRef) []AIChatActionResourceRef {
	if len(input) == 0 {
		return nil
	}
	out := make([]AIChatActionResourceRef, 0, len(input))
	for _, ref := range input {
		ref.ResourceType = normalizeResourceKind(ref.ResourceType)
		ref.Type = strings.ToLower(strings.TrimSpace(ref.Type))
		ref.Kind = normalizeResourceKind(ref.Kind)
		ref.ID = strings.TrimSpace(ref.ID)
		ref.FileID = strings.TrimSpace(ref.FileID)
		ref.Name = strings.TrimSpace(ref.Name)
		ref.Source = strings.TrimSpace(ref.Source)
		ref.Selector = strings.TrimSpace(ref.Selector)
		ref.Scope = strings.TrimSpace(ref.Scope)
		ref.OrdinalText = strings.TrimSpace(ref.OrdinalText)
		ref.Title = strings.TrimSpace(ref.Title)
		ref.TitleContains = strings.TrimSpace(ref.TitleContains)
		ref.NameContains = strings.TrimSpace(ref.NameContains)
		ref.FuzzyName = strings.TrimSpace(ref.FuzzyName)
		ref.Extension = normalizedResolverFileExtension(ref.Extension)
		ref.Extensions = normalizedResolverFileExtensions(ref.Extensions)
		ref.MimeType = normalizedMimeType(ref.MimeType)
		ref.MimeTypes = normalizedMimeTypes(ref.MimeTypes)
		ref.FileType = normalizeResourceToken(ref.FileType)
		if ref.Metadata != nil {
			ref.Metadata = copyStringAnyMap(ref.Metadata)
		}
		if isEmptyAIChatActionResourceRef(ref) {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func isEmptyAIChatActionResourceRef(ref AIChatActionResourceRef) bool {
	return ref.ID == "" &&
		ref.FileID == "" &&
		ref.Name == "" &&
		ref.Title == "" &&
		ref.Selector == "" &&
		ref.Scope == "" &&
		ref.Ordinal <= 0 &&
		ref.VisibleIndex <= 0 &&
		ref.OrdinalText == "" &&
		ref.TitleContains == "" &&
		ref.NameContains == "" &&
		ref.FuzzyName == "" &&
		ref.Extension == "" &&
		len(ref.Extensions) == 0 &&
		ref.MimeType == "" &&
		len(ref.MimeTypes) == 0 &&
		ref.FileType == "" &&
		!ref.Selected &&
		len(ref.Metadata) == 0
}

func aiChatActionPlannerResponseText(resp *adapter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(messageContentText(resp.Choices[0].Message.Content))
}

func knownAIChatActionCapabilityID(raw string, capabilities []AIChatActionCapability) (string, bool) {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return "", false
	}
	for _, capability := range capabilities {
		id := strings.TrimSpace(capability.ID)
		if strings.EqualFold(id, needle) {
			return id, true
		}
	}
	return "", false
}

func noAIChatActionMatch(reason string) AIChatActionDecision {
	return AIChatActionDecision{Matched: false, Reason: strings.TrimSpace(reason)}
}

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func aiChatActionCapabilitiesFromParts(parts *chatRequestParts) []AIChatActionCapability {
	if parts == nil {
		return nil
	}
	collector := newAIChatActionCapabilityCollector()
	collector.addFromContext(parts.RawOperationContext)
	collector.addFromContext(parts.OperationContext)
	return collector.values()
}

type aiChatActionCapabilityCollector struct {
	seen map[string]int
	out  []AIChatActionCapability
}

func newAIChatActionCapabilityCollector() *aiChatActionCapabilityCollector {
	return &aiChatActionCapabilityCollector{seen: map[string]int{}}
}

func (c *aiChatActionCapabilityCollector) addFromContext(context map[string]interface{}) {
	if c == nil || len(context) == 0 {
		return
	}
	for _, item := range operationItemsFromKeys(context, []string{"capabilities", "capability"}) {
		if capability, ok := aiChatActionCapabilityFromItem(item); ok {
			c.add(capability)
		}
	}
	for _, item := range operationItemsFromKeys(context, []string{"resources", "resource"}) {
		resource, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, id := range stringMetadataSlice(firstMapValue(resource, "capability_id", "capability_ids", "tool_id", "tool_ids")) {
			c.add(AIChatActionCapability{ID: id})
		}
	}
	for _, id := range stringMetadataSlice(firstMapValue(context, "capability_id", "capability_ids", "tool_id", "tool_ids")) {
		c.add(AIChatActionCapability{ID: id})
	}
}

func (c *aiChatActionCapabilityCollector) add(capability AIChatActionCapability) {
	if c == nil {
		return
	}
	capability = withAIChatActionCapabilityDefaults(capability)
	key := strings.ToLower(strings.TrimSpace(capability.ID))
	if key == "" {
		return
	}
	if idx, ok := c.seen[key]; ok {
		c.out[idx] = mergeAIChatActionCapability(c.out[idx], capability)
		return
	}
	c.seen[key] = len(c.out)
	c.out = append(c.out, capability)
}

func (c *aiChatActionCapabilityCollector) values() []AIChatActionCapability {
	if c == nil || len(c.out) == 0 {
		return nil
	}
	return normalizeAIChatActionCapabilities(c.out)
}

func aiChatActionCapabilityFromItem(item interface{}) (AIChatActionCapability, bool) {
	switch typed := item.(type) {
	case string:
		id := strings.TrimSpace(typed)
		return AIChatActionCapability{ID: id}, id != ""
	case map[string]interface{}:
		capability := AIChatActionCapability{
			ID:                   stringMetadataValue(firstMapValue(typed, "id", "capability_id", "tool_id")),
			Name:                 stringMetadataValue(firstMapValue(typed, "name", "tool_name", "label")),
			Description:          stringMetadataValue(firstMapValue(typed, "description", "summary")),
			Domain:               stringMetadataValue(typed["domain"]),
			Action:               stringMetadataValue(typed["action"]),
			RiskLevel:            stringMetadataValue(firstMapValue(typed, "risk_level", "risk")),
			RequiresConfirmation: boolMetadataValue(firstMapValue(typed, "requires_confirmation", "requires_approval")),
			ResourceTypes:        stringMetadataSlice(firstMapValue(typed, "resource_types", "allowed_resources", "resources")),
		}
		return capability, strings.TrimSpace(capability.ID) != ""
	default:
		return AIChatActionCapability{}, false
	}
}

func normalizeAIChatActionCapabilities(input []AIChatActionCapability) []AIChatActionCapability {
	if len(input) == 0 {
		return nil
	}
	collector := newAIChatActionCapabilityCollector()
	for _, capability := range input {
		collector.add(capability)
	}
	if collector == nil || len(collector.out) == 0 {
		return nil
	}
	return append([]AIChatActionCapability(nil), collector.out...)
}

func withAIChatActionCapabilityDefaults(capability AIChatActionCapability) AIChatActionCapability {
	capability.ID = strings.TrimSpace(capability.ID)
	capability.Name = strings.TrimSpace(capability.Name)
	capability.Description = strings.TrimSpace(capability.Description)
	capability.Domain = strings.TrimSpace(capability.Domain)
	capability.Action = strings.TrimSpace(capability.Action)
	capability.RiskLevel = strings.TrimSpace(capability.RiskLevel)
	capability.ResourceTypes = normalizedStringList(capability.ResourceTypes)
	if strings.EqualFold(capability.ID, consoleFilesActionCapabilityID) {
		if capability.Name == "" {
			capability.Name = "Read file"
		}
		if capability.Description == "" {
			capability.Description = "Read the content of selected or visible files for the current AIChat turn."
		}
		if capability.Domain == "" {
			capability.Domain = "file"
		}
		if capability.Action == "" {
			capability.Action = "read"
		}
		if capability.RiskLevel == "" {
			capability.RiskLevel = "low"
		}
		if len(capability.ResourceTypes) == 0 {
			capability.ResourceTypes = []string{"file"}
		}
	}
	return capability
}

func mergeAIChatActionCapability(current, next AIChatActionCapability) AIChatActionCapability {
	if strings.TrimSpace(current.ID) == "" {
		current.ID = next.ID
	}
	if strings.TrimSpace(current.Name) == "" {
		current.Name = next.Name
	}
	if strings.TrimSpace(current.Description) == "" {
		current.Description = next.Description
	}
	if strings.TrimSpace(current.Domain) == "" {
		current.Domain = next.Domain
	}
	if strings.TrimSpace(current.Action) == "" {
		current.Action = next.Action
	}
	if strings.TrimSpace(current.RiskLevel) == "" {
		current.RiskLevel = next.RiskLevel
	}
	if !current.RequiresConfirmation {
		current.RequiresConfirmation = next.RequiresConfirmation
	}
	if len(current.ResourceTypes) == 0 {
		current.ResourceTypes = append([]string(nil), next.ResourceTypes...)
	}
	return withAIChatActionCapabilityDefaults(current)
}

func normalizedStringList(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
