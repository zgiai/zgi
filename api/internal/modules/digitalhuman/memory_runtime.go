package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/datatypes"
)

const (
	digitalHumanMemoryToolName          = "manage_digital_human_memory"
	digitalHumanMemoryRetrievalToolName = "retrieve_digital_human_memory"
	digitalHumanMemoryModeExplicit      = "explicit"
	digitalHumanMemoryModeProactive     = "proactive"
)

const (
	defaultDigitalHumanMemoryRetrievalTopK = 5
	maxDigitalHumanMemoryRetrievalTopK     = 20
)

// DigitalHumanMemoryRetrievalRequest contains only model-controlled retrieval
// inputs. Identity, tenancy, and the internal projection are resolved by the
// runtime before the backend is called.
type DigitalHumanMemoryRetrievalRequest struct {
	Run           *Run
	Event         invocation.Event
	Query         string
	TopK          int
	RetrievalMode string
}

// DigitalHumanMemoryRetrievalResult is the model-facing projection returned by
// the internal retrieval backend. The backend may attach authoritative memory
// items for audit logging; their contents are never persisted in the tool trace.
type DigitalHumanMemoryRetrievalResult struct {
	Status        string
	Context       string
	ResultCount   int
	TopScore      *float64
	SourceSummary []map[string]any
	ContextBlocks []map[string]any
	Warnings      []string
	Items         []*DigitalHumanMemory
}

// DigitalHumanMemoryRetriever is intentionally narrower than the generic
// knowledge-base service. Implementations resolve the hidden per-DigitalHuman
// projection and return the active memories for that DigitalHuman.
type DigitalHumanMemoryRetriever interface {
	Retrieve(context.Context, DigitalHumanMemoryRetrievalRequest) (*DigitalHumanMemoryRetrievalResult, error)
}

// DigitalHumanMemoryProjectionRepairer rebuilds the hidden vector projection
// from the authoritative memory rows. It is optional: a deployment can serve
// recall without exposing the operator repair action.
type DigitalHumanMemoryProjectionRepairer interface {
	Repair(context.Context, memoryProjectionScope) (*MemoryProjectionRepairResult, error)
}

// MemoryRuntimePreparation is the safe context/tool projection for one run.
type MemoryRuntimePreparation struct {
	SystemPromptAddendum string
	Tools                []skillloop.RuntimeTool
	// fileEditors are the directories this run may change with memory_edit
	// (ADR 0014). The composite provider builds the one tool over them.
	fileEditors []memoryFileEditor
	// archiveSearch finds the user's archive entries; the composite provider
	// adds them to memory_search results.
	archiveSearch func(ctx context.Context, query string, limit int) ([]relationshipmemory.UserMemoryArchiveEntry, error)
	// forgottenLines lists what the person removed from their memory; the
	// composite provider withholds conversation search turns that repeat it.
	forgottenLines func(ctx context.Context) ([]string, error)
}

// MemoryRuntimeProvider keeps DigitalHuman memory out of the generic
// ChatRuntime storage model while exposing generic runtime tools at its edge.
type MemoryRuntimeProvider interface {
	Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*MemoryRuntimePreparation, error)
}

type memoryRuntimeFinalizer interface {
	Finalize(context.Context, *Run, *chatruntime.PreparedChat, *chatruntime.ChatResult) error
}

