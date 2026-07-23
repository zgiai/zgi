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
)

const SkillSuspensionBindingOperation = "suspend_organization_skills"

type SkillSuspensionImpactRequest struct {
	OrganizationID uuid.UUID
	SkillIDs       []string
	ActorID        uuid.UUID
}

type skillSuspensionImpactTokenPayload struct {
	ActorID        string   `json:"actor_id"`
	Operation      string   `json:"operation"`
	OrganizationID string   `json:"organization_id"`
	SkillIDs       []string `json:"skill_ids"`
	Revision       string   `json:"revision"`
	ExpiresAt      int64    `json:"expires_at"`
}

func (r *Repository) PreviewSkillSuspensionImpact(ctx context.Context, req SkillSuspensionImpactRequest, now time.Time) (*Impact, error) {
	normalized, err := normalizeSkillSuspensionImpactRequest(req)
	if err != nil {
		return nil, err
	}
	bindings, err := r.listSkillSuspensionBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	payload := skillSuspensionImpactPayload(normalized, bindings, now.Add(ImpactTokenTTL))
	token, err := encodeSkillSuspensionImpactToken(r.tokenSecret, payload)
	if err != nil {
		return nil, err
	}
	agents, err := r.impactAgents(ctx, bindings)
	if err != nil {
		return nil, err
	}
	return &Impact{
		Code:        ConflictCodeResourceBound,
		Operation:   SkillSuspensionBindingOperation,
		BindingType: BindingTypeSkill,
		ResourceIDs: append([]string(nil), normalized.SkillIDs...),
		Agents:      agents,
		ImpactToken: token,
		ExpiresAt:   payload.ExpiresAt,
	}, nil
}

func (r *Repository) VerifySkillSuspensionImpactToken(ctx context.Context, req SkillSuspensionImpactRequest, token string, now time.Time) error {
	normalized, err := normalizeSkillSuspensionImpactRequest(req)
	if err != nil {
		return err
	}
	payload, err := decodeSkillSuspensionImpactToken(r.tokenSecret, token)
	if err != nil {
		return err
	}
	if payload.ExpiresAt < now.Unix() {
		return ErrImpactTokenInvalid
	}
	bindings, err := r.listSkillSuspensionBindings(ctx, normalized)
	if err != nil {
		return err
	}
	expected := skillSuspensionImpactPayload(normalized, bindings, time.Unix(payload.ExpiresAt, 0))
	if !skillSuspensionImpactPayloadEqual(payload, expected) {
		return ErrImpactChanged
	}
	return nil
}

func (r *Repository) listSkillSuspensionBindings(ctx context.Context, req SkillSuspensionImpactRequest) ([]Binding, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("agent bindings database is required")
	}
	var bindings []Binding
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND binding_type = ? AND resource_id IN ?", req.OrganizationID, BindingTypeSkill, req.SkillIDs).
		Order("agent_id ASC, binding_scope ASC, published_version_uuid ASC, resource_id ASC").
		Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list skill suspension binding impact: %w", err)
	}
	return bindings, nil
}

func normalizeSkillSuspensionImpactRequest(req SkillSuspensionImpactRequest) (SkillSuspensionImpactRequest, error) {
	if req.OrganizationID == uuid.Nil || req.ActorID == uuid.Nil {
		return SkillSuspensionImpactRequest{}, fmt.Errorf("organization and actor are required")
	}
	seen := map[string]struct{}{}
	for _, raw := range req.SkillIDs {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	req.SkillIDs = req.SkillIDs[:0]
	for id := range seen {
		req.SkillIDs = append(req.SkillIDs, id)
	}
	sort.Strings(req.SkillIDs)
	if len(req.SkillIDs) == 0 {
		return SkillSuspensionImpactRequest{}, fmt.Errorf("disabled skill ids are required")
	}
	return req, nil
}

func skillSuspensionImpactPayload(req SkillSuspensionImpactRequest, bindings []Binding, expiresAt time.Time) skillSuspensionImpactTokenPayload {
	return skillSuspensionImpactTokenPayload{
		ActorID:        req.ActorID.String(),
		Operation:      SkillSuspensionBindingOperation,
		OrganizationID: req.OrganizationID.String(),
		SkillIDs:       append([]string(nil), req.SkillIDs...),
		Revision:       bindingImpactRevision(bindings),
		ExpiresAt:      expiresAt.Unix(),
	}
}

func skillSuspensionImpactPayloadEqual(left, right skillSuspensionImpactTokenPayload) bool {
	left.ExpiresAt = 0
	right.ExpiresAt = 0
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return hmac.Equal(leftJSON, rightJSON)
}

func encodeSkillSuspensionImpactToken(secret []byte, payload skillSuspensionImpactTokenPayload) (string, error) {
	if len(secret) == 0 {
		return "", ErrImpactTokenNotConfigured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode skill suspension impact token: %w", err)
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(digest.Sum(nil)), nil
}

func decodeSkillSuspensionImpactToken(secret []byte, token string) (skillSuspensionImpactTokenPayload, error) {
	if len(secret) == 0 {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenNotConfigured
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(raw)
	if !hmac.Equal(digest.Sum(nil), signature) {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	var payload skillSuspensionImpactTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return skillSuspensionImpactTokenPayload{}, ErrImpactTokenInvalid
	}
	return payload, nil
}
