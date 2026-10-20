package socialops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DraftBundle is the immutable reviewed payload. Internal Chinese text is
// never included in public connector input. Images name scoped local artifacts.
type DraftBundle struct {
	Action   string      `json:"action"`
	English  string      `json:"english"`
	Chinese  string      `json:"chinese"`
	TargetID string      `json:"target_id,omitempty"`
	Images   []uuid.UUID `json:"images,omitempty"`
	Sources  []uuid.UUID `json:"sources,omitempty"`
}

// PublicationGateway must use the ordinary native Connector router with the
// restored employee task identity. No implementation may resolve raw tokens or
// treat a draft as remote readback. Send is called at most once per publication.
type PublicationGateway interface {
	Send(context.Context, Publication, DraftBundle) (RemotePost, error)
	Read(context.Context, Publication) (RemotePost, error)
}

// NotSubmittedError may only be emitted by trusted gateway validation or
// authoritative native rejection evidence. Network errors and timeouts are
// never sufficient to prove that no external action occurred.
type NotSubmittedError struct{ Cause error }

func (e *NotSubmittedError) Error() string { return "social publication was not submitted" }
func (e *NotSubmittedError) Unwrap() error { return e.Cause }

func (s *Service) draftBundle(ctx context.Context, scope Scope, p Publication) (DraftBundle, error) {
	raw, err := s.ReadArtifact(ctx, scope, p.DraftArtifactID)
	if err != nil {
		return DraftBundle{}, err
	}
	var bundle DraftBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || decoder.Decode(new(any)) != io.EOF || len(bundle.Images) > 4 || len(bundle.Sources) > 32 {
		return DraftBundle{}, invalid()
	}
	switch bundle.Action {
	case "post":
		if bundle.TargetID != "" || p.ParentRemoteID != "" {
			return DraftBundle{}, invalid()
		}
	case "reply":
		if bundle.TargetID == "" || p.ParentRemoteID != bundle.TargetID {
			return DraftBundle{}, invalid()
		}
	case "quote", "repost":
		if bundle.TargetID == "" || p.ParentRemoteID != "" {
			return DraftBundle{}, invalid()
		}
	default:
		return DraftBundle{}, invalid()
	}
	if (bundle.Action != "repost" && strings.TrimSpace(bundle.English) == "") || strings.TrimSpace(bundle.Chinese) == "" || len(bundle.English) > 60000 || len(bundle.Chinese) > 60000 {
		return DraftBundle{}, invalid()
	}
	for _, id := range append(append([]uuid.UUID{}, bundle.Images...), bundle.Sources...) {
		if _, err = s.artifact(ctx, scope, id); err != nil {
			return DraftBundle{}, err
		}
	}
	return bundle, nil
}

