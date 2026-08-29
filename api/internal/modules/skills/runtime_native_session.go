package skills

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	NativeSkillProtocolProgressiveV1 = "progressive_v1"
	NativeSkillProtocolPreloadV1     = "native_preload_v1"

	DefaultNativeSkillCatalogBudgetChars = 8000
	DefaultNativeSkillSearchLimit        = 5
	MaxNativeSkillSearchLimit            = 10
	MaxNativeSkillActivationBatch        = 3
)

// NativeSkillCatalog is the lightweight candidate directory exposed before a
// skill's complete instructions and business tools are activated.
type NativeSkillCatalog struct {
	CandidateSkillIDs []string              `json:"candidate_skill_ids"`
	ExposedSkillIDs   []string              `json:"exposed_candidate_skill_ids"`
	OmittedCount      int                   `json:"omitted_candidate_count"`
	MetadataTokens    int                   `json:"metadata_tokens"`
	Message           llmadapter.Message    `json:"-"`
	Metadata          []SkillPromptMetadata `json:"-"`
}

type NativeSkillActivationAttempt struct {
	SkillID string `json:"skill_id"`
	Source  string `json:"source"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type NativeSkillActivationResult struct {
	ActivatedSkillIDs     []string
	AlreadyActiveSkillIDs []string
	SkippedSkills         []NativeSkillSkip
	InstructionMessages   []llmadapter.Message
	ProviderTools         []llmadapter.Tool
}

// NativeSkillSession owns the progressively expanding native tool set for one
// model turn. It is request-scoped and must not be shared between goroutines.
type NativeSkillSession struct {
	runtime    *Runtime
	resolved   *ResolvedSkills
	options    NativeToolSetOptions
	catalog    NativeSkillCatalog
	active     NativeToolSet
	activeSet  map[string]struct{}
	exposedSet map[string]struct{}
	attempts   []NativeSkillActivationAttempt
}

// BuildNativeSkillCatalog creates a compact candidate directory. maxTokens is
// enforced with estimateMessageTokens when the model context limit is known.
func BuildNativeSkillCatalog(
	resolved *ResolvedSkills,
	prioritySkillIDs []string,
	maxChars int,
	maxTokens int,
	estimateMessageTokens func(llmadapter.Message) int,
) NativeSkillCatalog {
	if maxChars <= 0 || maxChars > DefaultNativeSkillCatalogBudgetChars {
		maxChars = DefaultNativeSkillCatalogBudgetChars
	}
	catalog := NativeSkillCatalog{}
	if resolved == nil {
		catalog.Message = nativeSkillCatalogMessage(nil, 0, 0)
		return catalog
	}
	docs := orderedNativeSkillCatalogDocuments(resolved.Skills, prioritySkillIDs)
	for _, doc := range docs {
		if skillID := normalizeSkillID(doc.Metadata.ID); skillID != "" {
			catalog.CandidateSkillIDs = append(catalog.CandidateSkillIDs, skillID)
		}
	}
	exposureLimit := min(len(docs), MaxNativeSkillSearchLimit)
	if len(docs) > MaxNativeSkillSearchLimit {
		exposureLimit = DefaultNativeSkillSearchLimit
	}
	for _, doc := range docs {
		if len(catalog.Metadata) >= exposureLimit {
			break
		}
		item := skillPromptMetadata(doc)
		candidate, _ := skillPromptMetadataWithFieldBudget(item, skillMetadataLongFieldBudgetChars)
		if !nativeSkillCatalogItemFits(catalog.Metadata, candidate, len(docs), maxChars, maxTokens, estimateMessageTokens) {
			candidate, _ = skillPromptMetadataWithFieldBudget(item, skillMetadataShortFieldBudgetChars)
		}
		if !nativeSkillCatalogItemFits(catalog.Metadata, candidate, len(docs), maxChars, maxTokens, estimateMessageTokens) {
			candidate.Description = ""
			candidate.WhenToUse = ""
		}
		if !nativeSkillCatalogItemFits(catalog.Metadata, candidate, len(docs), maxChars, maxTokens, estimateMessageTokens) {
			break
		}
		catalog.Metadata = append(catalog.Metadata, candidate)
		catalog.ExposedSkillIDs = append(catalog.ExposedSkillIDs, normalizeSkillID(candidate.ID))
	}
	catalog.OmittedCount = max(0, len(catalog.CandidateSkillIDs)-len(catalog.ExposedSkillIDs))
	catalog.Message = nativeSkillCatalogMessage(catalog.Metadata, len(catalog.CandidateSkillIDs), catalog.OmittedCount)
	if estimateMessageTokens != nil {
		catalog.MetadataTokens = estimateMessageTokens(catalog.Message)
	}
	return catalog
}

func nativeSkillCatalogItemFits(
	existing []SkillPromptMetadata,
	candidate SkillPromptMetadata,
	enabledCount int,
	maxChars int,
	maxTokens int,
	estimateMessageTokens func(llmadapter.Message) int,
) bool {
	items := append(append([]SkillPromptMetadata(nil), existing...), candidate)
	omitted := max(0, enabledCount-len(items))
	message := nativeSkillCatalogMessage(items, enabledCount, omitted)
	if utf8.RuneCountInString(strings.TrimSpace(stringFromNativeMessage(message.Content))) > maxChars {
		return false
	}
	return maxTokens <= 0 || estimateMessageTokens == nil || estimateMessageTokens(message) <= maxTokens
}

func orderedNativeSkillCatalogDocuments(input []SkillDocument, priority []string) []SkillDocument {
	order := make(map[string]int, len(priority))
	for index, skillID := range priority {
		if skillID = normalizeSkillID(skillID); skillID != "" {
			if _, exists := order[skillID]; !exists {
				order[skillID] = index
			}
		}
	}
	out := append([]SkillDocument(nil), input...)
	sort.SliceStable(out, func(i, j int) bool {
		left, leftOK := order[normalizeSkillID(out[i].Metadata.ID)]
		right, rightOK := order[normalizeSkillID(out[j].Metadata.ID)]
		if leftOK != rightOK {
			return leftOK
		}
		return leftOK && left < right
	})
	return out
}

func nativeSkillCatalogMessage(metadata []SkillPromptMetadata, enabledCount int, omittedCount int) llmadapter.Message {
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte("[]")
	}
	note := ""
	if omittedCount > 0 {
		note = " Some candidates were omitted from this compact directory; use search_skills only when the needed capability is not listed."
	}
	content := "The following skills are candidates for this turn; their full instructions and business tools are not active yet. " +
		"When a listed skill is needed, call activate_skills before doing the business work. " +
		"Activate only the smallest relevant set and do not describe activation to the user." + note +
		" enabled_count=" + nativeIntString(enabledCount) +
		" exposed_count=" + nativeIntString(len(metadata)) +
		" omitted_count=" + nativeIntString(omittedCount) +
		" Candidate skills JSON: " + string(payload)
	return llmadapter.Message{Role: "system", Content: content}
}

func nativeIntString(value int) string {
	return strconv.Itoa(value)
}

func stringFromNativeMessage(value interface{}) string {
	text, _ := value.(string)
	return text
}

// NewNativeSkillSession creates a request-scoped progressive activation state.
func NewNativeSkillSession(runtime *Runtime, resolved *ResolvedSkills, catalog NativeSkillCatalog, options NativeToolSetOptions) *NativeSkillSession {
	session := &NativeSkillSession{
		runtime:    runtime,
		resolved:   resolved,
		options:    options,
		catalog:    catalog,
		activeSet:  map[string]struct{}{},
		exposedSet: map[string]struct{}{},
		active: NativeToolSet{
			ToolBindings: make(map[string]NativeToolBinding),
			BudgetChars:  options.BudgetChars,
			BudgetTokens: options.BudgetTokens,
		},
	}
	for _, skillID := range catalog.ExposedSkillIDs {
		session.exposedSet[normalizeSkillID(skillID)] = struct{}{}
	}
	return session
}

func (s *NativeSkillSession) CatalogMessage() llmadapter.Message {
	if s == nil {
		return llmadapter.Message{}
	}
	metadata := make([]SkillPromptMetadata, 0, len(s.catalog.Metadata))
	for _, item := range s.catalog.Metadata {
		if _, active := s.activeSet[normalizeSkillID(item.ID)]; active {
			continue
		}
		metadata = append(metadata, item)
	}
	inactiveCount := 0
	for _, skillID := range s.catalog.CandidateSkillIDs {
		if _, active := s.activeSet[normalizeSkillID(skillID)]; !active {
			inactiveCount++
		}
	}
	return nativeSkillCatalogMessage(metadata, inactiveCount, s.OmittedCandidateCount())
}

func (s *NativeSkillSession) CandidateSkillIDs() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.catalog.CandidateSkillIDs...)
}

func (s *NativeSkillSession) OmittedCandidateCount() int {
	if s == nil {
		return 0
	}
	count := 0
	for _, skillID := range s.catalog.CandidateSkillIDs {
		normalized := normalizeSkillID(skillID)
		if _, active := s.activeSet[normalized]; active {
			continue
		}
		if _, exposed := s.exposedSet[normalized]; !exposed {
			count++
		}
	}
	return count
}

func (s *NativeSkillSession) MetadataTokens() int {
	if s == nil {
		return 0
	}
	return s.catalog.MetadataTokens
}

// RemainingBudgetChars returns the request-scoped instruction and schema
// character budget that can still be used by later activations.
func (s *NativeSkillSession) RemainingBudgetChars() int {
	if s == nil || s.options.BudgetChars <= 0 {
		return 0
	}
	return max(0, s.options.BudgetChars-s.active.InstructionChars-s.active.SchemaChars)
}

// RemainingBudgetTokens returns the request-scoped token budget that can still
// be used by later activations. Zero also represents an unbounded token budget.
func (s *NativeSkillSession) RemainingBudgetTokens() int {
	if s == nil || s.options.BudgetTokens <= 0 {
		return 0
	}
	return max(0, s.options.BudgetTokens-s.active.InstructionTokens-s.active.SchemaTokens)
}

func (s *NativeSkillSession) ExposedSkillIDs() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.exposedSet))
	for _, skillID := range s.catalog.CandidateSkillIDs {
		normalized := normalizeSkillID(skillID)
		if _, active := s.activeSet[normalized]; active {
			continue
		}
		if _, ok := s.exposedSet[normalized]; ok {
			out = append(out, normalized)
		}
	}
	return out
}

// RankNativeSkillIDs orders candidates by explicit priority followed by a
// deterministic metadata match against the current task text.
func RankNativeSkillIDs(resolved *ResolvedSkills, query string, prioritySkillIDs []string) []string {
	if resolved == nil {
		return nil
	}
	priority := make(map[string]int, len(prioritySkillIDs))
	for index, skillID := range prioritySkillIDs {
		skillID = normalizeSkillID(skillID)
		if skillID == "" {
			continue
		}
		if _, exists := priority[skillID]; !exists {
			priority[skillID] = index
		}
	}
	type rankedSkill struct {
		id       string
		score    int
		index    int
		priority int
		pinned   bool
	}
	ranked := make([]rankedSkill, 0, len(resolved.Skills))
	for index, doc := range resolved.Skills {
		id := normalizeSkillID(doc.Metadata.ID)
		priorityIndex, pinned := priority[id]
		ranked = append(ranked, rankedSkill{id: id, score: nativeSkillSearchScore(doc, query), index: index, priority: priorityIndex, pinned: pinned})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].pinned != ranked[j].pinned {
			return ranked[i].pinned
		}
		if ranked[i].pinned && ranked[i].priority != ranked[j].priority {
			return ranked[i].priority < ranked[j].priority
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].index < ranked[j].index
	})
	out := make([]string, 0, len(ranked))
	for _, item := range ranked {
		if item.id != "" {
			out = append(out, item.id)
		}
	}
	return out
}

func (s *NativeSkillSession) SearchAvailable() bool {
	return s != nil && s.OmittedCandidateCount() > 0
}

func (s *NativeSkillSession) ActiveSkillIDs() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.active.ActiveSkillIDs...)
}

func (s *NativeSkillSession) ToolSet() NativeToolSet {
	if s == nil {
		return NativeToolSet{ToolBindings: map[string]NativeToolBinding{}}
	}
	return cloneNativeToolSet(s.active)
}

// AddToolProjections keeps request-scoped aliases inside the progressive
// session so later Skill activations preserve them and reserve their names.
func (s *NativeSkillSession) AddToolProjections(
	projections []NativeToolProjection,
	options NativeToolProjectionOptions,
) int {
	if s == nil {
		return 0
	}
	options.ReservedToolNames = append(
		append([]string(nil), s.options.ReservedToolNames...),
		options.ReservedToolNames...,
	)
	return AppendNativeToolProjections(&s.active, projections, options)
}

func (s *NativeSkillSession) ActivationAttempts() []NativeSkillActivationAttempt {
	if s == nil {
		return nil
	}
	return append([]NativeSkillActivationAttempt(nil), s.attempts...)
}

// Activate adds complete, valid skills to the session without changing any
// already active skill. Dependencies are activated before their dependents.
func (s *NativeSkillSession) Activate(ctx context.Context, requested []string, source string) NativeSkillActivationResult {
	result := NativeSkillActivationResult{}
	if s == nil || s.runtime == nil || s.resolved == nil {
		return result
	}
	candidateSet := make(map[string]struct{}, len(s.catalog.CandidateSkillIDs))
	for _, skillID := range s.catalog.CandidateSkillIDs {
		candidateSet[normalizeSkillID(skillID)] = struct{}{}
	}
	selected := make([]string, 0, len(requested))
	for _, raw := range requested {
		skillID := normalizeSkillID(raw)
		if skillID == "" {
			continue
		}
		if _, ok := candidateSet[skillID]; !ok {
			skip := NativeSkillSkip{SkillID: skillID, Reason: "not_candidate", Detail: "skill is not available for this turn"}
			result.SkippedSkills = append(result.SkippedSkills, skip)
			s.recordActivationAttempt(skillID, source, "skipped", skip.Reason, skip.Detail)
			continue
		}
		if _, ok := s.activeSet[skillID]; ok {
			result.AlreadyActiveSkillIDs = appendUniqueNativeID(result.AlreadyActiveSkillIDs, skillID)
			s.recordActivationAttempt(skillID, source, "already_active", "", "")
			continue
		}
		selected = appendUniqueNativeID(selected, nativeSkillDependencyIDs(s.resolved, skillID)...)
		selected = appendUniqueNativeID(selected, skillID)
	}
	selected = removeActiveNativeIDs(selected, s.activeSet)
	if len(selected) == 0 {
		return result
	}
	options := s.options
	options.SelectedSkillIDs = append([]string(nil), selected...)
	options.PrioritySkillIDs = append([]string(nil), selected...)
	options.AlreadyActiveSkillIDs = s.ActiveSkillIDs()
	options.ReservedToolNames = nativeToolSetNames(s.active)
	if options.BudgetChars > 0 {
		options.BudgetChars = max(1, options.BudgetChars-s.active.InstructionChars-s.active.SchemaChars)
	}
	if options.BudgetTokens > 0 {
		options.BudgetTokens = max(1, options.BudgetTokens-s.active.InstructionTokens-s.active.SchemaTokens)
	}
	increment := s.runtime.BuildNativeToolSet(ctx, s.resolved, options)
	result.ActivatedSkillIDs = append([]string(nil), increment.ActiveSkillIDs...)
	result.InstructionMessages = append([]llmadapter.Message(nil), increment.InstructionMessages...)
	result.ProviderTools = append([]llmadapter.Tool(nil), increment.ProviderTools...)
	result.SkippedSkills = append(result.SkippedSkills, increment.SkippedSkills...)
	s.mergeToolSet(increment)
	activated := make(map[string]struct{}, len(increment.ActiveSkillIDs))
	for _, skillID := range increment.ActiveSkillIDs {
		activated[normalizeSkillID(skillID)] = struct{}{}
		s.recordActivationAttempt(skillID, source, "activated", "", "")
	}
	for _, skip := range increment.SkippedSkills {
		s.recordActivationAttempt(skip.SkillID, source, "skipped", skip.Reason, skip.Detail)
	}
	for _, skillID := range selected {
		if _, ok := activated[normalizeSkillID(skillID)]; ok {
			continue
		}
		if !nativeSkipContains(increment.SkippedSkills, skillID) {
			s.recordActivationAttempt(skillID, source, "skipped", "dependency_unavailable", "a required dependency could not be activated")
		}
	}
	return result
}

// Search returns compact metadata for candidate skills and makes the returned
// IDs eligible for a later activate_skills call.
func (s *NativeSkillSession) Search(query string, limit int) []SkillPromptMetadata {
	if s == nil || s.resolved == nil {
		return nil
	}
	if limit <= 0 {
		limit = DefaultNativeSkillSearchLimit
	}
	limit = min(limit, MaxNativeSkillSearchLimit)
	type ranked struct {
		doc   SkillDocument
		score int
		index int
	}
	items := make([]ranked, 0, len(s.resolved.Skills))
	for index, doc := range s.resolved.Skills {
		skillID := normalizeSkillID(doc.Metadata.ID)
		if _, ok := s.activeSet[skillID]; ok {
			continue
		}
		if _, ok := s.exposedSet[skillID]; ok {
			continue
		}
		items = append(items, ranked{doc: doc, score: nativeSkillSearchScore(doc, query), index: index})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].index < items[j].index
	})
	out := make([]SkillPromptMetadata, 0, min(limit, len(items)))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		metadata, _ := skillPromptMetadataWithFieldBudget(skillPromptMetadata(item.doc), skillMetadataShortFieldBudgetChars)
		out = append(out, metadata)
		s.exposedSet[normalizeSkillID(metadata.ID)] = struct{}{}
	}
	return out
}

func (s *NativeSkillSession) mergeToolSet(increment NativeToolSet) {
	for _, skillID := range increment.ActiveSkillIDs {
		skillID = normalizeSkillID(skillID)
		if _, exists := s.activeSet[skillID]; exists {
			continue
		}
		s.activeSet[skillID] = struct{}{}
		s.active.ActiveSkillIDs = append(s.active.ActiveSkillIDs, skillID)
	}
	s.active.InstructionMessages = append(s.active.InstructionMessages, increment.InstructionMessages...)
	s.active.ProviderTools = append(s.active.ProviderTools, increment.ProviderTools...)
	for name, binding := range increment.ToolBindings {
		s.active.ToolBindings[name] = binding
	}
	s.active.SkippedSkills = append(s.active.SkippedSkills, increment.SkippedSkills...)
	s.active.SkippedTools = append(s.active.SkippedTools, increment.SkippedTools...)
	s.active.InstructionChars += increment.InstructionChars
	s.active.SchemaChars += increment.SchemaChars
	s.active.InstructionTokens += increment.InstructionTokens
	s.active.SchemaTokens += increment.SchemaTokens
}

func (s *NativeSkillSession) recordActivationAttempt(skillID string, source string, outcome string, reason string, detail string) {
	s.attempts = append(s.attempts, NativeSkillActivationAttempt{
		SkillID: normalizeSkillID(skillID),
		Source:  strings.TrimSpace(source),
		Outcome: strings.TrimSpace(outcome),
		Reason:  strings.TrimSpace(reason),
		Detail:  strings.TrimSpace(detail),
	})
}

func nativeSkillDependencyIDs(resolved *ResolvedSkills, skillID string) []string {
	doc, ok := resolved.Get(skillID)
	if !ok || doc == nil || !nativeSkillRequiresPromptProfessionalizer(*doc) {
		return nil
	}
	return []string{SkillPromptProfessionalizer}
}

func nativeToolSetNames(toolSet NativeToolSet) []string {
	out := make([]string, 0, len(toolSet.ProviderTools))
	for _, tool := range toolSet.ProviderTools {
		if name := strings.TrimSpace(tool.Function.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func cloneNativeToolSet(input NativeToolSet) NativeToolSet {
	out := input
	out.ActiveSkillIDs = append([]string(nil), input.ActiveSkillIDs...)
	out.InstructionMessages = append([]llmadapter.Message(nil), input.InstructionMessages...)
	out.ProviderTools = append([]llmadapter.Tool(nil), input.ProviderTools...)
	out.SkippedSkills = append([]NativeSkillSkip(nil), input.SkippedSkills...)
	out.SkippedTools = append([]NativeToolSkip(nil), input.SkippedTools...)
	out.ExternalActionIntentKeys = append([]string(nil), input.ExternalActionIntentKeys...)
	out.ExternalActionCandidates = make([]NativeExternalActionCandidate, 0, len(input.ExternalActionCandidates))
	for _, candidate := range input.ExternalActionCandidates {
		out.ExternalActionCandidates = append(out.ExternalActionCandidates, cloneNativeExternalActionCandidate(candidate))
	}
	out.ToolBindings = make(map[string]NativeToolBinding, len(input.ToolBindings))
	for name, binding := range input.ToolBindings {
		out.ToolBindings[name] = cloneNativeToolBinding(binding)
	}
	return out
}

func appendUniqueNativeID(input []string, values ...string) []string {
	seen := make(map[string]struct{}, len(input)+len(values))
	for _, value := range input {
		seen[normalizeSkillID(value)] = struct{}{}
	}
	for _, value := range values {
		value = normalizeSkillID(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		input = append(input, value)
	}
	return input
}

func removeActiveNativeIDs(input []string, active map[string]struct{}) []string {
	out := make([]string, 0, len(input))
	for _, skillID := range input {
		if _, ok := active[normalizeSkillID(skillID)]; !ok {
			out = append(out, normalizeSkillID(skillID))
		}
	}
	return out
}

func nativeSkipContains(skips []NativeSkillSkip, skillID string) bool {
	for _, skip := range skips {
		if normalizeSkillID(skip.SkillID) == normalizeSkillID(skillID) {
			return true
		}
	}
	return false
}

func nativeSkillSearchScore(doc SkillDocument, query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	haystack := strings.ToLower(strings.Join(nativeSkillSearchFields(doc), " "))
	score := 0
	if strings.Contains(haystack, query) {
		score += 1000
	}
	for _, token := range nativeSearchTokens(query) {
		if strings.Contains(haystack, token) {
			score += 20 + utf8.RuneCountInString(token)
		}
	}
	return score
}

func nativeSkillSearchFields(doc SkillDocument) []string {
	fields := []string{doc.Metadata.ID, doc.Metadata.Name, doc.Metadata.Description, doc.Metadata.WhenToUse}
	for _, values := range []map[string]string{doc.Metadata.Display.Label, doc.Metadata.Display.Description, doc.Metadata.Display.WhenToUse} {
		for _, value := range values {
			fields = append(fields, value)
		}
	}
	fields = append(fields, doc.Metadata.Display.Scenarios...)
	for _, values := range doc.Metadata.Display.Tags {
		fields = append(fields, values...)
	}
	return fields
}

func nativeSearchTokens(value string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	appendToken := func(token string) {
		token = strings.TrimFunc(strings.ToLower(token), func(current rune) bool {
			return unicode.IsSpace(current) || unicode.IsPunct(current)
		})
		if utf8.RuneCountInString(token) < 2 {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	for _, token := range strings.FieldsFunc(value, func(current rune) bool {
		return unicode.IsSpace(current) || unicode.IsPunct(current)
	}) {
		appendToken(token)
		runes := []rune(token)
		if len(runes) > 2 && !allNativeASCII(runes) {
			for index := 0; index+1 < len(runes); index++ {
				appendToken(string(runes[index : index+2]))
			}
		}
	}
	return out
}

func allNativeASCII(value []rune) bool {
	for _, current := range value {
		if current > unicode.MaxASCII {
			return false
		}
	}
	return true
}
