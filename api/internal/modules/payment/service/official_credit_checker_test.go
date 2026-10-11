package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
)

// The official credit checker is an official-channel call site: while the console link is
// stale it must refuse locally instead of sending a call the console would reject. The
// credential itself is kept loaded (the sync client needs it to repair the link), so the
// gate is the explicit stale flag, not a missing credential.
func TestOfficialCreditCheckerRefusesWhileConsoleLinkIsStale(t *testing.T) {
	consolecred.Configure("instance-1", "secret")
	t.Cleanup(func() {
		consolecred.Global().SetStale(false)
		consolecred.Global().Clear()
	})

	checker := &consoleOfficialCreditChecker{
		grpcAddr:       "127.0.0.1:1",
		dialTimeout:    50 * time.Millisecond,
		requestTimeout: 50 * time.Millisecond,
	}

	consolecred.Global().SetStale(true)
	if !consolecred.Global().Loaded() {
		t.Fatalf("precondition: the credential must stay loaded while stale")
	}

	_, err := checker.GetOfficialBalance(context.Background(), uuid.New())
	if !errors.Is(err, ErrOfficialCreditUnavailable) {
		t.Fatalf("error = %v, want ErrOfficialCreditUnavailable while stale", err)
	}

	// Once the link recovers the checker is allowed to dial again (the dial itself fails
	// here because there is no console, which is a different, later error).
	consolecred.Global().SetStale(false)
	_, err = checker.GetOfficialBalance(context.Background(), uuid.New())
	if errors.Is(err, consolecred.ErrConsoleLinkStale) {
		t.Fatalf("checker still refused after the stale flag was cleared: %v", err)
	}
}
