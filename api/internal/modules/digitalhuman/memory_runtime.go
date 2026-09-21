package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/datatypes"
)

const (
	digitalHumanMemoryToolName      = "manage_digital_human_memory"
	digitalHumanMemoryModeExplicit  = "explicit"
	digitalHumanMemoryModeProactive = "proactive"
)

// MemoryRuntimePreparation is the safe context/tool projection for one run.
type MemoryRuntimePreparation struct {
	SystemPromptAddendum string
	Tools                []skillloop.RuntimeTool
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

// WithMemoryPromotionAuthorizer requires an explicit workspace permission
// decision before private conversation content can be promoted.
func WithMemoryPromotionAuthorizer(authorizer MemoryPromotionAuthorizer) MemoryRuntimeProviderOption {
	return func(provider *DefaultMemoryRuntimeProvider) {
		provider.promotionAuthorizer = authorizer
	}
}

type DefaultMemoryRuntimeProvider struct {
	service             *Service
	promotionAuthorizer MemoryPromotionAuthorizer
	now                 func() time.Time
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
	authorizedScopes := authorizedMemoryScopes(event, run.WorkspaceID)
	items, err := p.service.RetrieveMemories(ctx, scope, run.DigitalHumanID, authorizedScopes, 50)
	if err != nil {
		return nil, err
	}
	state, err := p.service.memoryState(ctx, scope, run.DigitalHumanID)
	if err != nil {
		return nil, err
	}
	p.recordRetrieval(ctx, run, items, authorizedScopes)
	promotionAuthorized := false
	if p.promotionAuthorizer != nil && run.ActorID != nil && *run.ActorID != uuid.Nil {
		promotionAuthorized, err = p.promotionAuthorizer.CanPromoteDigitalHumanMemory(
			ctx, run.OrganizationID, run.WorkspaceID, *run.ActorID,
		)
		if err != nil {
			return nil, err
		}
	}
	toolEnabled := digitalHumanMemoryToolEnabled(event, state, promotionAuthorized)
	preparation := &MemoryRuntimePreparation{
		SystemPromptAddendum: renderDigitalHumanMemoryContext(items, toolEnabled, state.AutoExtractionEnabled),
	}
	if toolEnabled {
		preparation.Tools = []skillloop.RuntimeTool{p.memoryMutationTool(run, event, state, authorizedScopes, promotionAuthorized)}
	}
	return preparation, nil
}

func digitalHumanMemoryToolEnabled(event invocation.Event, state *MemoryState, actorAuthorized bool) bool {
	if state == nil {
		return false
	}
	if event.Origin.Kind == invocation.OriginManual {
		return actorAuthorized
	}
	return actorAuthorized || state.AutoExtractionEnabled
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

func authorizedMemoryScopes(event invocation.Event, workspaceID uuid.UUID) []memoryruntime.MemoryScopeRef {
	scopes := []memoryruntime.MemoryScopeRef{{Type: memoryruntime.MemoryScopeWorkspace, ID: workspaceID.String()}}
	trusted, _ := event.Context["memory_scopes_authorized"].(bool)
	if !trusted {
		return scopes
	}
	raw, ok := event.Context["memory_scopes"].([]interface{})
	if !ok {
		return scopes
	}
	seen := map[string]struct{}{string(memoryruntime.MemoryScopeWorkspace) + "\x00" + workspaceID.String(): {}}
	for _, item := range raw {
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		var candidate memoryruntime.MemoryScopeRef
		if json.Unmarshal(encoded, &candidate) != nil {
			continue
		}
		candidate, err = memoryruntime.ValidateMemoryScopeRef(candidate)
		if err != nil {
			continue
		}
		key := string(candidate.Type) + "\x00" + candidate.ID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		scopes = append(scopes, candidate)
	}
	return scopes
}

func renderDigitalHumanMemoryContext(items []*DigitalHumanMemory, toolEnabled, proactiveAllowed bool) string {
	sections := []string{
		"Long-term memory belongs to this DigitalHuman, not to the current user.",
		"Treat memory as fallible context: respect its scope and revision, and prefer current tool evidence when facts conflict.",
	}
	if len(items) == 0 {
		sections = append(sections, "No active long-term memory is available in the authorized scopes for this run.")
	} else {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			scope := "self"
			if item.MemoryClass == memoryruntime.MemoryClassBusiness {
				scope = string(item.ScopeType) + ":" + item.ScopeID
			}
			lines = append(lines, fmt.Sprintf("- [%s/%s; id=%s; key=%s; scope=%s; revision=%d] %s", item.MemoryClass, item.MemoryType, item.ID, item.MemoryKey, scope, item.Revision, item.Content))
		}
		sections = append(sections, "Authorized active long-term memory:\n"+strings.Join(lines, "\n"))
	}
	if toolEnabled {
		modePolicy := "Proactive memory maintenance is disabled. Use explicit mode only when the current user directly establishes, corrects, or asks to forget durable DigitalHuman working context."
		if proactiveAllowed {
			modePolicy = "Proactive memory maintenance is enabled. Proactive mode may save only high-confidence durable work experience, decisions, open loops, reflections, or authorized business facts directly supported by the current user message."
		}
		sections = append(sections,
			"Decide at runtime whether a memory change is warranted from the meaning and context of the conversation, not from trigger words. If no safe durable update is needed, do not call the memory tool.",
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

func (p *DefaultMemoryRuntimeProvider) memoryMutationTool(
	run *Run,
	event invocation.Event,
	state *MemoryState,
	authorizedScopes []memoryruntime.MemoryScopeRef,
	promotionAuthorized bool,
) skillloop.RuntimeTool {
	modes := []string{digitalHumanMemoryModeExplicit}
	if state.AutoExtractionEnabled {
		modes = append(modes, digitalHumanMemoryModeProactive)
	}
	definition := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        digitalHumanMemoryToolName,
		Description: "Propose one evidence-backed long-term memory change for this DigitalHuman when the conversation semantically warrants it; do not call this tool when no safe durable change is needed. Use explicit mode when the current user directly establishes, corrects, or asks to forget durable working context, including a standing role or responsibility. Use proactive mode only when available and only for high-confidence durable information directly stated by the current user. Store a durable working directive as self/decision, never self/identity; it does not modify the published persona or rules. Every operation requires an exact quote from the current user message as transient evidence. To replace a stale memory with a new semantic key, provide its id and revision as supersedes_id and expected_superseded_revision.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":            map[string]any{"type": "string", "enum": []string{"upsert", "clear"}},
				"mode":              map[string]any{"type": "string", "enum": modes},
				"evidence":          map[string]any{"type": "string", "description": "An exact non-empty quote from the current user message supporting this operation."},
				"key":               map[string]any{"type": "string", "description": "Stable semantic key, max 160 characters."},
				"memory_class":      map[string]any{"type": "string", "enum": []string{"self", "business"}},
				"memory_type":       map[string]any{"type": "string", "enum": []string{"experience", "decision", "fact", "open_loop", "reflection"}},
				"scope_type":        map[string]any{"type": "string", "enum": []string{"workspace", "project", "customer", "campaign", "channel"}},
				"scope_id":          map[string]any{"type": "string"},
				"content":           map[string]any{"type": "string", "description": "A concise, redacted summary. Empty only for clear."},
				"expected_revision": map[string]any{"type": "integer", "minimum": 0},
				"supersedes_id":     map[string]any{"type": "string", "format": "uuid", "description": "ID of an active same-class, same-scope memory replaced by this new key."},
				"expected_superseded_revision": map[string]any{
					"type": "integer", "minimum": 1, "description": "Current revision of supersedes_id.",
				},
				"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
			},
			"required":             []string{"action", "mode", "evidence", "key", "memory_class", "memory_type", "expected_revision"},
			"additionalProperties": false,
		},
	}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		args, err := decodeMemoryToolArguments(call.Function.Arguments)
		if err != nil {
			return memoryToolFailure("invalid_arguments", true)
		}
		operationID := memoryToolOperationID(run.ID, call)
		request := MemoryProposalRequest{
			OperationID: operationID, Key: args.Key, Action: args.Action, MemoryClass: args.MemoryClass, MemoryType: args.MemoryType,
			Content: args.Content, ExpectedRevision: args.ExpectedRevision, SupersedesID: args.SupersedesID,
			ExpectedSupersededRevision: args.ExpectedSupersededRevision, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Visibility: MemoryVisibilityWorkspace, Confidence: args.Confidence, RunID: &run.ID,
			Evidence: []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeRun, ID: run.ID.String(), Digest: run.RunSpecRevision}},
			SourceContext: MemorySourceContext{
				PrivateConversation:       event.Origin.Kind == invocation.OriginManual,
				PromotedByAuthorizedActor: promotionAuthorized,
			},
			OccurredAt: p.now(),
		}
		if args.MemoryClass == memoryruntime.MemoryClassBusiness {
			request.Scope = &memoryruntime.MemoryScopeRef{Type: args.ScopeType, ID: args.ScopeID}
		}
		if err := validateMemoryToolDecision(args, run.Query, state.AutoExtractionEnabled); err != nil {
			p.service.recordRejectedMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request, err)
			return memoryToolFailure(AppCodeMemoryPolicyRejected.String(), false)
		}
		if args.MemoryClass == memoryruntime.MemoryClassBusiness {
			if !memoryToolBusinessScopeAuthorized(args, authorizedScopes) {
				p.service.recordRejectedMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request, ErrMemoryPolicyRejected)
				return memoryToolFailure(AppCodeMemoryPolicyRejected.String(), false)
			}
		}
		result, err := p.service.ApplyAutonomousMemoryProposal(ctx, runScope(run), run.DigitalHumanID, request)
		if err != nil {
			return memoryToolFailure(memoryPublicErrorCode(err), false)
		}
		return skillloop.RuntimeToolResult{
			Status: "success",
			Arguments: map[string]any{
				"action": args.Action, "mode": args.Mode, "key": strings.TrimSpace(args.Key), "memory_class": args.MemoryClass,
				"memory_type": args.MemoryType, "supersedes_id": args.SupersedesID,
			},
			Result: map[string]any{"status": "committed", "operation_id": result.Event.OperationID.String(), "memory_id": result.Memory.ID.String(), "revision": result.Memory.Revision, "replayed": result.Replayed},
		}
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanMemory, Handler: handler}
}

