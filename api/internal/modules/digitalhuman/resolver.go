package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
)

const executionSnapshotSchemaV1 = "digital_human_execution_snapshot.v1"

// ResolverStore is the immutable read surface needed to prepare one execution.
type ResolverStore interface {
	RuntimePublishedVersion(context.Context, runtimeidentity.SubjectRef, *uuid.UUID) (*Definition, *Version, error)
	resourcebindings.Reader
}

// ExecutionSnapshot contains all model-facing configuration required by a worker.
// It deliberately contains credential references only, never secret material.
type ExecutionSnapshot struct {
	FileUploadPolicy             *chatfile.Policy           `json:"file_upload_policy,omitempty"`
	SchemaVersion                string                     `json:"schema_version"`
	Revision                     string                     `json:"revision"`
	Subject                      runtimeidentity.SubjectRef `json:"subject"`
	DigitalHumanVersion          uuid.UUID                  `json:"digital_human_version_id"`
	IdentityName                 string                     `json:"identity_name"`
	Persona                      string                     `json:"persona"`
	Goals                        []string                   `json:"goals"`
	Rules                        []string                   `json:"rules"`
	Model                        string                     `json:"model"`
	Bindings                     []resourcebindings.Binding `json:"bindings,omitempty"`
	UseUserMemory                bool                       `json:"use_user_memory"`
	UseAgentMemory               bool                       `json:"use_agent_memory"`
	UserMemoryAutoCaptureAllowed bool                       `json:"user_memory_auto_capture_allowed"`
	// Phase is "draft" for a configuration preview; published runs leave it
	// empty so their snapshot revisions are unchanged.
	Phase                 string                  `json:"phase,omitempty"`
	ChannelResponsePolicy *ChannelResponsePolicy  `json:"channel_response_policy,omitempty"`
	ExecutionRuntime      *ExecutionRuntimeConfig `json:"execution_runtime,omitempty"`
	ResolvedAt            time.Time               `json:"resolved_at"`
}

type ResolvedExecution struct {
	Spec     runspec.Spec      `json:"run_spec"`
	Snapshot ExecutionSnapshot `json:"execution_snapshot"`
}

// Resolver prepares a sealed RunSpec and a digest-bound execution snapshot.
type Resolver struct {
	store ResolverStore
	now   func() time.Time
}

