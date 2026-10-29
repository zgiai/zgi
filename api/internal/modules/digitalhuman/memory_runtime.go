package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/datatypes"
)

const (
	digitalHumanMemoryModeExplicit  = "explicit"
	digitalHumanMemoryModeProactive = "proactive"
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
	// StableGuidance is the memory rules, which stay the same from run to run
	// of a conversation; they go in the system prompt.
	StableGuidance string
	// SystemPromptAddendum is the memory itself, which changes from run to
	// run; it is sent as turn context.
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
	preparation := &MemoryRuntimePreparation{
		StableGuidance: renderDigitalHumanMemoryGuidance(toolEnabled, state.AutoExtractionEnabled),
	}
	if toolEnabled {
		if section := renderMemoryInstructions(p.service.runMemoryInstructions(ctx, run)); section != "" {
			preparation.StableGuidance += "\n\n" + section
		}
		if editor := p.fileEditor(run, event, state, promotionAuthorized); editor != nil {
			preparation.fileEditors = append(preparation.fileEditors, editor)
		}
	}
	// Memory is files (ADR 0014): recalled at run start, read with memory_view
	// and memory_search, changed with memory_edit.
	if section := p.prepareMemoryFiles(ctx, run, event); section != "" {
		preparation.SystemPromptAddendum = section
	}
	preparation.Tools = append(preparation.Tools, p.memoryFileTools(run, event)...)
	if search, ok := p.conversationSearchTool(run, event); ok {
		preparation.Tools = append(preparation.Tools, search)
		preparation.StableGuidance += "\n\nWhen the user refers to something discussed in an earlier conversation that your memory files do not cover, search for it with conversation_search before saying you do not know."
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

// renderDigitalHumanMemoryGuidance is the memory policy of a run. Memory is
// recalled into the context as files and read with memory_view and
// memory_search (ADR 0014).
func renderDigitalHumanMemoryGuidance(toolEnabled, proactiveAllowed bool) string {
	sections := []string{
		"Long-term memory belongs to this DigitalHuman, not to the current user.",
		"Treat memory as fallible context: respect its scope and revision, and prefer current tool evidence when facts conflict.",
		memoryFilesGuidance,
	}
	if toolEnabled {
		modePolicy := "Proactive memory maintenance is disabled. Use explicit mode only when the current user directly establishes, corrects, or asks to forget durable DigitalHuman working context."
		if proactiveAllowed {
			modePolicy = "Proactive memory maintenance is enabled. Proactive mode may save only high-confidence durable work experience, decisions, open loops, reflections, or evidence-backed facts directly supported by the current user message."
		}
		sections = append(sections,
			"Decide at runtime whether a memory change is warranted from the meaning and context of the conversation, not from trigger words. If no safe durable update is needed, do not call the memory tool.",
			"Change memory files only with memory_edit, as described below.",
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

// commitMemoryToolArguments admits and applies one model-proposed memory
// change: the run's trust decides class and scope, the decision is checked,
// and the change is committed with its fences, audit event, and undo record.
// memory_edit has already checked that the evidence quotes the user.
func (p *DefaultMemoryRuntimeProvider) commitMemoryToolArguments(
	ctx context.Context,
	run *Run,
	event invocation.Event,
	state *MemoryState,
	promotionAuthorized bool,
	operationID uuid.UUID,
	args memoryToolArguments,
) skillloop.RuntimeToolResult {
	mode := memoryWriteExplicit
	if args.Mode == digitalHumanMemoryModeProactive {
		mode = memoryWriteProactive
	}
	admission := admitMemoryWrite(run, event, mode, state.AutoExtractionEnabled)
	request := MemoryProposalRequest{
		OperationID: operationID, Key: args.Key, Action: args.Action, MemoryType: args.MemoryType,
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
	if mode == memoryWriteProactive {
		request.noteSourceKind = relationshipmemory.SourceAutomatic
	}
	if !admission.Allowed {
		p.service.recordRejectedMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request, admission.err())
		return memoryToolFailure(AppCodeMemoryPolicyRejected.String(), false)
	}
	if decisionErr := validateMemoryToolMode(args, state.AutoExtractionEnabled); decisionErr != nil {
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
			"action": args.Action, "mode": args.Mode, "key": strings.TrimSpace(args.Key), "note": admission.NoteInstanceID != "",
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
