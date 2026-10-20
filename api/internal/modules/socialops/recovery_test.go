package socialops

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArchiveRecoveryNeedsActualArtifactOnly(t *testing.T) {
	s, scope, p, g := publishFixture(t)
	ctx := context.Background()
	done, err := s.Publish(ctx, scope, p.ID, g, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.root.Remove(p.DraftArtifactID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, scope, done.ID, g, "UTC"); err != nil {
		t.Fatal("archive depends on removed draft", err)
	}
	if g.sends != 1 {
		t.Fatal("archive sent again")
	}
}
func TestEmptyRepostAndSubsecondMetricObservation(t *testing.T) {
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	raw, _ := json.Marshal(DraftBundle{Action: "repost", Chinese: "转发原帖", TargetID: "original"})
	a, err := s.PutArtifact(ctx, scope, "application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PreparePublication(ctx, PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "share", Platform: "x", DraftArtifactID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	g := &gatewaySpy{post: RemotePost{ID: "new-repost", URL: "https://x.com/example/status/new-repost", PublishedAt: now, RepostedID: "original"}}
	p, err = s.Publish(ctx, scope, p.ID, g, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(7*24*time.Hour + 500*time.Millisecond) }
	m, err := s.RecordMetric(ctx, scope, p.ID, map[string]*int64{})
	if err != nil || !m.Late {
		t.Fatal(m, err)
	}
}

func TestQuoteReadbackMustReferenceSelectedOriginal(t *testing.T) {
	s, scope, p, g := publishFixture(t)
	ctx := context.Background()
	raw, _ := json.Marshal(DraftBundle{Action: "quote", English: "Hello developers", Chinese: "开发者们好", TargetID: "selected-original"})
	a, err := s.PutArtifact(ctx, scope, "application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReplaceDraft(ctx, scope, p.ID, p.Revision, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	g.post.QuotedID = "different-original"
	if _, err = s.Publish(ctx, scope, p.ID, g, "UTC"); err == nil {
		t.Fatal("quote target mismatch archived")
	}
	g.post.QuotedID = "selected-original"
	if _, err = s.Publish(ctx, scope, p.ID, g, "UTC"); err != nil {
		t.Fatal(err)
	}
	if g.sends != 1 {
		t.Fatal("quote re-sent")
	}
}
func TestCorruptDraftCannotBeReviewed(t *testing.T) {
	s, scope, dir, _ := fixture(t)
	ctx := context.Background()
	a, err := s.PutArtifact(ctx, scope, "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PreparePublication(ctx, PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "corrupt", Platform: "x", DraftArtifactID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(dir+"/"+a.ID.String(), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest); err == nil {
		t.Fatal("corrupt draft reviewed")
	}
}
func TestMediaReadbackAndLocalArchiveReferences(t *testing.T) {
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	image, err := s.PutArtifact(ctx, scope, "image/png", []byte("local-image-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(DraftBundle{Action: "post", English: "Visual", Chinese: "配图", Images: []uuid.UUID{image.ID}})
	a, err := s.PutArtifact(ctx, scope, "application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PreparePublication(ctx, PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "visual", Platform: "x", DraftArtifactID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	media := RemoteMedia{ID: "media-1", URL: "https://example.com/image.png"}
	g := &gatewaySpy{post: RemotePost{ID: "post", URL: "https://example.com/post", Text: "Visual", PublishedAt: now, Media: []RemoteMedia{media}}}
	p, err = s.Publish(ctx, scope, p.ID, g, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.WriteDailyArchive(ctx, scope, p.ArchiveDate)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadArtifact(ctx, scope, record.ArtifactID)
	if err != nil || !strings.Contains(string(data), image.ID.String()) || !strings.Contains(string(data), media.URL) {
		t.Fatal(string(data), err)
	}
}

func TestSupplementalCommentRequiresVerifiedSameAccountParent(t *testing.T) {
	s, scope, parent, g := publishFixture(t)
	ctx := context.Background()
	a, err := s.PutArtifact(ctx, scope, "text/plain", []byte("supplement"))
	if err != nil {
		t.Fatal(err)
	}
	r := PublicationRequest{Scope: scope, ConnectionID: parent.ConnectionID, ContentKey: "comment", Platform: "x", DraftArtifactID: a.ID, ParentPublicationID: &parent.ID}
	if _, err = s.PreparePublication(ctx, r); err == nil {
		t.Fatal("unverified parent allowed")
	}
	parent, err = s.Publish(ctx, scope, parent.ID, g, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	r.ConnectionID = uuid.New()
	if _, err = s.PreparePublication(ctx, r); err == nil {
		t.Fatal("cross account parent allowed")
	}
	r.ConnectionID = parent.ConnectionID
	r.ParentRemoteID = "forged"
	if _, err = s.PreparePublication(ctx, r); err == nil {
		t.Fatal("parent substitution allowed")
	}
	r.ParentRemoteID = ""
	comment, err := s.PreparePublication(ctx, r)
	if err != nil || comment.ParentRemoteID != parent.RemoteID {
		t.Fatal(comment, err)
	}
}

func TestPublicationAccountsRemainSeparateWithinOneConnection(t *testing.T) {
	s, scope, _, _ := fixture(t)
	ctx := context.Background()
	a, err := s.PutArtifact(ctx, scope, "text/plain", []byte("draft"))
	if err != nil {
		t.Fatal(err)
	}
	r := PublicationRequest{Scope: scope, ConnectionID: uuid.New(), AccountID: "organization-a", ContentKey: "same-day", Platform: "linkedin", DraftArtifactID: a.ID}
	first, err := s.PreparePublication(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	r.AccountID = "organization-b"
	second, err := s.PreparePublication(ctx, r)
	if err != nil || first.ID == second.ID {
		t.Fatal(first, second, err)
	}
	again, err := s.PreparePublication(ctx, r)
	if err != nil || again.ID != second.ID {
		t.Fatal(again, err)
	}
}
