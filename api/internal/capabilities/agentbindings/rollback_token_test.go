package agentbindings

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRollbackImpactTokenBindsActorVersionAndHealth(t *testing.T) {
	repo := NewRepositoryWithTokenSecret(nil, "test-secret")
	now := time.Unix(1_700_000_000, 0)
	actorID := uuid.New()
	agentID := uuid.New()
	token, err := repo.CreateRollbackImpactToken(actorID, agentID, "version-1", "health-1", now)
	if err != nil {
		t.Fatalf("CreateRollbackImpactToken() error = %v", err)
	}
	if err := repo.VerifyRollbackImpactToken(actorID, agentID, "version-1", "health-1", token, now.Add(time.Minute)); err != nil {
		t.Fatalf("VerifyRollbackImpactToken() error = %v", err)
	}
	for name, verify := range map[string]func() error{
		"actor": func() error {
			return repo.VerifyRollbackImpactToken(uuid.New(), agentID, "version-1", "health-1", token, now)
		},
		"version": func() error {
			return repo.VerifyRollbackImpactToken(actorID, agentID, "version-2", "health-1", token, now)
		},
		"health": func() error {
			return repo.VerifyRollbackImpactToken(actorID, agentID, "version-1", "health-2", token, now)
		},
		"expired": func() error {
			return repo.VerifyRollbackImpactToken(actorID, agentID, "version-1", "health-1", token, now.Add(ImpactTokenTTL+time.Second))
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := verify(); !errors.Is(err, ErrRollbackImpactTokenInvalid) {
				t.Fatalf("VerifyRollbackImpactToken() error = %v, want invalid", err)
			}
		})
	}
}
