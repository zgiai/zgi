package agentbindings

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ImpactTokenTTL            = 5 * time.Minute
	ConflictCodeResourceBound = "agent_resource_bound"
)

var (
	ErrImpactTokenInvalid       = errors.New("agent binding impact token is invalid")
	ErrImpactTokenNotConfigured = errors.New("agent binding impact token secret is not configured")
	ErrImpactChanged            = errors.New("agent binding impact changed")
)

type ImpactAgent struct {
	AgentID     string `json:"agent_id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	IconType    string `json:"icon_type,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type Impact struct {
	Code        string        `json:"code"`
	Operation   string        `json:"operation"`
	BindingType BindingType   `json:"binding_type"`
	ResourceID  string        `json:"resource_id"`
	ResourceIDs []string      `json:"resource_ids,omitempty"`
	Agents      []ImpactAgent `json:"agents"`
	ImpactToken string        `json:"impact_token"`
	ExpiresAt   int64         `json:"expires_at"`
}

type ConflictError struct {
	Impact Impact
}

func (e *ConflictError) Error() string {
	return "resource is bound to agents"
}

type impactTokenPayload struct {
	ActorID     string      `json:"actor_id"`
	Operation   string      `json:"operation"`
	BindingType BindingType `json:"binding_type"`
	ResourceID  string      `json:"resource_id"`
	ParentID    string      `json:"parent_id,omitempty"`
	Revision    string      `json:"revision"`
	ExpiresAt   int64       `json:"expires_at"`
}

func (r *Repository) PreviewImpact(ctx context.Context, ref ResourceRef, operation string, actorID uuid.UUID, now time.Time) (*Impact, error) {
	bindings, err := r.ListImpact(ctx, ref)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	operation = strings.TrimSpace(operation)
	if operation == "" || actorID == uuid.Nil {
		return nil, fmt.Errorf("operation and actor id are required")
	}
	revision := bindingImpactRevision(bindings)
	payload := impactTokenPayload{
		ActorID:     actorID.String(),
		Operation:   operation,
		BindingType: ref.BindingType,
		ResourceID:  strings.TrimSpace(ref.ResourceID),
		ParentID:    strings.TrimSpace(ref.ParentResourceID),
		Revision:    revision,
		ExpiresAt:   now.Add(ImpactTokenTTL).Unix(),
	}
	token, err := encodeImpactToken(r.tokenSecret, payload)
	if err != nil {
		return nil, err
	}
	agents, err := r.impactAgents(ctx, bindings)
	if err != nil {
		return nil, err
	}
	return &Impact{
		Code:        ConflictCodeResourceBound,
		Operation:   operation,
		BindingType: ref.BindingType,
		ResourceID:  strings.TrimSpace(ref.ResourceID),
		Agents:      agents,
		ImpactToken: token,
		ExpiresAt:   payload.ExpiresAt,
	}, nil
}

func (r *Repository) VerifyImpactToken(ctx context.Context, ref ResourceRef, operation string, actorID uuid.UUID, token string, now time.Time) error {
	payload, err := decodeImpactToken(r.tokenSecret, token)
	if err != nil {
		return err
	}
	if payload.ExpiresAt < now.Unix() || payload.ActorID != actorID.String() || payload.Operation != strings.TrimSpace(operation) ||
		payload.BindingType != ref.BindingType || payload.ResourceID != strings.TrimSpace(ref.ResourceID) || payload.ParentID != strings.TrimSpace(ref.ParentResourceID) {
		return ErrImpactTokenInvalid
	}
	bindings, err := r.ListImpact(ctx, ref)
	if err != nil {
		return err
	}
	if bindingImpactRevision(bindings) != payload.Revision {
		return ErrImpactChanged
	}
	return nil
}