// MemoryPromotionAuthorizer decides whether a run actor may promote private
// conversation content into shared DigitalHuman memory.
type MemoryPromotionAuthorizer interface {
	CanPromoteDigitalHumanMemory(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

// MemoryPromotionAuthorizerFunc adapts a function to MemoryPromotionAuthorizer.
type MemoryPromotionAuthorizerFunc func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)

func (f MemoryPromotionAuthorizerFunc) CanPromoteDigitalHumanMemory(ctx context.Context, organizationID, workspaceID, actorID uuid.UUID) (bool, error) {
	return f(ctx, organizationID, workspaceID, actorID)
}

// MemoryRuntimeProviderOption configures the DigitalHuman-owned memory edge.
type MemoryRuntimeProviderOption func(*DefaultMemoryRuntimeProvider)

// WithDigitalHumanMemoryRetriever attaches the hidden, query-aware retrieval
// backend. It does not expose a dataset ID or a resource binding to callers.
func WithDigitalHumanMemoryRetriever(retriever DigitalHumanMemoryRetriever) MemoryRuntimeProviderOption {
	return func(provider *DefaultMemoryRuntimeProvider) {
		provider.retriever = retriever
	}
}

type DefaultMemoryRuntimeProvider struct {
	service   *Service
	retriever DigitalHumanMemoryRetriever
	now       func() time.Time
}

func NewMemoryRuntimeProvider(service *Service, options ...MemoryRuntimeProviderOption) *DefaultMemoryRuntimeProvider {
	if service == nil {
		panic("DigitalHuman memory runtime requires service")
	}
	provider := &DefaultMemoryRuntimeProvider{service: service, now: func() time.Time { return time.Now().UTC() }}
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	return provider
}

func (p *DefaultMemoryRuntimeProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (*MemoryRuntimePreparation, error) {
	if run == nil || resolved == nil || event.Target.ID != run.DigitalHumanID {
		return nil, ErrInvalidRequest
	}
	scope := runScope(run)
	state, err := p.service.memoryState(ctx, scope, run.DigitalHumanID)
	if err != nil {
		return nil, err
	}
	// The run's admission-time trust level decides, so a channel run never
	// borrows the installer's permission.
	promotionAuthorized := RunSourceTrust(run) == SourceTrustAuthorizedOperator
	toolEnabled := memoryToolAvailable(run, state.AutoExtractionEnabled)
	files := memoryFilesEnabled()
	preparation := &MemoryRuntimePreparation{
		SystemPromptAddendum: renderDigitalHumanMemoryGuidance(toolEnabled, state.AutoExtractionEnabled, files),
	}
	if files && toolEnabled {
		if editor := p.fileEditor(run, event, state, promotionAuthorized); editor != nil {
			preparation.fileEditors = append(preparation.fileEditors, editor)
		}
	} else if _, ok := runInstanceScopeID(run); ok && toolEnabled {
		preparation.SystemPromptAddendum += "\n\nMemory changes made in this conversation belong to this end user's relationship only. They cannot change the DigitalHuman's own self memory or what other users see. If memory is full, update or clear an entry that no longer matters instead of adding one."
	}
	if !files && toolEnabled && RunSourceTrust(run) == SourceTrustChannelParticipant {
		preparation.SystemPromptAddendum += "\n\nMemory changes made in this conversation are internal: they are shared with everyone inside the team, in every channel and the console, and external users never see them. Memory that external users see can be changed only by an operator."
	}
	if files {
		if section := p.prepareMemoryFiles(ctx, run, event); section != "" {
			preparation.SystemPromptAddendum += "\n\n" + section
		}
		preparation.Tools = append(preparation.Tools, p.memoryFileTools(run, event)...)
		if search, ok := p.conversationSearchTool(run, event); ok {
			preparation.Tools = append(preparation.Tools, search)
			preparation.SystemPromptAddendum += "\n\nWhen the user refers to something discussed in an earlier conversation that your memory files do not cover, search for it with conversation_search before saying you do not know."
		}
	} else if p.retriever != nil {
		preparation.Tools = append(preparation.Tools, p.memoryRetrievalTool(run, event))
	}
	if toolEnabled && !files {
		preparation.Tools = append(preparation.Tools, p.memoryMutationTool(run, event, state, promotionAuthorized))
	}
	return preparation, nil
}

// Finalize persists only durable source references for later extraction. The
// worker reloads message content transiently after the run has completed.
func (p *DefaultMemoryRuntimeProvider) Finalize(ctx context.Context, run *Run, prepared *chatruntime.PreparedChat, result *chatruntime.ChatResult) error {
	if run == nil || prepared == nil || prepared.Conversation == nil || prepared.Message == nil || result == nil {
		return ErrInvalidRequest
	}
	return p.service.ScheduleMemoryExtraction(
		ctx,
		runScope(run),
		run,
		prepared.Conversation.ID,
		prepared.Message.ID,
		safeMemoryArtifactRefs(result.Metadata),
	)
}

func renderDigitalHumanMemoryContext(items []*DigitalHumanMemory, toolEnabled, proactiveAllowed bool) string {
	_ = items
	return renderDigitalHumanMemoryGuidance(toolEnabled, proactiveAllowed, false)
}

// renderDigitalHumanMemoryGuidance is the memory policy of a run. With files
// (ADR 0014) memory is recalled into the context and read with memory_view
// and memory_search instead of retrieve_digital_human_memory.
func renderDigitalHumanMemoryGuidance(toolEnabled, proactiveAllowed, files bool) string {
	recall := "Long-term memory is not preloaded. When the current question may depend on durable context, call retrieve_digital_human_memory with a concise query before answering."
	lookup := "Retrieval returns the memory_id, memory_key and revision of every match. Correct or clear an existing memory only with its returned key and revision, create a new memory with expected_revision 0, and replace an existing memory only by passing its returned memory_id as supersedes_id."
	if files {
		recall = memoryFilesGuidance
		lookup = "Change memory files only with memory_edit, as described below."
	}
	sections := []string{
		"Long-term memory belongs to this DigitalHuman, not to the current user.",
		"Treat memory as fallible context: respect its scope and revision, and prefer current tool evidence when facts conflict.",
		recall,
	}
	if toolEnabled {
		modePolicy := "Proactive memory maintenance is disabled. Use explicit mode only when the current user directly establishes, corrects, or asks to forget durable DigitalHuman working context."
		if proactiveAllowed {
			modePolicy = "Proactive memory maintenance is enabled. Proactive mode may save only high-confidence durable work experience, decisions, open loops, reflections, or evidence-backed facts directly supported by the current user message."
		}
		sections = append(sections,
			"Decide at runtime whether a memory change is warranted from the meaning and context of the conversation, not from trigger words. If no safe durable update is needed, do not call the memory tool.",
			lookup,
			modePolicy,
			"Every memory operation must include an exact non-empty quote from the current user message as transient evidence. Never use assistant text, tool output, inferred behavior, hypotheticals, temporary details, or third-party facts as evidence.",
		)
	} else {
		sections = append(sections, "Runtime memory mutation is unavailable for this actor and invocation. Do not claim that memory was saved or changed.")
	}
	sections = append(sections,
		"Treat an explicit durable working directive (for example, a future role, recurring responsibility, or default response behavior) as self/decision memory. It does not modify the published identity, persona, or rules.",
		"Never store a conversational role directive as self/identity. Published identity, persona, and rules can only change through configuration and publishing.",
		"Never claim a memory was saved unless the memory tool reports committed.",
		"Never store credentials, private conversation details without explicit promotion, full business objects, document bodies, or artifact bodies.",
	)
	return strings.Join(sections, "\n\n")
}

func (p *DefaultMemoryRuntimeProvider) memoryRetrievalTool(run *Run, event invocation.Event) skillloop.RuntimeTool {
	definition := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        digitalHumanMemoryRetrievalToolName,
		Description: "Retrieve relevant long-term memory for the current question. Call this only when durable DigitalHuman context may affect the answer. The backend automatically limits results to this DigitalHuman; never ask for or provide a dataset ID. Every returned match carries the memory_id, memory_key and revision that identify it, which are required to correct, clear or replace it later.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":          map[string]any{"type": "string", "description": "A concise query describing the durable fact or context needed to answer the current question."},
				"top_k":          map[string]any{"type": "integer", "minimum": 1, "maximum": maxDigitalHumanMemoryRetrievalTopK},
				"retrieval_mode": map[string]any{"type": "string", "enum": []string{"vector", "hybrid"}},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		args, err := decodeMemoryRetrievalToolArguments(call.Function.Arguments)
		if err != nil {
			return skillloop.RuntimeToolResult{Status: "error", Error: err, Recoverable: true}
		}
		result, err := p.retriever.Retrieve(ctx, DigitalHumanMemoryRetrievalRequest{
			Run: run, Event: event, Query: args.Query, TopK: args.TopK, RetrievalMode: args.RetrievalMode,
		})
		if err != nil {
			return skillloop.RuntimeToolResult{Status: "error", Arguments: map[string]any{"query": args.Query}, Error: err, Recoverable: true}
		}
		if result == nil {
			return skillloop.RuntimeToolResult{Status: "error", Arguments: map[string]any{"query": args.Query}, Error: errors.New("digital human memory retrieval returned nil result"), Recoverable: true}
		}
		p.recordRetrieval(ctx, run, result.Items)
		modelResult := map[string]any{
			"query": args.Query, "status": result.Status, "context": result.Context,
			"result_count": result.ResultCount, "source_summary": result.SourceSummary,
			"context_blocks": result.ContextBlocks, "warnings": result.Warnings,
		}
		if result.TopScore != nil {
			modelResult["top_score"] = *result.TopScore
		}
		return skillloop.RuntimeToolResult{
			Status:      "success",
			Arguments:   map[string]any{"query": args.Query, "top_k": args.TopK, "retrieval_mode": args.RetrievalMode},
			Result:      map[string]any{"status": result.Status, "result_count": result.ResultCount},
			ModelResult: modelResult,
		}
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanMemory, Handler: handler}
}

type memoryRetrievalToolArguments struct {
	Query         string `json:"query"`
	TopK          int    `json:"top_k,omitempty"`
	RetrievalMode string `json:"retrieval_mode,omitempty"`
}

func decodeMemoryRetrievalToolArguments(raw string) (memoryRetrievalToolArguments, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var args memoryRetrievalToolArguments
	if err := decoder.Decode(&args); err != nil {
		return memoryRetrievalToolArguments{}, ErrInvalidRequest
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return memoryRetrievalToolArguments{}, ErrInvalidRequest
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return memoryRetrievalToolArguments{}, ErrInvalidRequest
	}
	if args.TopK == 0 {
		args.TopK = defaultDigitalHumanMemoryRetrievalTopK
	}
	if args.TopK < 1 || args.TopK > maxDigitalHumanMemoryRetrievalTopK {
		return memoryRetrievalToolArguments{}, ErrInvalidRequest
	}
	args.RetrievalMode = strings.TrimSpace(strings.ToLower(args.RetrievalMode))
	if args.RetrievalMode == "" {
		args.RetrievalMode = "hybrid"
	}
	if args.RetrievalMode != "vector" && args.RetrievalMode != "hybrid" {
		return memoryRetrievalToolArguments{}, ErrInvalidRequest
	}
	return args, nil
}

// memoryMutationTool exposes the single converged memory write surface. The
// tool never accepts memory_class or scope fields: the run's trust level
// decides the class and scope (ADR 0004).
func (p *DefaultMemoryRuntimeProvider) memoryMutationTool(
	run *Run,
	event invocation.Event,
	state *MemoryState,
	promotionAuthorized bool,
) skillloop.RuntimeTool {
	modes := []string{digitalHumanMemoryModeExplicit}
	if state.AutoExtractionEnabled {
		modes = append(modes, digitalHumanMemoryModeProactive)
	}
	definition := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        digitalHumanMemoryToolName,
		Description: "Propose one evidence-backed long-term memory change for this DigitalHuman when the conversation semantically warrants it; do not call this tool when no safe durable change is needed. Use explicit mode when the current user directly establishes, corrects, or asks to forget durable working context, including a standing role or responsibility. Use proactive mode only when available and only for high-confidence durable information directly stated by the current user. Store a durable working directive as self/decision, never self/identity; it does not modify the published persona or rules. Every operation requires an exact quote from the current user message as transient evidence. To correct or clear an existing memory, retrieve it first and reuse its returned memory_key as key with its returned revision as expected_revision; never guess either value, and never give a fact that is already stored a second key. Use expected_revision 0 only for a key that does not exist yet. To replace a stale memory with a new semantic key, retrieve it first and pass its returned memory_id as supersedes_id with its returned revision as expected_superseded_revision.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":            map[string]any{"type": "string", "enum": []string{"upsert", "clear"}},
				"mode":              map[string]any{"type": "string", "enum": modes},
				"evidence":          map[string]any{"type": "string", "description": "An exact non-empty quote from the current user message supporting this operation."},
				"key":               map[string]any{"type": "string", "description": "Stable semantic key, max 160 characters."},
				"memory_type":       map[string]any{"type": "string", "enum": []string{"experience", "decision", "fact", "open_loop", "reflection"}},
				"content":           map[string]any{"type": "string", "description": "A concise, redacted summary. Empty only for clear."},
				"expected_revision": map[string]any{"type": "integer", "minimum": 0},
				"supersedes_id":     map[string]any{"type": "string", "format": "uuid", "description": "ID of an active memory replaced by this new key."},
				"expected_superseded_revision": map[string]any{
					"type": "integer", "minimum": 1, "description": "Current revision of supersedes_id.",
				},
				"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
			},
			"required":             []string{"action", "mode", "evidence", "key", "memory_type", "expected_revision"},
			"additionalProperties": false,
		},
	}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		args, err := decodeMemoryToolArguments(call.Function.Arguments)
		if err != nil {
			return memoryToolFailure("invalid_arguments", true)
		}
		return p.commitMemoryToolArguments(ctx, run, event, state, promotionAuthorized, memoryToolOperationID(run.ID, call), args, false)
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanMemory, Handler: handler}
}

