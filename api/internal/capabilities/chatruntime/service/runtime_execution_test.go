package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestOldRunLeaseRenewalFailureDoesNotCancelNewOwner(t *testing.T) {
	tests := []struct {
		name        string
		active      bool
		renewErr    error
		lastSuccess time.Time
	}{
		{
			name:        "lease ownership replaced",
			active:      false,
			lastSuccess: time.Now(),
		},
		{
			name:        "lease renewal failed past grace period",
			renewErr:    errors.New("database unavailable"),
			lastSuccess: time.Now().Add(-runtimeLeaseFailureTTL),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messageID := uuid.New()
			oldRunID := uuid.New()
			newRunID := uuid.New()
			newCtx, newCancel := context.WithCancel(context.Background())
			t.Cleanup(newCancel)
			registry := newStreamRegistry()
			registry.Begin(messageID, newRunID, newCancel)
			lease := &runtimeLeaseRenewalStub{
				active:   test.active,
				renewErr: test.renewErr,
				renewed:  make(chan uuid.UUID, 1),
			}
			svc := &service{
				streams: registry,
				repos:   &repository.Repositories{RuntimeLease: lease},
			}

			returned := make(chan struct{})
			go func() {
				svc.renewRuntimeLeaseAtInterval(
					context.Background(),
					make(chan struct{}),
					messageID,
					oldRunID,
					test.lastSuccess,
					time.Millisecond,
				)
				close(returned)
			}()

			select {
			case renewedRunID := <-lease.renewed:
				if renewedRunID != oldRunID {
					t.Fatalf("Renew run ID = %s, want old owner %s", renewedRunID, oldRunID)
				}
			case <-time.After(time.Second):
				t.Fatal("lease renewal was not attempted")
			}
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("old lease renewal loop did not exit")
			}
			select {
			case <-newCtx.Done():
				t.Fatal("old lease renewal failure canceled the new owner")
			default:
			}
			if registry.CancelFunc(messageID, newRunID) == nil {
				t.Fatal("old lease renewal failure removed the new owner cancel function")
			}
		})
	}
}

type runtimeLeaseRenewalStub struct {
	active   bool
	renewErr error
	renewed  chan uuid.UUID
}

func (s *runtimeLeaseRenewalStub) Begin(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}

func (s *runtimeLeaseRenewalStub) Renew(_ context.Context, _ uuid.UUID, runID uuid.UUID, _ time.Time) (bool, error) {
	s.renewed <- runID
	return s.active, s.renewErr
}

func (s *runtimeLeaseRenewalStub) Release(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *runtimeLeaseRenewalStub) ListExpiredActiveIDs(context.Context, time.Time, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}

func (s *runtimeLeaseRenewalStub) MarkExpiredActiveAsError(context.Context, time.Time, time.Time, string) ([]uuid.UUID, error) {
	return nil, nil
}
