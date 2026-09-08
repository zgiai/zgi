package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

const userRelationshipMemoryToolName = "mutate_user_relationship_memory"

// CompositeMemoryRuntimeProvider keeps DigitalHuman-owned memory and
// user-owned relationship memory as separate providers and storage domains.
type CompositeMemoryRuntimeProvider struct {
	providers []MemoryRuntimeProvider
}

func NewCompositeMemoryRuntimeProvider(providers ...MemoryRuntimeProvider) *CompositeMemoryRuntimeProvider {
	filtered := make([]MemoryRuntimeProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &CompositeMemoryRuntimeProvider{providers: filtered}
}

func (p *CompositeMemoryRuntimeProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (*MemoryRuntimePreparation, error) {
	result := &MemoryRuntimePreparation{}
	for _, provider := range p.providers {
		prepared, err := provider.Prepare(ctx, run, resolved, event)
		if err != nil {
			return nil, err
		}
		if prepared == nil {
			continue
		}
		if addendum := strings.TrimSpace(prepared.SystemPromptAddendum); addendum != "" {
			if result.SystemPromptAddendum != "" {
				result.SystemPromptAddendum += "\n\n"
			}
			result.SystemPromptAddendum += addendum
		}
		result.Tools = append(result.Tools, prepared.Tools...)
	}
	return result, nil
}

func (p *CompositeMemoryRuntimeProvider) Finalize(ctx context.Context, run *Run, prepared *chatruntime.PreparedChat, result *chatruntime.ChatResult) error {
	var combined error
	for _, provider := range p.providers {
		if finalizer, ok := provider.(memoryRuntimeFinalizer); ok {
			combined = errors.Join(combined, finalizer.Finalize(ctx, run, prepared, result))
		}
	}
	return combined
}

// RelationshipMemoryRuntimeProvider exposes only current-user relationship
// data. It never runs for channels, schedules, service principals, or API users.
type RelationshipMemoryRuntimeProvider struct {
	service *relationshipmemory.Service
}

func NewRelationshipMemoryRuntimeProvider(db *gorm.DB) *RelationshipMemoryRuntimeProvider {
	if db == nil {
		panic("DigitalHuman relationship memory runtime requires db")
	}
	return &RelationshipMemoryRuntimeProvider{service: relationshipmemory.NewService(db)}
}

func (p *RelationshipMemoryRuntimeProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (*MemoryRuntimePreparation, error) {
	if run == nil || resolved == nil || !resolved.Snapshot.UseUserMemory || !relationshipmemory.RuntimeEnabled() {
		return nil, nil
	}
	identity := relationshipmemory.InvocationIdentity{
		Principal:        relationshipmemory.PrincipalRef{Type: string(event.Sender.Type)},
		Surface:          relationshipmemory.SurfaceDigitalHumanConsole,
		ConversationType: relationshipmemory.ConversationPrivate,
	}
	if event.Origin.Kind == invocation.OriginSchedule {
		identity.Surface = relationshipmemory.SurfaceDigitalHumanTask
		identity.ConversationType = relationshipmemory.ConversationTask
	} else if event.Origin.Kind != invocation.OriginManual {
		identity.Surface = relationshipmemory.SurfaceChannel
	}
	if event.Sender.Type == invocation.PrincipalAccount && run.ActorID != nil && *run.ActorID != uuid.Nil && strings.TrimSpace(event.Sender.ID) == run.ActorID.String() {
		subject := relationshipmemory.AccountSubject(*run.ActorID)
		identity.Principal.ID = *run.ActorID
		identity.UserSubject = &subject
	}
	decision := relationshipmemory.DecideAccess(identity)
	relationshipmemory.RecordAccessDecision(ctx, relationshipmemory.ConsumerDigitalHuman, identity, decision)
	// Policy is rechecked at the product edge so an installation owner can
	// never become a channel end-user by accident.
	if !decision.Read || event.Origin.Kind != invocation.OriginManual {
		return nil, nil
	}
	relationship, err := p.service.EnsureRelationship(ctx, relationshipmemory.EnsureRelationshipRequest{
		OrganizationID:       run.OrganizationID,
		WorkspaceID:          run.WorkspaceID,
		Subject:              relationshipmemory.AccountSubject(*run.ActorID),
		ConsumerType:         relationshipmemory.ConsumerDigitalHuman,
		ConsumerID:           run.DigitalHumanID,
		ConsumerNameSnapshot: resolved.Snapshot.IdentityName,
	})
	if err != nil {
		return nil, err
	}
	contract := relationshipmemory.ConsumerMemoryContract{
		Enabled:            true,
		DefinitionMode:     "dynamic",
		AllowedCategories:  []string{"profile", "preference", "constraint", "relationship"},
		AutoCaptureAllowed: resolved.Snapshot.UserMemoryAutoCaptureAllowed && relationshipmemory.AutomaticExtractionEnabled(),
		ConfigRevision:     resolved.Snapshot.Revision,
		RenderBudgetChars:  4000,
	}
	contextBlock, _, err := p.service.RenderContext(ctx, relationship.ID, contract)
	if err != nil {
		return nil, err
	}
	preparation := &MemoryRuntimePreparation{SystemPromptAddendum: renderUserRelationshipMemoryGuidance(contextBlock, relationship.AutoCaptureEnabled && contract.AutoCaptureAllowed)}
	if relationship.Status == relationshipmemory.RelationshipActive && relationship.MemoryEnabled {
		preparation.Tools = []skillloop.RuntimeTool{p.mutationTool(run, relationship, contract)}
	}
	return preparation, nil
}

func renderUserRelationshipMemoryGuidance(contextBlock string, proactiveAllowed bool) string {
	sections := []string{
		"User relationship memory belongs to the current user's relationship with this DigitalHuman. It is separate from the DigitalHuman's own long-term memory and must never be copied or promoted between the two stores.",
	}
	if strings.TrimSpace(contextBlock) != "" {
		sections = append(sections, contextBlock)
	} else {
		sections = append(sections, "No user relationship memory is currently stored.")
	}
	mode := "Only explicit updates are allowed when the current user directly asks to remember, correct, or forget something."
	if proactiveAllowed {
		mode = "Explicit and proactive updates are allowed, but every update must be directly supported by the current user message."
	}
	sections = append(sections, mode,
		"Never store project, customer, channel, credential, third-party, or document-body data here. Never claim an update succeeded unless the relationship-memory tool reports committed.")
	return strings.Join(sections, "\n\n")
}

type relationshipMemoryToolArguments struct {
	Action           string `json:"action"`
	Mode             string `json:"mode"`
	Key              string `json:"key"`
	Category         string `json:"category"`
	Content          string `json:"content"`
	Evidence         string `json:"evidence"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (p *RelationshipMemoryRuntimeProvider) mutationTool(run *Run, relationship *relationshipmemory.UserMemoryRelationship, contract relationshipmemory.ConsumerMemoryContract) skillloop.RuntimeTool {
	modes := []string{"explicit"}
	if relationship.AutoCaptureEnabled && contract.AutoCaptureAllowed {
		modes = append(modes, "proactive")
	}
	definition := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        userRelationshipMemoryToolName,
		Description: "Update the current user's private relationship memory only when directly supported by the current user message. Use semantic keys and one of the allowed personal categories. Do not store project, customer, channel, credential, or third-party data.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":            map[string]any{"type": "string", "enum": []string{"upsert", "clear"}},
				"mode":              map[string]any{"type": "string", "enum": modes},
				"key":               map[string]any{"type": "string", "maxLength": 64},
				"category":          map[string]any{"type": "string", "enum": contract.AllowedCategories},
				"content":           map[string]any{"type": "string", "maxLength": 2000},
				"evidence":          map[string]any{"type": "string", "description": "An exact quote from the current user message."},
				"expected_revision": map[string]any{"type": "integer", "minimum": 0},
			},
			"required":             []string{"action", "mode", "key", "category", "evidence", "expected_revision"},
			"additionalProperties": false,
		},
	}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		args, err := decodeRelationshipMemoryToolArguments(call.Function.Arguments)
		if err != nil || args.Evidence == "" || !strings.Contains(run.Query, args.Evidence) {
			return relationshipMemoryToolFailure(relationshipmemory.ErrorCodePolicyRejected)
		}
		if args.Mode == "proactive" && !(relationship.AutoCaptureEnabled && contract.AutoCaptureAllowed) {
			return relationshipMemoryToolFailure(relationshipmemory.ErrorCodePolicyRejected)
		}
		if args.Mode == "proactive" && args.Action == "clear" {
			return relationshipMemoryToolFailure(relationshipmemory.ErrorCodePolicyRejected)
		}
		if args.Action == "clear" && args.Content != "" {
			return relationshipMemoryToolFailure(relationshipmemory.ErrorCodePolicyRejected)
		}
		operationID := uuid.NewSHA1(run.ID, []byte(call.ID+"\x00"+call.Function.Arguments))
		expectedEpoch := relationship.MemoryEpoch
		if args.Action == "clear" {
			_, err = p.service.DeleteItem(ctx, relationship.ID, args.Key, args.ExpectedRevision, relationshipmemory.MutationMetadata{ActorType: "model", Source: "digital_human", SourceKind: relationshipmemory.SourceExplicit, OperationID: operationID, ExpectedMemoryEpoch: &expectedEpoch, EvidenceFromUserInput: true})
		} else if args.Action == "upsert" {
			_, err = p.service.PutItem(ctx, relationship.ID, relationshipmemory.PutItemRequest{Key: args.Key, DisplayName: args.Key, Category: args.Category, Content: args.Content, ExpectedRevision: args.ExpectedRevision, Contract: contract, Metadata: relationshipmemory.MutationMetadata{ActorType: "model", Source: "digital_human", SourceKind: sourceKindForRelationshipMode(args.Mode), OperationID: operationID, ExpectedMemoryEpoch: &expectedEpoch, EvidenceFromUserInput: true}})
		} else {
			err = relationshipmemory.ErrPolicyRejected
		}
		if err != nil {
			return relationshipMemoryToolFailure(relationshipMemoryErrorCode(err))
		}
		return skillloop.RuntimeToolResult{Status: "success", Arguments: map[string]any{"action": args.Action, "mode": args.Mode, "key": args.Key, "category": args.Category}, Result: map[string]any{"status": "committed", "operation_id": operationID.String()}}
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: skills.SkillDigitalHumanMemory, Handler: handler}
}

func decodeRelationshipMemoryToolArguments(raw string) (relationshipMemoryToolArguments, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var args relationshipMemoryToolArguments
	if err := decoder.Decode(&args); err != nil {
		return args, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return args, relationshipmemory.ErrPolicyRejected
	}
	args.Action = strings.ToLower(strings.TrimSpace(args.Action))
	args.Mode = strings.ToLower(strings.TrimSpace(args.Mode))
	args.Key = strings.TrimSpace(args.Key)
	args.Category = strings.ToLower(strings.TrimSpace(args.Category))
	args.Content = strings.TrimSpace(args.Content)
	args.Evidence = strings.TrimSpace(args.Evidence)
	if args.Mode != "explicit" && args.Mode != "proactive" {
		return args, relationshipmemory.ErrPolicyRejected
	}
	return args, nil
}

func sourceKindForRelationshipMode(mode string) string {
	if mode == "proactive" {
		return relationshipmemory.SourceAutomatic
	}
	return relationshipmemory.SourceExplicit
}

func relationshipMemoryErrorCode(err error) string {
	switch {
	case errors.Is(err, relationshipmemory.ErrRevisionConflict):
		return relationshipmemory.ErrorCodeRevisionConflict
	case errors.Is(err, relationshipmemory.ErrCapacityExceeded):
		return relationshipmemory.ErrorCodeCapacityExceeded
	case errors.Is(err, relationshipmemory.ErrAccessDenied):
		return relationshipmemory.ErrorCodeAccessDenied
	default:
		return relationshipmemory.ErrorCodePolicyRejected
	}
}

func relationshipMemoryToolFailure(code string) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{Status: "failed", Arguments: map[string]any{}, Result: map[string]any{"status": "rejected", "error_code": code}, Error: relationshipmemory.ErrPolicyRejected, Recoverable: false}
}