// commitMemoryToolArguments admits and applies one model-proposed memory
// change: the run's trust decides class and scope, the decision is checked,
// and the change is committed with its fences, audit event, and undo record.
// evidenceChecked lets a caller that has already verified the user quote
// (memory_edit) skip the exact-substring check.
func (p *DefaultMemoryRuntimeProvider) commitMemoryToolArguments(
	ctx context.Context,
	run *Run,
	event invocation.Event,
	state *MemoryState,
	promotionAuthorized bool,
	operationID uuid.UUID,
	args memoryToolArguments,
	evidenceChecked bool,
) skillloop.RuntimeToolResult {
	memoryClass, memoryScope := memoryruntime.MemoryClassSelf, (*memoryruntime.MemoryScopeRef)(nil)
	mode := memoryWriteExplicit
	if args.Mode == digitalHumanMemoryModeProactive {
		mode = memoryWriteProactive
	}
	admission := admitMemoryWrite(run, event, mode, state.AutoExtractionEnabled)
	if admission.Allowed {
		memoryClass, memoryScope = admission.Class, admission.Scope
	}
	request := MemoryProposalRequest{
		OperationID: operationID, Key: args.Key, Action: args.Action, MemoryClass: memoryClass, Scope: memoryScope, MemoryType: args.MemoryType,
		Content: args.Content, ExpectedRevision: args.ExpectedRevision, SupersedesID: args.SupersedesID,
		ExpectedSupersededRevision: args.ExpectedSupersededRevision, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
		Confidence: args.Confidence, RunID: &run.ID,
		Evidence: []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeRun, ID: run.ID.String(), Digest: run.RunSpecRevision}},
		SourceContext: MemorySourceContext{
			PrivateConversation:       event.Origin.Kind == invocation.OriginManual,
			PromotedByAuthorizedActor: promotionAuthorized,
		},
		OccurredAt: p.now(),
	}
	request.SourceContext.SourceTrust = admission.Trust
	admission.stamp(&request)
	if !admission.Allowed {
		p.service.recordRejectedMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request, admission.err())
		return memoryToolFailure(AppCodeMemoryPolicyRejected.String(), false)
	}
	decisionErr := validateMemoryToolDecision(args, run.Query, state.AutoExtractionEnabled)
	if evidenceChecked {
		decisionErr = validateMemoryToolMode(args, state.AutoExtractionEnabled)
	}
	if decisionErr != nil {
		p.service.recordRejectedMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request, decisionErr)
		return memoryToolFailure(AppCodeMemoryPolicyRejected.String(), false)
	}
	result, err := p.service.ApplyAutonomousMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request)
	if err != nil {
		return memoryToolFailure(memoryPublicErrorCode(err), false)
	}
	return skillloop.RuntimeToolResult{
		Status: "success",
		Arguments: map[string]any{
			"action": args.Action, "mode": args.Mode, "key": strings.TrimSpace(args.Key), "memory_class": memoryClass,
			"memory_type": args.MemoryType, "supersedes_id": args.SupersedesID,
		},
		Result: map[string]any{"status": "committed", "operation_id": result.Event.OperationID.String(), "memory_id": result.Memory.ID.String(), "revision": result.Memory.Revision, "replayed": result.Replayed},
	}
}

