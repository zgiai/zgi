package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type gatewaySpy struct {
	sends, reads       int
	post               RemotePost
	sendErr, errorRead error
	receipt            *RemotePost
}

func (g *gatewaySpy) Send(_ context.Context, _ Publication, _ DraftBundle) (RemotePost, error) {
	g.sends++
	if g.receipt != nil {
		return *g.receipt, g.sendErr
	}
	if g.sendErr != nil {
		return RemotePost{}, g.sendErr
	}
	return g.post, nil
}

func TestPublishResumesReservedAndReconcilesUnknownWithExactRead(t *testing.T) {
	for _, mode := range []string{"reserved", "unknown", "partial_receipt"} {
		t.Run(mode, func(t *testing.T) {
			s, scope, p, gateway := publishFixture(t)
			ctx := context.Background()
			if mode == "reserved" {
				var err error
				p, err = s.ReservePublication(ctx, scope, p.ID, p.Revision)
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unknown" {
				gateway.sendErr = errors.New("lost response")
				if _, err := s.Publish(ctx, scope, p.ID, gateway, "UTC"); err == nil {
					t.Fatal("unknown not surfaced")
				}
				gateway.sendErr = nil
				p, err := s.ReconcilePublication(ctx, scope, p.ID, "123", gateway)
				if err != nil || p.State != "remote_created" {
					t.Fatal(p, err)
				}
			}
			if mode == "partial_receipt" {
				gateway.receipt = &RemotePost{ID: "123"}
			}
			done, err := s.Publish(ctx, scope, p.ID, gateway, "UTC")
			if err != nil || done.State != "archived" {
				t.Fatal(done, err)
			}
			if gateway.sends != 1 {
				t.Fatal("wrong send count", gateway.sends)
			}
		})
	}
}
func (g *gatewaySpy) Read(_ context.Context, _ Publication) (RemotePost, error) {
	g.reads++
	return g.post, g.errorRead
}
func publishFixture(t *testing.T) (*Service, Scope, Publication, *gatewaySpy) {
	t.Helper()
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	data, _ := json.Marshal(DraftBundle{Action: "post", English: "Hello developers", Chinese: "开发者们好"})
	draft, err := s.PutArtifact(ctx, scope, "application/json", data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PreparePublication(ctx, PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "day-one", Platform: "x", DraftArtifactID: draft.ID})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, draft.Digest)
	if err != nil {
		t.Fatal(err)
	}
	return s, scope, p, &gatewaySpy{post: RemotePost{ID: "123", URL: "https://x.com/example/status/123", Text: "Hello developers", PublishedAt: now}}
}
func TestPublishRunsReserveDispatchReadbackAndActualArchive(t *testing.T) {
	s, scope, p, gateway := publishFixture(t)
	ctx := context.Background()
	done, err := s.Publish(ctx, scope, p.ID, gateway, "UTC")
	if err != nil || done.State != "archived" {
		t.Fatal(done, err)
	}
	if gateway.sends != 1 || gateway.reads != 1 {
		t.Fatal(gateway)
	}
	if _, err = s.Publish(ctx, scope, p.ID, gateway, "UTC"); err != nil {
		t.Fatal(err)
	}
	if gateway.sends != 1 {
		t.Fatal("republished archived content")
	}
}
func TestPublishReadbackRecoveryNeverResends(t *testing.T) {
	s, scope, p, gateway := publishFixture(t)
	ctx := context.Background()
	gateway.errorRead = errors.New("temporary read failure")
	if _, err := s.Publish(ctx, scope, p.ID, gateway, "UTC"); err == nil {
		t.Fatal("readback failure hidden")
	}
	restored, err := s.GetPublication(ctx, scope, p.ID)
	if err != nil || restored.State != "remote_created" {
		t.Fatal(restored, err)
	}
	gateway.errorRead = nil
	if _, err = s.Publish(ctx, scope, p.ID, gateway, "UTC"); err != nil {
		t.Fatal(err)
	}
	if gateway.sends != 1 || gateway.reads != 2 {
		t.Fatal(gateway)
	}
}
func TestPublishUnknownAndReadbackMismatchNeverArchive(t *testing.T) {
	for _, mode := range []string{"unknown", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			s, scope, p, gateway := publishFixture(t)
			if mode == "unknown" {
				gateway.sendErr = errors.New("network timed out")
			} else {
				gateway.post.Text = "wrong public content"
			}
			if _, err := s.Publish(context.Background(), scope, p.ID, gateway, "UTC"); err == nil {
				t.Fatal("invalid provider response accepted")
			}
			saved, err := s.GetPublication(context.Background(), scope, p.ID)
			if err != nil || saved.State == "archived" {
				t.Fatal(saved, err)
			}
			if mode == "unknown" {
				if saved.State != "outcome_unknown" {
					t.Fatal(saved)
				}
				_, _ = s.Publish(context.Background(), scope, p.ID, gateway, "UTC")
				if gateway.sends != 1 {
					t.Fatal("uncertain send retried")
				}
			}
		})
	}
}

func TestPublishRetriesOnlyProvenNotSubmittedFailures(t *testing.T) {
	s, scope, p, gateway := publishFixture(t)
	ctx := context.Background()
	gateway.sendErr = &NotSubmittedError{Cause: errors.New("authorization rejected before provider")}
	saved, err := s.Publish(ctx, scope, p.ID, gateway, "UTC")
	if err == nil || saved.State != "reviewed" || saved.Revision <= p.Revision {
		t.Fatal(saved, err)
	}
	gateway.sendErr = nil
	if _, err = s.Publish(ctx, scope, p.ID, gateway, "UTC"); err != nil {
		t.Fatal(err)
	}
	if gateway.sends != 2 {
		t.Fatal("wrong attempted dispatches", gateway.sends)
	}
}

func (g *gatewaySpy) ValidateTarget(context.Context, Publication, DraftBundle) error { return nil }
