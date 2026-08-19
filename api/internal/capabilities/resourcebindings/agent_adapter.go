package resourcebindings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

type agentBindingReader interface {
	ListScope(context.Context, agentbindings.ScopeRef) ([]agentbindings.Binding, error)
	FindBinding(context.Context, agentbindings.ScopeRef, agentbindings.Match) (*agentbindings.Binding, error)
}

// AgentAdapter preserves the existing Agent store while exposing the neutral contract.
type AgentAdapter struct {
	reader agentBindingReader
}

func NewAgentAdapter(reader agentBindingReader) *AgentAdapter {
	if reader == nil {
		panic("resource binding Agent adapter requires reader")
	}
	return &AgentAdapter{reader: reader}
}

func (a *AgentAdapter) ListSnapshot(ctx context.Context, ref SnapshotRef) ([]Binding, error) {
	ref, scopeRef, err := agentScopeRef(ref)
	if err != nil {
		return nil, err
	}
	rows, err := a.reader.ListScope(ctx, scopeRef)
	if err != nil {
		return nil, fmt.Errorf("list Agent binding snapshot: %w", err)
	}
	bindings := make([]Binding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, projectAgentBinding(ref, row))
	}
	return bindings, nil
}

func (a *AgentAdapter) Verify(ctx context.Context, ref SnapshotRef, match Match) (Decision, error) {
	ref, scopeRef, err := agentScopeRef(ref)
	if err != nil {
		return Decision{}, err
	}
	match, err = NormalizeMatch(match)
	if err != nil {
		return Decision{}, err
	}
	row, err := a.reader.FindBinding(ctx, scopeRef, agentbindings.Match{
		BindingType:      agentbindings.BindingType(match.Type),
		ResourceID:       match.ResourceID,
		ParentResourceID: match.ParentResourceID,
		AccessMode:       string(match.AccessMode),
	})
	if err != nil {
		return Decision{}, fmt.Errorf("verify Agent resource binding: %w", err)
	}
	if row == nil {
		return Decision{Reason: "resource_not_bound"}, nil
	}
	binding := projectAgentBinding(ref, *row)
	if match.ActionID != "" && !AllowsIntegrationAction(binding.Metadata, match.ActionID) {
		return Decision{Binding: &binding, Reason: "integration_action_not_allowed"}, nil
	}
	return Decision{Allowed: true, Binding: &binding}, nil
}

func agentScopeRef(ref SnapshotRef) (SnapshotRef, agentbindings.ScopeRef, error) {
	ref, err := ValidateSnapshotRef(ref)
	if err != nil {
		return SnapshotRef{}, agentbindings.ScopeRef{}, err
	}
	if ref.Subject.Type != runtimeidentity.SubjectTypeAgent {
		return SnapshotRef{}, agentbindings.ScopeRef{}, fmt.Errorf("%w: Agent adapter requires Agent subject", ErrInvalidReference)
	}
	scope := agentbindings.ScopeDraft
	var versionID = ref.VersionID
	if ref.Phase == PhasePublished {
		scope = agentbindings.ScopePublished
	}
	return ref, agentbindings.ScopeRef{AgentID: ref.Subject.ID, Scope: scope, PublishedVersionUUID: versionID}, nil
}

func projectAgentBinding(ref SnapshotRef, row agentbindings.Binding) Binding {
	metadata := cloneMetadata(row.Metadata)
	return Binding{
		ID: row.ID, Snapshot: ref, Type: BindingType(row.BindingType), ResourceID: row.ResourceID,
		ParentResourceID: row.ParentResourceID, DisplayName: row.DisplayName,
		AccessMode:    AccessMode(strings.ToLower(strings.TrimSpace(row.AccessMode))),
		Revision:      bindingRevision(row.BindingType, row.ResourceID, row.ParentResourceID, row.AccessMode, metadata),
		Metadata:      metadata,
		Authorization: AuthorizationEvidence{AuthorizedBy: row.AuthorizedBy, AuthorizedAt: row.AuthorizedAt},
	}
}

func bindingRevision(bindingType agentbindings.BindingType, resourceID, parentID, accessMode string, metadata map[string]interface{}) string {
	payload := struct {
		Type       agentbindings.BindingType `json:"type"`
		ResourceID string                    `json:"resource_id"`
		ParentID   string                    `json:"parent_resource_id"`
		AccessMode string                    `json:"access_mode"`
		Metadata   map[string]interface{}    `json:"metadata"`
	}{bindingType, resourceID, parentID, accessMode, metadata}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil
	}
	return cloned
}