type memoryToolArguments struct {
	Action                     memoryruntime.MutationAction `json:"action"`
	Mode                       string                       `json:"mode"`
	Evidence                   string                       `json:"evidence"`
	Key                        string                       `json:"key"`
	MemoryType                 memoryruntime.MemoryType     `json:"memory_type"`
	Content                    string                       `json:"content"`
	ExpectedRevision           int64                        `json:"expected_revision"`
	SupersedesID               *uuid.UUID                   `json:"supersedes_id"`
	ExpectedSupersededRevision int64                        `json:"expected_superseded_revision"`
	Confidence                 float64                      `json:"confidence"`
}

func decodeMemoryToolArguments(raw string) (memoryToolArguments, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var args memoryToolArguments
	if err := decoder.Decode(&args); err != nil {
		return memoryToolArguments{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return memoryToolArguments{}, ErrInvalidRequest
	}
	if args.Confidence == 0 {
		args.Confidence = 1
	}
	args.Mode = strings.ToLower(strings.TrimSpace(args.Mode))
	args.Evidence = strings.TrimSpace(args.Evidence)
	return args, nil
}

func validateMemoryToolDecision(args memoryToolArguments, latestUserMessage string, proactiveAllowed bool) error {
	latestUserMessage = strings.TrimSpace(latestUserMessage)
	if latestUserMessage == "" || args.Evidence == "" || !strings.Contains(latestUserMessage, args.Evidence) {
		return ErrMemoryPolicyRejected
	}
	return validateMemoryToolMode(args, proactiveAllowed)
}

// validateMemoryToolMode checks the write mode: explicit always, proactive
// only when automatic memory is on and only for an upsert.
func validateMemoryToolMode(args memoryToolArguments, proactiveAllowed bool) error {
	switch args.Mode {
	case digitalHumanMemoryModeExplicit:
		return nil
	case digitalHumanMemoryModeProactive:
		if proactiveAllowed && args.Action == memoryruntime.MutationActionUpsert {
			return nil
		}
		return ErrMemoryPolicyRejected
	default:
		return ErrMemoryPolicyRejected
	}
}

func memoryToolOperationID(runID uuid.UUID, call adapter.ToolCall) uuid.UUID {
	identity := strings.TrimSpace(call.ID)
	if identity == "" {
		identity = call.Function.Name + "\x00" + call.Function.Arguments
	}
	return uuid.NewSHA1(runID, []byte(identity))
}

func memoryToolFailure(code string, recoverable bool) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{Status: "failed", Arguments: map[string]any{}, Result: map[string]any{"status": "rejected", "error_code": code, "retry_allowed": recoverable}, Error: ErrMemoryPolicyRejected, Recoverable: recoverable}
}