func memoryToolBusinessScopeAuthorized(args memoryToolArguments, authorizedScopes []memoryruntime.MemoryScopeRef) bool {
	if args.MemoryClass != memoryruntime.MemoryClassBusiness {
		return true
	}
	return memoryScopeAuthorized(memoryruntime.MemoryScopeRef{Type: args.ScopeType, ID: args.ScopeID}, authorizedScopes)
}

type memoryToolArguments struct {
	Action                     memoryruntime.MutationAction  `json:"action"`
	Mode                       string                        `json:"mode"`
	Evidence                   string                        `json:"evidence"`
	Key                        string                        `json:"key"`
	MemoryClass                memoryruntime.MemoryClass     `json:"memory_class"`
	MemoryType                 memoryruntime.MemoryType      `json:"memory_type"`
	ScopeType                  memoryruntime.MemoryScopeType `json:"scope_type"`
	ScopeID                    string                        `json:"scope_id"`
	Content                    string                        `json:"content"`
	ExpectedRevision           int64                         `json:"expected_revision"`
	SupersedesID               *uuid.UUID                    `json:"supersedes_id"`
	ExpectedSupersededRevision int64                         `json:"expected_superseded_revision"`
	Confidence                 float64                       `json:"confidence"`
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

func (p *DefaultMemoryRuntimeProvider) recordRetrieval(ctx context.Context, run *Run, items []*DigitalHumanMemory, scopes []memoryruntime.MemoryScopeRef) {
	refs := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item != nil {
			refs = append(refs, map[string]any{"memory_id": item.ID, "revision": item.Revision, "content_digest": item.ContentDigest})
		}
	}
	detail, _ := json.Marshal(map[string]any{"memory_refs": refs, "authorized_scopes": scopes})
	_ = p.service.repo.CreateActionLedger(context.WithoutCancel(ctx), &ActionLedgerEntry{
		ID: uuid.New(), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, DigitalHumanID: run.DigitalHumanID,
		RunID: &run.ID, ActionType: "memory.retrieve", Status: "completed", Detail: datatypes.JSON(detail), CreatedAt: p.now(),
	})
}