// RevokeAndPruneDrafts removes a resource from every affected draft configuration and
// revokes both draft and published effective binding rows in the caller's transaction.
func (r *Repository) RevokeAndPruneDrafts(ctx context.Context, tx *gorm.DB, ref ResourceRef, actorID uuid.UUID) ([]uuid.UUID, error) {
	if tx == nil {
		return nil, fmt.Errorf("agent binding transaction is required")
	}
	txRepo := r.WithTx(tx)
	if err := txRepo.LockResources(ctx, tx, []ResourceRef{ref}); err != nil {
		return nil, err
	}
	bindings, err := txRepo.ListImpact(ctx, ref)
	if err != nil {
		return nil, err
	}
	allAgentSet := map[uuid.UUID]struct{}{}
	draftAgentSet := map[uuid.UUID]struct{}{}
	for _, binding := range bindings {
		allAgentSet[binding.AgentID] = struct{}{}
		if binding.BindingScope == ScopeDraft {
			draftAgentSet[binding.AgentID] = struct{}{}
		}
	}
	allAgentIDs := make([]uuid.UUID, 0, len(allAgentSet))
	for agentID := range allAgentSet {
		allAgentIDs = append(allAgentIDs, agentID)
	}
	sort.Slice(allAgentIDs, func(i, j int) bool { return allAgentIDs[i].String() < allAgentIDs[j].String() })
	draftAgentIDs := make([]uuid.UUID, 0, len(draftAgentSet))
	for agentID := range draftAgentSet {
		draftAgentIDs = append(draftAgentIDs, agentID)
	}
	sort.Slice(draftAgentIDs, func(i, j int) bool { return draftAgentIDs[i].String() < draftAgentIDs[j].String() })
	if len(draftAgentIDs) > 0 {
		var configs []struct {
			ID        uuid.UUID
			AgentsID  uuid.UUID
			AgentMode *string
		}
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Table("agents_configs").
			Select("id, agents_id, agent_mode").
			Where("agents_id IN ? AND deleted_at IS NULL", draftAgentIDs).
			Order("agents_id ASC, id ASC").
			Find(&configs).Error; err != nil {
			return nil, fmt.Errorf("load agent draft configs for resource revoke: %w", err)
		}
		for _, config := range configs {
			updated, changed, err := pruneAgentModeResource(config.AgentMode, ref)
			if err != nil {
				return nil, fmt.Errorf("prune agent %s resource binding: %w", config.AgentsID, err)
			}
			if !changed {
				continue
			}
			updates := map[string]interface{}{"agent_mode": updated, "updated_at": time.Now()}
			if actorID != uuid.Nil {
				updates["updated_by"] = actorID
			}
			if err := tx.WithContext(ctx).Table("agents_configs").Where("id = ?", config.ID).Updates(updates).Error; err != nil {
				return nil, fmt.Errorf("update agent draft after resource revoke: %w", err)
			}
		}
	}
	if err := txRepo.RevokeResource(ctx, tx, ref); err != nil {
		return nil, err
	}
	return allAgentIDs, nil
}

func (r *Repository) impactAgents(ctx context.Context, bindings []Binding) ([]ImpactAgent, error) {
	agentIDs := make([]uuid.UUID, 0)
	seen := map[uuid.UUID]struct{}{}
	for _, binding := range bindings {
		if _, exists := seen[binding.AgentID]; exists {
			continue
		}
		seen[binding.AgentID] = struct{}{}
		agentIDs = append(agentIDs, binding.AgentID)
	}
	if len(agentIDs) == 0 {
		return []ImpactAgent{}, nil
	}
	type agentPresentation struct {
		ID          uuid.UUID
		Name        string
		Description string
		IconType    *string
		Icon        *string
	}
	var presentations []agentPresentation
	if err := r.db.WithContext(ctx).Table("agents").
		Select("id, name, description, icon_type, icon").
		Where("id IN ? AND deleted_at IS NULL", agentIDs).
		Find(&presentations).Error; err != nil {
		return nil, fmt.Errorf("load impacted agent presentation: %w", err)
	}
	presentationByID := make(map[uuid.UUID]agentPresentation, len(presentations))
	for _, presentation := range presentations {
		presentationByID[presentation.ID] = presentation
	}
	out := make([]ImpactAgent, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		presentation := presentationByID[agentID]
		out = append(out, ImpactAgent{
			AgentID:     agentID.String(),
			Name:        strings.TrimSpace(presentation.Name),
			Description: strings.TrimSpace(presentation.Description),
			IconType:    stringPtrValue(presentation.IconType),
			Icon:        stringPtrValue(presentation.Icon),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		leftName := strings.ToLower(out[i].Name)
		rightName := strings.ToLower(out[j].Name)
		if leftName == rightName {
			return out[i].AgentID < out[j].AgentID
		}
		if leftName == "" {
			return false
		}
		if rightName == "" {
			return true
		}
		return leftName < rightName
	})
	return out, nil
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func pruneAgentModeResource(raw *string, ref ResourceRef) (string, bool, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return "{}", false, nil
	}
	mode := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*raw), &mode); err != nil {
		return "", false, fmt.Errorf("decode agent mode: %w", err)
	}
	changed := false
	switch ref.BindingType {
	case BindingTypeSkill:
		mode["enabled_skill_ids"], changed = removeStringValue(mode["enabled_skill_ids"], ref.ResourceID)
	case BindingTypeKnowledgeDataset:
		mode["knowledge_dataset_ids"], changed = removeStringValue(mode["knowledge_dataset_ids"], ref.ResourceID)
	case BindingTypeDatabase:
		mode["database_bindings"], changed = removeDatabaseBinding(mode["database_bindings"], ref.ResourceID)
	case BindingTypeDatabaseTable:
		mode["database_bindings"], changed = removeDatabaseTableBinding(mode["database_bindings"], ref.ParentResourceID, ref.ResourceID)
	case BindingTypeWorkflow:
		mode["workflow_bindings"], changed = removeWorkflowBinding(mode["workflow_bindings"], ref.ResourceID)
	default:
		return "", false, fmt.Errorf("unsupported binding type %q", ref.BindingType)
	}
	if !changed {
		return *raw, false, nil
	}
	encoded, err := json.Marshal(mode)
	if err != nil {
		return "", false, fmt.Errorf("encode agent mode: %w", err)
	}
	return string(encoded), true, nil
}