func memoryPublicErrorCode(err error) string {
	switch {
	case apperror.IsCode(err, AppCodeMemoryConflict):
		return AppCodeMemoryConflict.String()
	case errors.Is(err, errInstanceMemoryFull):
		return memoryAdmissionReasonInstanceFull
	case apperror.IsCode(err, AppCodeMemoryPolicyRejected):
		return AppCodeMemoryPolicyRejected.String()
	default:
		return "digital_human.memory.failed"
	}
}

func safeMemoryArtifactRefs(metadata map[string]interface{}) datatypes.JSON {
	refs := make([]map[string]interface{}, 0)
	seen := make(map[string]struct{})
	allowed := allowedTraceFields("artifact_id", "artifact_type", "file_id", "invocation_id")
	for _, section := range []string{"conversation_artifacts", "generated_files"} {
		for _, item := range traceMapSlice(metadata[section], allowed, 100) {
			encoded, err := json.Marshal(item)
			if err != nil {
				continue
			}
			key := string(encoded)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, item)
			if len(refs) == 100 {
				break
			}
		}
		if len(refs) == 100 {
			break
		}
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		return datatypes.JSON([]byte(`[]`))
	}
	return datatypes.JSON(encoded)
}

func (p *DefaultMemoryRuntimeProvider) recordRetrieval(ctx context.Context, run *Run, items []*DigitalHumanMemory) {
	refs := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item != nil {
			refs = append(refs, map[string]any{"memory_id": item.ID, "revision": item.Revision, "content_digest": item.ContentDigest})
		}
	}
	detail, _ := json.Marshal(map[string]any{"memory_refs": refs})
	_ = p.service.repo.CreateActionLedger(context.WithoutCancel(ctx), &ActionLedgerEntry{
		ID: uuid.New(), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, DigitalHumanID: run.DigitalHumanID,
		RunID: &run.ID, ActionType: "memory.retrieve", Status: "completed", Detail: datatypes.JSON(detail), CreatedAt: p.now(),
	})
}
