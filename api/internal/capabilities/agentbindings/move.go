package agentbindings

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const WorkspaceMoveBindingOperation = "move_workspace_assets"

type MoveImpactRequest struct {
	OrganizationID    uuid.UUID
	TargetWorkspaceID uuid.UUID
	ResourceRefs      []ResourceRef
	MovingAgentIDs    []uuid.UUID
	ActorID           uuid.UUID
}

// MoveDependencyRequest describes a move batch before a target workspace is
// selected. It is used only to report Agent bindings that the move may affect;
// the target-specific preview and signed impact token are still produced by
// PreviewMoveImpact after the user confirms a target workspace.
type MoveDependencyRequest struct {
	OrganizationID uuid.UUID
	ResourceRefs   []ResourceRef
	MovingAgentIDs []uuid.UUID
}

type moveImpactTokenRef struct {
	BindingType      BindingType `json:"binding_type"`
	ResourceID       string      `json:"resource_id"`
	ParentResourceID string      `json:"parent_resource_id,omitempty"`
	WorkspaceID      string      `json:"workspace_id,omitempty"`
}

type moveImpactTokenPayload struct {
	ActorID           string               `json:"actor_id"`
	Operation         string               `json:"operation"`
	OrganizationID    string               `json:"organization_id"`
	TargetWorkspaceID string               `json:"target_workspace_id"`
	ResourceRefs      []moveImpactTokenRef `json:"resource_refs"`
	MovingAgentIDs    []string             `json:"moving_agent_ids"`
	Revision          string               `json:"revision"`
	ExpiresAt         int64                `json:"expires_at"`
}

func (r *Repository) PreviewMoveImpact(ctx context.Context, req MoveImpactRequest, now time.Time) (*Impact, error) {
	normalized, err := normalizeMoveImpactRequest(req)
	if err != nil {
		return nil, err
	}
	bindings, err := r.listMoveImpactBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	payload := moveImpactPayload(normalized, bindings, now.Add(ImpactTokenTTL))
	token, err := encodeMoveImpactToken(r.tokenSecret, payload)
	if err != nil {
		return nil, err
	}
	agents, err := r.impactAgents(ctx, bindings)
	if err != nil {
		return nil, err
	}
	return &Impact{
		Code:        ConflictCodeResourceBound,
		Operation:   WorkspaceMoveBindingOperation,
		ResourceID:  normalized.TargetWorkspaceID.String(),
		Agents:      agents,
		ImpactToken: token,
		ExpiresAt:   payload.ExpiresAt,
	}, nil
}

// PreviewMoveDependencies returns the Agents whose bindings may be affected by
// a move batch without treating the result as approval to mutate bindings.
func (r *Repository) PreviewMoveDependencies(ctx context.Context, req MoveDependencyRequest) ([]ImpactAgent, error) {
	normalized, err := normalizeMoveDependencyRequest(req)
	if err != nil {
		return nil, err
	}
	bindings, err := r.listMoveImpactBindings(ctx, MoveImpactRequest{
		OrganizationID: normalized.OrganizationID,
		ResourceRefs:   normalized.ResourceRefs,
		MovingAgentIDs: normalized.MovingAgentIDs,
	})
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []ImpactAgent{}, nil
	}
	return r.impactAgents(ctx, bindings)
}

func (r *Repository) VerifyMoveImpactToken(ctx context.Context, req MoveImpactRequest, token string, now time.Time) error {
	normalized, err := normalizeMoveImpactRequest(req)
	if err != nil {
		return err
	}
	payload, err := decodeMoveImpactToken(r.tokenSecret, token)
	if err != nil {
		return err
	}
	bindings, err := r.listMoveImpactBindings(ctx, normalized)
	if err != nil {
		return err
	}
	expected := moveImpactPayload(normalized, bindings, time.Unix(payload.ExpiresAt, 0))
	if payload.ExpiresAt < now.Unix() || !moveImpactPayloadEqual(payload, expected) {
		return ErrImpactTokenInvalid
	}
	return nil
}