// Publish resumes readback/archive phases without replaying Send. A process
// that dies during Send leaves dispatching, which requires explicit outcome
// reconciliation. It deliberately never infers that a missing receipt means
// the provider did not publish.
func (s *Service) Publish(ctx context.Context, scope Scope, id uuid.UUID, gateway PublicationGateway, timezone string) (Publication, error) {
	if gateway == nil {
		return Publication{}, invalid()
	}
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if p.ArchiveTimezone != "" {
		timezone = p.ArchiveTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Publication{}, invalid()
	}

	p, err = s.refreshUnsubmittedReservation(ctx, p)
	if err != nil {
		return p, err
	}
	var bundle DraftBundle
	if p.State != "verified" && p.State != "archived" {
		bundle, err = s.draftBundle(ctx, scope, p)
		if err != nil {
			return Publication{}, err
		}
	}
	if (p.State == "reviewed" || p.State == "reserved") && bundle.TargetID != "" {
		validator, ok := gateway.(TargetValidator)
		if !ok {
			return p, invalid()
		}
		if err = validator.ValidateTarget(ctx, p, bundle); err != nil {
			return p, err
		}
	}
	if p.State == "reviewed" {
		p, err = s.ReservePublication(ctx, scope, p.ID, p.Revision)
		if err != nil {
			return Publication{}, err
		}
	}
	if p.State == "reserved" {
		p, err = s.StartDispatch(ctx, scope, p.ID, p.Revision)
		if err != nil {
			return Publication{}, err
		}
		remote, sendErr := gateway.Send(ctx, p, bundle)
		// Persist after a canceled transport too. This context is bounded and only
		// used for local/DB receipt storage; it cannot issue another network write.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if sendErr != nil && remote.ID == "" {
			var notSubmitted *NotSubmittedError
			var saved Publication
			var saveErr error
			if errors.As(sendErr, &notSubmitted) {
				saved, saveErr = s.transition(persist, scope, p.ID, p.Revision, "dispatching", "reviewed", nil)
			} else {
				saved, saveErr = s.RecordUnknown(persist, scope, p.ID, p.Revision)
			}
			cancel()
			if saveErr != nil {
				return p, saveErr
			}
			return saved, sendErr
		}
		p, err = s.RecordCreated(persist, scope, p.ID, p.Revision, remote)
		cancel()
		if err != nil {
			return p, err
		}
		if sendErr != nil {
			return p, sendErr
		}
	}
	if p.State == "remote_created" {
		actual, readErr := gateway.Read(ctx, p)
		if readErr != nil {
			return p, readErr
		}
		if !readbackMatchesBundle(actual, bundle) {
			return p, conflict()
		}
		artifact, writeErr := s.PutArtifact(ctx, scope, "text/plain", []byte(actual.Text))
		if writeErr != nil {
			return p, writeErr
		}
		p, err = s.RecordVerified(ctx, scope, p.ID, p.Revision, actual, artifact.ID, bundle.Images...)
		if err != nil {
			return p, err
		}
	}
	if p.State == "verified" {
		p, err = s.Archive(ctx, scope, p.ID, p.Revision, timezone)
		if err != nil {
			return p, err
		}
	}
	if p.State != "archived" {
		return p, conflict()
	}
	_, err = s.WriteDailyArchive(ctx, scope, p.ArchiveDate)
	return p, err
}

// TargetValidator verifies an external reply/quote/repost target through a real
// platform read before a new write. Local supplemental parents have an
// additional journal identity check.
type TargetValidator interface {
	ValidateTarget(context.Context, Publication, DraftBundle) error
}

// ReconcilePublication recovers only from an explicit remote ID using a fresh
// native read. It never searches by similar text and never sends. The gateway
// must verify the actual author/account and any quoted/replied target.
func (s *Service) ReconcilePublication(ctx context.Context, scope Scope, id uuid.UUID, remoteID string, gateway PublicationGateway) (Publication, error) {
	if gateway == nil || !validKey(remoteID) {
		return Publication{}, invalid()
	}
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if p.State != "outcome_unknown" && p.State != "dispatching" {
		return p, conflict()
	}
	bundle, err := s.draftBundle(ctx, scope, p)
	if err != nil {
		return p, err
	}
	// Missing upload receipts cannot certify that a discovered image matches the
	// reviewed local image. Keep such uncertain media submissions for review.
	if len(bundle.Images) > 0 || bundle.Action == "repost" {
		return p, conflict()
	}
	candidate := p
	candidate.RemoteID = remoteID
	actual, err := gateway.Read(ctx, candidate)
	if err != nil {
		return p, err
	}
	if !validRemote(actual, s.now().UTC()) || actual.ID != remoteID || !readbackMatchesBundle(actual, bundle) || actual.ParentID != p.ParentRemoteID || actual.PublishedAt.Before(p.CreatedAt.Truncate(time.Second)) {
		return p, conflict()
	}
	return s.transition(ctx, scope, id, p.Revision, p.State, "remote_created", receiptFields(actual))
}

func readbackMatchesBundle(actual RemotePost, bundle DraftBundle) bool {
	if bundle.Action != "repost" && actual.Text != bundle.English {
		return false
	}
	if bundle.Action == "quote" && actual.QuotedID != bundle.TargetID {
		return false
	}
	if bundle.Action == "repost" && actual.RepostedID != bundle.TargetID {
		return false
	}
	return true
}
