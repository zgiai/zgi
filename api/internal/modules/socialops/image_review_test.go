package socialops

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"testing"

	"github.com/google/uuid"
)

func inspectionImage(t *testing.T, s *Service, scope Scope) Artifact {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 120, 80))); err != nil {
		t.Fatal(err)
	}
	artifact, err := s.PutArtifact(context.Background(), scope, "image/png", encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func inspectionEvidence(t *testing.T, s *Service, scope Scope, image Artifact, revision int64, approved bool) Artifact {
	t.Helper()
	value := approved
	data, err := json.Marshal(imageInspectionEvidence{ArtifactID: image.ID, Digest: image.Digest, ConfigurationRevision: revision,
		Approved: &value, Issues: []string{}, Description: "actual pixels inspected", Provider: "fixture", Model: "vision", ObservedAt: s.now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := s.PutArtifact(context.Background(), scope, "application/json", data)
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func imagePublication(t *testing.T, s *Service, cfg Configuration, key string, image Artifact) (Publication, Artifact) {
	t.Helper()
	data, err := json.Marshal(DraftBundle{Action: "post", English: "Reviewed image", Chinese: "已审核配图", Images: []uuid.UUID{image.ID}})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.PutArtifact(context.Background(), cfg.Scope, "application/json", data)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := s.PreparePublication(context.Background(), PublicationRequest{Scope: cfg.Scope, ConnectionID: cfg.Platforms[0].ConnectionID,
		AccountID: cfg.Platforms[0].AccountID, ContentKey: key, Platform: cfg.Platforms[0].Platform, DraftArtifactID: draft.ID})
	if err != nil {
		t.Fatal(err)
	}
	return publication, draft
}

func TestConfiguredPublicationRequiresServerRecordedImageInspectionAtReviewAndReserve(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 2)
	imageArtifact := inspectionImage(t, s, scope)
	publication, draft := imagePublication(t, s, cfg, "image-policy", imageArtifact)
	if _, err := s.ReviewPublication(context.Background(), scope, publication.ID, publication.Revision, draft.Digest); err == nil {
		t.Fatal("configured image was reviewed without inspection")
	}

	foreign := scope
	foreign.OwnerID = uuid.New()
	foreignEvidence := inspectionEvidence(t, s, foreign, imageArtifact, cfg.Revision, true)
	if err := s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, foreignEvidence.ID, true); err == nil {
		t.Fatal("foreign review artifact was accepted")
	}

	rejectedEvidence := inspectionEvidence(t, s, scope, imageArtifact, cfg.Revision, false)
	if err := s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, rejectedEvidence.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewPublication(context.Background(), scope, publication.ID, publication.Revision, draft.Digest); err == nil {
		t.Fatal("rejected image was reviewed")
	}

	approvedEvidence := inspectionEvidence(t, s, scope, imageArtifact, cfg.Revision, true)
	if err := s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, approvedEvidence.ID, true); err != nil {
		t.Fatal(err)
	}
	var inspection ImageInspection
	if err := scoped(s.db, scope).Where("artifact_id = ? AND configuration_revision = ?", imageArtifact.ID, cfg.Revision).Take(&inspection).Error; err != nil || !inspection.Approved || inspection.ArtifactDigest != imageArtifact.Digest {
		t.Fatal(inspection, err)
	}
	publication, err := s.ReviewPublication(context.Background(), scope, publication.ID, publication.Revision, draft.Digest)
	if err != nil {
		t.Fatal(err)
	}
	newRejectedEvidence := inspectionEvidence(t, s, scope, imageArtifact, cfg.Revision, false)
	if err = s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, newRejectedEvidence.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReservePublication(context.Background(), scope, publication.ID, publication.Revision); err == nil {
		t.Fatal("reservation ignored revoked image approval")
	}
	if err = s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, approvedEvidence.ID, true); err != nil {
		t.Fatal(err)
	}
	publication, err = s.ReservePublication(context.Background(), scope, publication.ID, publication.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, newRejectedEvidence.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartDispatch(context.Background(), scope, publication.ID, publication.Revision); err == nil {
		t.Fatal("dispatch ignored revoked image approval")
	}
}

func TestImageInspectionCannotCrossConfigurationRevision(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 2)
	imageArtifact := inspectionImage(t, s, scope)
	evidence := inspectionEvidence(t, s, scope, imageArtifact, cfg.Revision, true)
	if err := s.RecordImageInspection(context.Background(), scope, imageArtifact.ID, evidence.ID, true); err != nil {
		t.Fatal(err)
	}
	cfg.Revision++
	savePolicyConfiguration(t, s, cfg, 1)
	publication, draft := imagePublication(t, s, cfg, "new-configuration", imageArtifact)
	if _, err := s.ReviewPublication(context.Background(), scope, publication.ID, publication.Revision, draft.Digest); err == nil {
		t.Fatal("old configuration image inspection was reused")
	}
}
