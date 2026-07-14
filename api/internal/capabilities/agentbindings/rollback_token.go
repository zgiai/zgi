package agentbindings

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrRollbackImpactTokenInvalid = errors.New("agent rollback impact token is invalid")

type rollbackImpactTokenPayload struct {
	ActorID        string `json:"actor_id"`
	AgentID        string `json:"agent_id"`
	VersionID      string `json:"version_id"`
	HealthRevision string `json:"health_revision"`
	ExpiresAt      int64  `json:"expires_at"`
}

func (r *Repository) CreateRollbackImpactToken(actorID, agentID uuid.UUID, versionID, healthRevision string, now time.Time) (string, error) {
	if r == nil || len(r.tokenSecret) == 0 {
		return "", ErrImpactTokenNotConfigured
	}
	payload := rollbackImpactTokenPayload{
		ActorID:        actorID.String(),
		AgentID:        agentID.String(),
		VersionID:      strings.TrimSpace(versionID),
		HealthRevision: strings.TrimSpace(healthRevision),
		ExpiresAt:      now.Add(ImpactTokenTTL).Unix(),
	}
	if actorID == uuid.Nil || agentID == uuid.Nil || payload.VersionID == "" || payload.HealthRevision == "" {
		return "", ErrRollbackImpactTokenInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := hmac.New(sha256.New, r.tokenSecret)
	_, _ = digest.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(digest.Sum(nil)), nil
}

func (r *Repository) VerifyRollbackImpactToken(actorID, agentID uuid.UUID, versionID, healthRevision, token string, now time.Time) error {
	if r == nil || len(r.tokenSecret) == 0 {
		return ErrImpactTokenNotConfigured
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return ErrRollbackImpactTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrRollbackImpactTokenInvalid
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return ErrRollbackImpactTokenInvalid
	}
	digest := hmac.New(sha256.New, r.tokenSecret)
	_, _ = digest.Write(raw)
	if !hmac.Equal(digest.Sum(nil), signature) {
		return ErrRollbackImpactTokenInvalid
	}
	var payload rollbackImpactTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ErrRollbackImpactTokenInvalid
	}
	if payload.ExpiresAt < now.Unix() || payload.ActorID != actorID.String() || payload.AgentID != agentID.String() ||
		payload.VersionID != strings.TrimSpace(versionID) || payload.HealthRevision != strings.TrimSpace(healthRevision) {
		return ErrRollbackImpactTokenInvalid
	}
	return nil
}