func removeStringValue(value interface{}, target string) ([]interface{}, bool) {
	target = strings.TrimSpace(target)
	items, _ := value.([]interface{})
	out := make([]interface{}, 0, len(items))
	changed := false
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(item)), target) {
			changed = true
			continue
		}
		out = append(out, item)
	}
	return out, changed
}

func removeDatabaseBinding(value interface{}, dataSourceID string) ([]interface{}, bool) {
	items, _ := value.([]interface{})
	out := make([]interface{}, 0, len(items))
	changed := false
	for _, item := range items {
		binding, _ := item.(map[string]interface{})
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(binding["data_source_id"])), strings.TrimSpace(dataSourceID)) {
			changed = true
			continue
		}
		out = append(out, item)
	}
	return out, changed
}

func removeDatabaseTableBinding(value interface{}, dataSourceID, tableID string) ([]interface{}, bool) {
	items, _ := value.([]interface{})
	out := make([]interface{}, 0, len(items))
	changed := false
	for _, item := range items {
		binding, _ := item.(map[string]interface{})
		if dataSourceID != "" && !strings.EqualFold(strings.TrimSpace(fmt.Sprint(binding["data_source_id"])), strings.TrimSpace(dataSourceID)) {
			out = append(out, item)
			continue
		}
		tables, removed := removeStringValue(binding["table_ids"], tableID)
		if !removed {
			out = append(out, item)
			continue
		}
		changed = true
		if len(tables) == 0 {
			continue
		}
		binding["table_ids"] = tables
		binding["writable_table_ids"], _ = removeStringValue(binding["writable_table_ids"], tableID)
		out = append(out, binding)
	}
	return out, changed
}

func removeWorkflowBinding(value interface{}, resourceID string) ([]interface{}, bool) {
	items, _ := value.([]interface{})
	out := make([]interface{}, 0, len(items))
	changed := false
	for _, item := range items {
		binding, _ := item.(map[string]interface{})
		matches := false
		for _, key := range []string{"binding_id", "workflow_id", "agent_id"} {
			matches = matches || strings.EqualFold(strings.TrimSpace(fmt.Sprint(binding[key])), strings.TrimSpace(resourceID))
		}
		if matches {
			changed = true
			continue
		}
		out = append(out, item)
	}
	return out, changed
}

func bindingImpactRevision(bindings []Binding) string {
	rows := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		version := ""
		if binding.PublishedVersionUUID != nil {
			version = binding.PublishedVersionUUID.String()
		}
		rows = append(rows, strings.Join([]string{binding.AgentID.String(), string(binding.BindingScope), version, string(binding.BindingType), binding.ResourceID, binding.ParentResourceID, binding.AccessMode}, "|"))
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return hex.EncodeToString(sum[:])
}

func encodeImpactToken(secret []byte, payload impactTokenPayload) (string, error) {
	if len(secret) == 0 {
		return "", ErrImpactTokenNotConfigured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode agent binding impact token: %w", err)
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(digest.Sum(nil)), nil
}

func decodeImpactToken(secret []byte, token string) (impactTokenPayload, error) {
	if len(secret) == 0 {
		return impactTokenPayload{}, ErrImpactTokenNotConfigured
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return impactTokenPayload{}, ErrImpactTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return impactTokenPayload{}, ErrImpactTokenInvalid
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return impactTokenPayload{}, ErrImpactTokenInvalid
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	if !hmac.Equal(digest.Sum(nil), signature) {
		return impactTokenPayload{}, ErrImpactTokenInvalid
	}
	var payload impactTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return impactTokenPayload{}, ErrImpactTokenInvalid
	}
	return payload, nil
}