func NewResolver(store ResolverStore) *Resolver {
	if store == nil {
		panic("DigitalHuman resolver requires store")
	}
	return &Resolver{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Resolver) Resolve(ctx context.Context, event invocation.Event, versionID *uuid.UUID) (*ResolvedExecution, error) {
	event, err := invocation.ValidateEvent(event)
	if err != nil {
		return nil, fmt.Errorf("validate DigitalHuman invocation: %w", err)
	}
	if event.Target.Type != runtimeidentity.SubjectTypeDigitalHuman {
		return nil, fmt.Errorf("%w: invocation target is not a DigitalHuman", ErrInvalidRequest)
	}
	definition, version, err := r.store.RuntimePublishedVersion(ctx, event.Target, versionID)
	if err != nil {
		return nil, err
	}
	if definition.Status != StatusActive {
		return nil, ErrPublishInvalid
	}
	return r.resolveVersion(ctx, event, definition, version, resourcebindings.PhasePublished)
}

// draftVersionStore reads the editable draft of a DigitalHuman.
type draftVersionStore interface {
	RuntimeDraftVersion(context.Context, runtimeidentity.SubjectRef) (*Definition, *Version, error)
}

// ResolveDraft prepares a preview of the unpublished draft. The draft must be
// complete enough to publish; it need not have been published before.
func (r *Resolver) ResolveDraft(ctx context.Context, event invocation.Event) (*ResolvedExecution, error) {
	event, err := invocation.ValidateEvent(event)
	if err != nil {
		return nil, fmt.Errorf("validate DigitalHuman invocation: %w", err)
	}
	if event.Target.Type != runtimeidentity.SubjectTypeDigitalHuman {
		return nil, fmt.Errorf("%w: invocation target is not a DigitalHuman", ErrInvalidRequest)
	}
	store, ok := r.store.(draftVersionStore)
	if !ok {
		return nil, ErrVersionNotFound
	}
	definition, version, err := store.RuntimeDraftVersion(ctx, event.Target)
	if err != nil {
		return nil, err
	}
	if err := validatePublishable(version); err != nil {
		return nil, err
	}
	return r.resolveVersion(ctx, event, definition, version, resourcebindings.PhaseDraft)
}

func (r *Resolver) resolveVersion(ctx context.Context, event invocation.Event, definition *Definition, version *Version, phase resourcebindings.Phase) (*ResolvedExecution, error) {
	var err error
	policyRefs := make([]runspec.VersionRef, 0)
	snapshotRef := resourcebindings.SnapshotRef{Subject: event.Target, Phase: phase, VersionID: &version.ID}
	bindings, err := r.store.ListSnapshot(ctx, snapshotRef)
	if err != nil {
		return nil, err
	}
	rules := make([]string, 0)
	if err := json.Unmarshal(version.Rules, &rules); err != nil {
		return nil, fmt.Errorf("decode DigitalHuman rules: %w", err)
	}
	goals := make([]string, 0)
	if len(version.Goals) > 0 {
		if err := json.Unmarshal(version.Goals, &goals); err != nil {
			return nil, fmt.Errorf("decode DigitalHuman goals: %w", err)
		}
	}
	identityName := strings.TrimSpace(version.IdentityName)
	if identityName == "" {
		// Compatibility for versions created before runtime identity was split
		// from workspace display metadata.
		identityName = definition.Name
	}
	now := r.now()
	userMemoryEnabled := invocationAllowsUserMemory(event)
	snapshot := ExecutionSnapshot{
		FileUploadPolicy: chatfile.Clone(version.FileUploadPolicy),
		SchemaVersion:    executionSnapshotSchemaV1, Subject: event.Target, DigitalHumanVersion: version.ID,
		IdentityName: identityName, Persona: version.Persona, Goals: goals, Rules: rules, Model: version.DefaultModel, Bindings: bindings,
		UseUserMemory: userMemoryEnabled, UseAgentMemory: false,
		// Kept in the v1 snapshot for compatibility with persisted runs. DigitalHuman
		// relationship memory is a product capability now; the user's relationship
		// setting is the only per-user automatic-capture control.
		UserMemoryAutoCaptureAllowed: userMemoryEnabled,
		ChannelResponsePolicy:        channelResponsePolicyPointer(channelResponsePolicyFromTriggerPolicy(version.TriggerPolicy)),
		ExecutionRuntime:             cloneExecutionRuntime(version.ExecutionRuntime),
		ResolvedAt:                   now,
	}
	if phase == resourcebindings.PhaseDraft {
		// A preview neither remembers its user nor runs in a Linux sandbox.
		snapshot.Phase, snapshot.UseUserMemory, snapshot.UserMemoryAutoCaptureAllowed, snapshot.ExecutionRuntime = string(phase), false, false, nil
	}
	snapshot.Revision, err = executionSnapshotRevision(snapshot)
	if err != nil {
		return nil, err
	}
	policyRefs = append(policyRefs, runspec.VersionRef{Kind: "execution_snapshot", ID: version.ID.String(), Revision: snapshot.Revision})
	runBindings := make([]runspec.BindingRef, 0, len(bindings))
	for _, binding := range bindings {
		runBindings = append(runBindings, runspec.BindingRef{
			Type: string(binding.Type), ResourceID: binding.ResourceID, ParentResourceID: binding.ParentResourceID,
			Revision: binding.Revision, AccessMode: runspec.AccessMode(binding.AccessMode), CredentialRef: binding.CredentialRef,
		})
	}
	spec, err := runspec.Seal(runspec.Spec{
		InvocationID: event.ID, Subject: event.Target,
		Identity: runspec.VersionRef{Kind: "identity", ID: version.ID.String(), Version: strconv.Itoa(version.VersionNumber), Revision: version.Revision},
		Model:    runspec.VersionRef{Kind: "model", ID: version.DefaultModel, Revision: version.Revision},
		Bindings: runBindings, Policies: dedupeVersionRefs(policyRefs),
		Budget:     runspec.Budget{MaxDurationSeconds: 300, MaxModelTokens: 16_000, MaxToolCalls: 32, MaxExternalActions: 8},
		ResolvedAt: now,
	})
	if err != nil {
		return nil, err
	}
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, nil
}

func invocationAllowsUserMemory(event invocation.Event) bool {
	surface, _ := event.Context["surface"].(string)
	switch strings.TrimSpace(surface) {
	case SurfaceConsole:
		return event.Sender.Type == invocation.PrincipalAccount
	case SurfaceWebApp:
		return event.Sender.Type == invocation.PrincipalAccount
	case SurfaceAPI:
		return event.Sender.Type == invocation.PrincipalService
	default:
		// Legacy console invocations predate the explicit surface field. Keep
		// those account-authenticated manual calls memory-capable without
		// broadening channel, task, or external-user access.
		return event.Type == invocation.EventTypeManual &&
			event.Origin.Kind == invocation.OriginManual &&
			event.Sender.Type == invocation.PrincipalAccount
	}
}

func executionSnapshotRevision(snapshot ExecutionSnapshot) (string, error) {
	snapshot.Revision = ""
	return hashRevision(snapshot)
}

func dedupeVersionRefs(refs []runspec.VersionRef) []runspec.VersionRef {
	seen := make(map[string]struct{}, len(refs))
	out := make([]runspec.VersionRef, 0, len(refs))
	for _, ref := range refs {
		key := strings.Join([]string{ref.Kind, ref.ID, ref.Version, ref.Revision}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	slices.SortFunc(out, func(a, b runspec.VersionRef) int {
		return strings.Compare(strings.Join([]string{a.Kind, a.ID, a.Version, a.Revision}, "\x00"), strings.Join([]string{b.Kind, b.ID, b.Version, b.Revision}, "\x00"))
	})
	return out
}
