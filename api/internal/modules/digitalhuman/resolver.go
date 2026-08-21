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
)

const executionSnapshotSchemaV1 = "digital_human_execution_snapshot.v1"

// ResolverStore is the immutable read surface needed to prepare one execution.
type ResolverStore interface {
	RuntimePublishedVersion(context.Context, runtimeidentity.SubjectRef, *uuid.UUID) (*Definition, *Version, error)
	ListRoleAssignments(context.Context, Scope, uuid.UUID, uuid.UUID) ([]*RoleAssignment, error)
	PublishedRoleTemplateVersion(context.Context, Scope, uuid.UUID) (*RoleTemplateVersion, error)
	resourcebindings.Reader
}

// ResolvedRole contains the exact published role and assignment applied to a run.
type ResolvedRole struct {
	Assignment *RoleAssignment      `json:"assignment"`
	Version    *RoleTemplateVersion `json:"version"`
}

// ExecutionSnapshot contains all model-facing configuration required by a worker.
// It deliberately contains credential references only, never secret material.
type ExecutionSnapshot struct {
	SchemaVersion       string                     `json:"schema_version"`
	Revision            string                     `json:"revision"`
	Subject             runtimeidentity.SubjectRef `json:"subject"`
	DigitalHumanVersion uuid.UUID                  `json:"digital_human_version_id"`
	Name                string                     `json:"name"`
	Avatar              string                     `json:"avatar,omitempty"`
	Description         string                     `json:"description,omitempty"`
	Persona             string                     `json:"persona"`
	Rules               []string                   `json:"rules"`
	Model               string                     `json:"model"`
	Roles               []ResolvedRole             `json:"roles,omitempty"`
	Bindings            []resourcebindings.Binding `json:"bindings,omitempty"`
	UseUserMemory       bool                       `json:"use_user_memory"`
	UseAgentMemory      bool                       `json:"use_agent_memory"`
	ChannelResponsePolicy *ChannelResponsePolicy    `json:"channel_response_policy,omitempty"`
	ResolvedAt          time.Time                  `json:"resolved_at"`
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
	scope := Scope{OrganizationID: event.Target.OrganizationID, WorkspaceID: event.Target.WorkspaceID}
	assignments, err := r.store.ListRoleAssignments(ctx, scope, definition.ID, version.ID)
	if err != nil {
		return nil, err
	}
	roles := make([]ResolvedRole, 0, len(assignments))
	roleRefs := make([]runspec.VersionRef, 0, len(assignments))
	capabilityRefs := make([]runspec.VersionRef, 0)
	sopRefs := make([]runspec.VersionRef, 0)
	policyRefs := make([]runspec.VersionRef, 0)
	evaluationRefs := make([]runspec.VersionRef, 0)
	for _, assignment := range assignments {
		if !assignmentEffectiveAt(assignment, event.OccurredAt) {
			continue
		}
		roleVersion, roleErr := r.store.PublishedRoleTemplateVersion(ctx, scope, assignment.RoleTemplateVersionID)
		if roleErr != nil {
			return nil, roleErr
		}
		roles = append(roles, ResolvedRole{Assignment: cloneRoleAssignment(assignment), Version: cloneRoleTemplateVersion(roleVersion)})
		roleRefs = append(roleRefs, runspec.VersionRef{
			Kind: "role", ID: roleVersion.RoleTemplateID.String(), Version: strconv.Itoa(roleVersion.VersionNumber), Revision: roleVersion.Revision,
		})
		if err := appendRoleVersionRefs(roleVersion, &capabilityRefs, &sopRefs, &evaluationRefs); err != nil {
			return nil, err
		}
		policyRefs = append(policyRefs, runspec.VersionRef{Kind: "policy", ID: roleVersion.ID.String(), Revision: roleVersion.Revision})
	}
	snapshotRef := resourcebindings.SnapshotRef{Subject: event.Target, Phase: resourcebindings.PhasePublished, VersionID: &version.ID}
	bindings, err := r.store.ListSnapshot(ctx, snapshotRef)
	if err != nil {
		return nil, err
	}
	rules := make([]string, 0)
	if err := json.Unmarshal(version.Rules, &rules); err != nil {
		return nil, fmt.Errorf("decode DigitalHuman rules: %w", err)
	}
	now := r.now()
	snapshot := ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Subject: event.Target, DigitalHumanVersion: version.ID,
		Name: definition.Name, Avatar: definition.Avatar, Description: definition.Description,
		Persona: version.Persona, Rules: rules, Model: version.DefaultModel, Roles: roles, Bindings: bindings,
		UseUserMemory: false, UseAgentMemory: false,
		ChannelResponsePolicy: channelResponsePolicyPointer(channelResponsePolicyFromTriggerPolicy(version.TriggerPolicy)),
		ResolvedAt: now,
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
		Roles:    roleRefs, Capabilities: dedupeVersionRefs(capabilityRefs), SOPs: dedupeVersionRefs(sopRefs),
		Bindings: runBindings, Policies: dedupeVersionRefs(policyRefs), Evaluations: dedupeVersionRefs(evaluationRefs),
		Budget:     runspec.Budget{MaxDurationSeconds: 300, MaxModelTokens: 16_000, MaxToolCalls: 32, MaxExternalActions: 8},
		ResolvedAt: now,
	})
	if err != nil {
		return nil, err
	}
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, nil
}

func executionSnapshotRevision(snapshot ExecutionSnapshot) (string, error) {
	snapshot.Revision = ""
	return hashRevision(snapshot)
}

func assignmentEffectiveAt(assignment *RoleAssignment, at time.Time) bool {
	if assignment.EffectiveFrom != nil && at.Before(*assignment.EffectiveFrom) {
		return false
	}
	return assignment.EffectiveTo == nil || at.Before(*assignment.EffectiveTo)
}

func appendRoleVersionRefs(role *RoleTemplateVersion, capabilities, sops, evaluations *[]runspec.VersionRef) error {
	for _, item := range []struct {
		encoded []byte
		target  *[]runspec.VersionRef
	}{
		{encoded: role.Capabilities, target: capabilities},
		{encoded: role.SOPRefs, target: sops},
		{encoded: role.EvaluationRefs, target: evaluations},
	} {
		encoded, target := string(item.encoded), item.target
		if strings.TrimSpace(encoded) == "" || strings.TrimSpace(encoded) == "null" {
			continue
		}
		var refs []runspec.VersionRef
		if err := json.Unmarshal([]byte(encoded), &refs); err != nil {
			return fmt.Errorf("decode DigitalHuman role references: %w", err)
		}
		*target = append(*target, refs...)
	}
	return nil
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

func cloneRoleAssignment(value *RoleAssignment) *RoleAssignment {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Parameters = jsonClone(value.Parameters)
	cloned.OverridePolicy = jsonClone(value.OverridePolicy)
	return &cloned
}

func cloneRoleTemplateVersion(value *RoleTemplateVersion) *RoleTemplateVersion {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Goals, cloned.Responsibilities = jsonClone(value.Goals), jsonClone(value.Responsibilities)
	cloned.Capabilities, cloned.SOPRefs = jsonClone(value.Capabilities), jsonClone(value.SOPRefs)
	cloned.KnowledgeRefs, cloned.Policy = jsonClone(value.KnowledgeRefs), jsonClone(value.Policy)
	cloned.EvaluationRefs = jsonClone(value.EvaluationRefs)
	return &cloned
}