// ApplyMoveImpact verifies the complete move batch, removes only affected Agent
// bindings, and moves the surviving bindings for co-moved Agents to the target
// workspace. The caller must pass the transaction used for the asset move.
func (r *Repository) ApplyMoveImpact(ctx context.Context, tx *gorm.DB, req MoveImpactRequest, token string, now time.Time) ([]uuid.UUID, error) {
	if tx == nil {
		return nil, fmt.Errorf("agent binding transaction is required")
	}
	normalized, err := normalizeMoveImpactRequest(req)
	if err != nil {
		return nil, err
	}
	txRepo := r.WithTx(tx)
	preLockBindings, err := txRepo.listMoveImpactBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	lockRefs := moveImpactResourceLockRefs(normalized, preLockBindings)
	if err := txRepo.LockResources(ctx, tx, lockRefs); err != nil {
		return nil, err
	}
	if err := txRepo.LockAgents(ctx, tx, normalized.MovingAgentIDs); err != nil {
		return nil, err
	}
	bindings, err := txRepo.listMoveImpactBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if bindingImpactRevision(bindings) != bindingImpactRevision(preLockBindings) {
		return nil, ErrImpactChanged
	}
	if len(bindings) > 0 {
		if err := txRepo.VerifyMoveImpactToken(ctx, normalized, token, now); err != nil {
			return nil, err
		}
	}

	affectedAgents := map[uuid.UUID]struct{}{}
	seen := map[string]struct{}{}
	for _, binding := range bindings {
		key := strings.Join([]string{binding.AgentID.String(), string(binding.BindingType), binding.ResourceID, binding.ParentResourceID}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		agentID := binding.AgentID
		ref := ResourceRef{
			OrganizationID:   normalized.OrganizationID,
			AgentID:          &agentID,
			BindingType:      binding.BindingType,
			ResourceID:       binding.ResourceID,
			ParentResourceID: binding.ParentResourceID,
		}
		pruned, err := txRepo.RevokeAndPruneDrafts(ctx, tx, ref, normalized.ActorID)
		if err != nil {
			return nil, err
		}
		for _, prunedAgentID := range pruned {
			affectedAgents[prunedAgentID] = struct{}{}
		}
	}

	if len(normalized.MovingAgentIDs) > 0 {
		if err := tx.WithContext(ctx).Model(&Binding{}).
			Where("organization_id = ? AND agent_id IN ?", normalized.OrganizationID, normalized.MovingAgentIDs).
			Update("workspace_id", normalized.TargetWorkspaceID).Error; err != nil {
			return nil, fmt.Errorf("move surviving agent bindings to target workspace: %w", err)
		}
	}
	out := make([]uuid.UUID, 0, len(affectedAgents))
	for agentID := range affectedAgents {
		out = append(out, agentID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func moveImpactResourceLockRefs(req MoveImpactRequest, bindings []Binding) []ResourceRef {
	refs := make([]ResourceRef, 0, len(req.ResourceRefs)+len(bindings))
	for _, ref := range req.ResourceRefs {
		ref.OrganizationID = req.OrganizationID
		refs = append(refs, ref)
	}
	for _, binding := range bindings {
		refs = append(refs, ResourceRef{
			OrganizationID:   req.OrganizationID,
			BindingType:      binding.BindingType,
			ResourceID:       binding.ResourceID,
			ParentResourceID: binding.ParentResourceID,
		})
	}
	return refs
}

func (r *Repository) listMoveImpactBindings(ctx context.Context, req MoveImpactRequest) ([]Binding, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	resourceBindings := make([]Binding, 0)
	for _, ref := range req.ResourceRefs {
		query := r.db.WithContext(ctx).Where("organization_id = ?", req.OrganizationID)
		if ref.BindingType == BindingTypeDatabase {
			query = query.Where(
				"(binding_type = ? AND resource_id = ?) OR (binding_type = ? AND parent_resource_id = ?)",
				BindingTypeDatabase, ref.ResourceID, BindingTypeDatabaseTable, ref.ResourceID,
			)
		} else if ref.BindingType == BindingTypeWorkflow {
			query = query.Where(
				"binding_type = ? AND (resource_id = ? OR parent_resource_id = ?)",
				BindingTypeWorkflow, ref.ResourceID, ref.ResourceID,
			)
		} else {
			query = query.Where("binding_type = ? AND resource_id = ?", ref.BindingType, ref.ResourceID)
			if ref.ParentResourceID != "" {
				query = query.Where("parent_resource_id = ?", ref.ParentResourceID)
			}
		}
		var bindings []Binding
		if err := query.Find(&bindings).Error; err != nil {
			return nil, fmt.Errorf("list bindings for moving resource: %w", err)
		}
		resourceBindings = append(resourceBindings, bindings...)
	}

	agentBindings := make([]Binding, 0)
	if len(req.MovingAgentIDs) > 0 {
		if err := r.db.WithContext(ctx).
			Where("organization_id = ? AND agent_id IN ? AND binding_type <> ?", req.OrganizationID, req.MovingAgentIDs, BindingTypeSkill).
			Find(&agentBindings).Error; err != nil {
			return nil, fmt.Errorf("list bindings for moving agents: %w", err)
		}
	}
	return classifyMoveImpactBindings(req, resourceBindings, agentBindings), nil
}

func classifyMoveImpactBindings(req MoveImpactRequest, resourceBindings, agentBindings []Binding) []Binding {
	movingAgents := make(map[uuid.UUID]struct{}, len(req.MovingAgentIDs))
	for _, agentID := range req.MovingAgentIDs {
		movingAgents[agentID] = struct{}{}
	}
	movingResources := make(map[string]struct{}, len(req.ResourceRefs))
	for _, ref := range req.ResourceRefs {
		movingResources[moveResourceKey(ref.BindingType, ref.ResourceID, ref.ParentResourceID)] = struct{}{}
	}

	impacted := map[string]Binding{}
	add := func(binding Binding) {
		key := binding.ID.String()
		if binding.ID == uuid.Nil {
			version := ""
			if binding.PublishedVersionUUID != nil {
				version = binding.PublishedVersionUUID.String()
			}
			key = strings.Join([]string{binding.AgentID.String(), string(binding.BindingScope), version, string(binding.BindingType), binding.ResourceID, binding.ParentResourceID}, "|")
		}
		impacted[key] = binding
	}

	for _, binding := range resourceBindings {
		if _, coMoved := movingAgents[binding.AgentID]; !coMoved {
			add(binding)
		}
	}

	for _, binding := range agentBindings {
		if binding.BindingType == BindingTypeSkill || moveBindingResourceIsIncluded(binding, movingResources) {
			continue
		}
		add(binding)
	}

	out := make([]Binding, 0, len(impacted))
	for _, binding := range impacted {
		out = append(out, binding)
	}
	sort.Slice(out, func(i, j int) bool {
		return moveBindingSortKey(out[i]) < moveBindingSortKey(out[j])
	})
	return out
}

func normalizeMoveImpactRequest(req MoveImpactRequest) (MoveImpactRequest, error) {
	if req.OrganizationID == uuid.Nil || req.TargetWorkspaceID == uuid.Nil || req.ActorID == uuid.Nil {
		return MoveImpactRequest{}, fmt.Errorf("organization, target workspace, and actor are required")
	}
	return normalizeMoveImpactResources(req)
}

func normalizeMoveDependencyRequest(req MoveDependencyRequest) (MoveDependencyRequest, error) {
	if req.OrganizationID == uuid.Nil {
		return MoveDependencyRequest{}, fmt.Errorf("organization is required")
	}
	normalized, err := normalizeMoveImpactResources(MoveImpactRequest{
		OrganizationID: req.OrganizationID,
		ResourceRefs:   req.ResourceRefs,
		MovingAgentIDs: req.MovingAgentIDs,
	})
	if err != nil {
		return MoveDependencyRequest{}, err
	}
	return MoveDependencyRequest{
		OrganizationID: normalized.OrganizationID,
		ResourceRefs:   normalized.ResourceRefs,
		MovingAgentIDs: normalized.MovingAgentIDs,
	}, nil
}

func normalizeMoveImpactResources(req MoveImpactRequest) (MoveImpactRequest, error) {
	if len(req.ResourceRefs) == 0 && len(req.MovingAgentIDs) == 0 {
		return MoveImpactRequest{}, fmt.Errorf("moving resources or agents are required")
	}
	refMap := map[string]ResourceRef{}
	for _, ref := range req.ResourceRefs {
		ref.ResourceID = strings.TrimSpace(ref.ResourceID)
		ref.ParentResourceID = strings.TrimSpace(ref.ParentResourceID)
		if ref.BindingType == "" || ref.BindingType == BindingTypeSkill || ref.ResourceID == "" {
			return MoveImpactRequest{}, fmt.Errorf("invalid moving resource reference")
		}
		if ref.OrganizationID != uuid.Nil && ref.OrganizationID != req.OrganizationID {
			return MoveImpactRequest{}, fmt.Errorf("moving resource organization mismatch")
		}
		ref.OrganizationID = req.OrganizationID
		refMap[moveTokenRefKey(ref)] = ref
	}
	req.ResourceRefs = make([]ResourceRef, 0, len(refMap))
	for _, ref := range refMap {
		req.ResourceRefs = append(req.ResourceRefs, ref)
	}
	sort.Slice(req.ResourceRefs, func(i, j int) bool {
		return moveTokenRefKey(req.ResourceRefs[i]) < moveTokenRefKey(req.ResourceRefs[j])
	})

	agentMap := map[uuid.UUID]struct{}{}
	for _, agentID := range req.MovingAgentIDs {
		if agentID == uuid.Nil {
			return MoveImpactRequest{}, fmt.Errorf("invalid moving agent id")
		}
		agentMap[agentID] = struct{}{}
	}
	req.MovingAgentIDs = make([]uuid.UUID, 0, len(agentMap))
	for agentID := range agentMap {
		req.MovingAgentIDs = append(req.MovingAgentIDs, agentID)
	}
	sort.Slice(req.MovingAgentIDs, func(i, j int) bool { return req.MovingAgentIDs[i].String() < req.MovingAgentIDs[j].String() })
	return req, nil
}

func moveImpactPayload(req MoveImpactRequest, bindings []Binding, expiresAt time.Time) moveImpactTokenPayload {
	refs := make([]moveImpactTokenRef, 0, len(req.ResourceRefs))
	for _, ref := range req.ResourceRefs {
		workspaceID := ""
		if ref.WorkspaceID != nil {
			workspaceID = ref.WorkspaceID.String()
		}
		refs = append(refs, moveImpactTokenRef{
			BindingType:      ref.BindingType,
			ResourceID:       ref.ResourceID,
			ParentResourceID: ref.ParentResourceID,
			WorkspaceID:      workspaceID,
		})
	}
	agentIDs := make([]string, 0, len(req.MovingAgentIDs))
	for _, agentID := range req.MovingAgentIDs {
		agentIDs = append(agentIDs, agentID.String())
	}
	return moveImpactTokenPayload{
		ActorID:           req.ActorID.String(),
		Operation:         WorkspaceMoveBindingOperation,
		OrganizationID:    req.OrganizationID.String(),
		TargetWorkspaceID: req.TargetWorkspaceID.String(),
		ResourceRefs:      refs,
		MovingAgentIDs:    agentIDs,
		Revision:          bindingImpactRevision(bindings),
		ExpiresAt:         expiresAt.Unix(),
	}
}

func moveImpactPayloadEqual(left, right moveImpactTokenPayload) bool {
	left.ExpiresAt = 0
	right.ExpiresAt = 0
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return hmac.Equal(leftJSON, rightJSON)
}

func encodeMoveImpactToken(secret []byte, payload moveImpactTokenPayload) (string, error) {
	if len(secret) == 0 {
		return "", ErrImpactTokenNotConfigured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode agent binding move impact token: %w", err)
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(digest.Sum(nil)), nil
}

func decodeMoveImpactToken(secret []byte, token string) (moveImpactTokenPayload, error) {
	if len(secret) == 0 {
		return moveImpactTokenPayload{}, ErrImpactTokenNotConfigured
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return moveImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return moveImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return moveImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	if !hmac.Equal(digest.Sum(nil), signature) {
		return moveImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	var payload moveImpactTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return moveImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	return payload, nil
}

func moveBindingResourceIsIncluded(binding Binding, resources map[string]struct{}) bool {
	if _, ok := resources[moveResourceKey(binding.BindingType, binding.ResourceID, binding.ParentResourceID)]; ok {
		return true
	}
	if binding.BindingType == BindingTypeDatabaseTable {
		_, ok := resources[moveResourceKey(BindingTypeDatabase, binding.ParentResourceID, "")]
		return ok
	}
	if binding.BindingType == BindingTypeWorkflow {
		if _, ok := resources[moveResourceKey(BindingTypeWorkflow, binding.ParentResourceID, "")]; ok {
			return true
		}
		_, ok := resources[moveResourceKey(BindingTypeWorkflow, binding.ResourceID, "")]
		return ok
	}
	return false
}

func moveResourceKey(bindingType BindingType, resourceID, parentID string) string {
	return strings.Join([]string{string(bindingType), strings.TrimSpace(resourceID), strings.TrimSpace(parentID)}, "|")
}

func moveTokenRefKey(ref ResourceRef) string {
	workspaceID := ""
	if ref.WorkspaceID != nil {
		workspaceID = ref.WorkspaceID.String()
	}
	return strings.Join([]string{moveResourceKey(ref.BindingType, ref.ResourceID, ref.ParentResourceID), workspaceID}, "|")
}

func moveBindingSortKey(binding Binding) string {
	version := ""
	if binding.PublishedVersionUUID != nil {
		version = binding.PublishedVersionUUID.String()
	}
	return strings.Join([]string{binding.AgentID.String(), string(binding.BindingScope), version, string(binding.BindingType), binding.ResourceID, binding.ParentResourceID}, "|")
}
