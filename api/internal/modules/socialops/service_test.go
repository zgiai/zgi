package socialops

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*Service, Scope, string, time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err = db.AutoMigrate(&Artifact{}, &ImageInspection{}, &Record{}, &Publication{}, &Metric{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s, err := New(db, dir, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}, dir, now
}
func TestLocalArtifactPersistenceAndScopedRecordCAS(t *testing.T) {
	s, scope, dir, _ := fixture(t)
	ctx := context.Background()
	artifact, err := s.PutArtifact(ctx, scope, "text/markdown", []byte("English publication\n中文审核"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := New(s.db, dir, s.now)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	data, err := restored.ReadArtifact(ctx, scope, artifact.ID)
	if err != nil || string(data) != "English publication\n中文审核" {
		t.Fatal(string(data), err)
	}
	other := scope
	other.OwnerID = uuid.New()
	if _, err = restored.ReadArtifact(ctx, other, artifact.ID); err == nil {
		t.Fatal("cross-owner artifact")
	}
	record, err := s.SaveRecord(ctx, scope, "draft", "content-1", 0, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRecord(ctx, scope, "draft", "content-1", 0, artifact.ID); err == nil {
		t.Fatal("overwrote record without revision")
	}
	if _, err = s.SaveRecord(ctx, scope, "draft", "content-1", record.Revision, artifact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRecord(ctx, other, "draft", "forged", 0, artifact.ID); err == nil {
		t.Fatal("bound foreign artifact")
	}
	if _, err = s.SaveRecord(ctx, scope, "draft", "../../escape", 0, artifact.ID); err == nil {
		t.Fatal("accepted path key")
	}
	if err = os.Remove(dir + "/" + artifact.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("/etc/passwd", dir+"/"+artifact.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadArtifact(ctx, scope, artifact.ID); err == nil {
		t.Fatal("read symlink")
	}
}
func TestPublicationOnceReadbackArchiveAndMetricAge(t *testing.T) {
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	a, err := s.PutArtifact(ctx, scope, "application/json", []byte(`{"english":"hello","chinese":"你好"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "content-1", Platform: "x", DraftArtifactID: a.ID}
	p, err := s.PreparePublication(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReservePublication(ctx, scope, p.ID, p.Revision); err == nil {
		t.Fatal("unreviewed published")
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.ReservePublication(ctx, scope, p.ID, p.Revision); err == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("duplicate reservations", winners.Load())
	}
	p, err = s.GetPublication(ctx, scope, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.StartDispatch(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Archive(ctx, scope, p.ID, p.Revision, "UTC"); err == nil {
		t.Fatal("archived unverified")
	}
	p, err = s.RecordCreated(ctx, scope, p.ID, p.Revision, RemotePost{ID: "123", URL: "https://x.com/example/status/123", Text: "hello", PublishedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.PutArtifact(ctx, scope, "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.RecordVerified(ctx, scope, p.ID, p.Revision, RemotePost{ID: "different"}, actual.ID)
	if err == nil {
		t.Fatal("wrong remote ID verified")
	}
	p, err = s.GetPublication(ctx, scope, requestID(t, s, scope, request))
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.RecordVerified(ctx, scope, p.ID, p.Revision, RemotePost{ID: "123", URL: "https://x.com/example/status/123", Text: "hello", PublishedAt: now}, actual.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Archive(ctx, scope, p.ID, p.Revision, "Asia/Shanghai")
	if err != nil || p.ArchiveDate != "2026-10-20" {
		t.Fatal(p, err)
	}
	if _, err = s.RecordMetric(ctx, scope, p.ID, map[string]*int64{"likes": nil}); err == nil {
		t.Fatal("early seven day sample")
	}
	s.now = func() time.Time { return now.Add(7*24*time.Hour + 2*time.Hour) }
	metric, err := s.RecordMetric(ctx, scope, p.ID, map[string]*int64{"likes": nil})
	if err != nil || !metric.Late || metric.AgeSeconds != int64((7*24+2)*3600) {
		t.Fatal(metric, err)
	}
	if _, err = s.RecordMetric(ctx, scope, p.ID, map[string]*int64{}); err == nil {
		t.Fatal("duplicate metric capture")
	}
}
func requestID(t *testing.T, s *Service, scope Scope, r PublicationRequest) uuid.UUID {
	t.Helper()
	p, err := s.PreparePublication(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}
func TestUnknownDispatchCannotBeRetried(t *testing.T) {
	s, scope, dir, _ := fixture(t)
	ctx := context.Background()
	a, err := s.PutArtifact(ctx, scope, "text/plain", []byte("draft"))
	if err != nil {
		t.Fatal(err)
	}
	r := PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "same", Platform: "facebook", DraftArtifactID: a.ID}
	p, err := s.PreparePublication(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReservePublication(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.StartDispatch(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := New(s.db, dir, s.now)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err = restored.StartDispatch(ctx, scope, p.ID, p.Revision); err == nil {
		t.Fatal("retried in-flight after restart")
	}
	p, err = restored.RecordUnknown(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restored.ReservePublication(ctx, scope, p.ID, p.Revision); err == nil {
		t.Fatal("retried unknown")
	}
	other := scope
	other.WorkspaceID = uuid.New()
	if _, err = s.GetPublication(ctx, other, p.ID); err == nil {
		t.Fatal("cross workspace access")
	}
}

func TestReviewedContentRevisionAndReplyParent(t *testing.T) {
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	draft, err := s.PutArtifact(ctx, scope, "text/plain", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	revised, err := s.PutArtifact(ctx, scope, "text/plain", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	request := PublicationRequest{Scope: scope, ConnectionID: uuid.New(), ContentKey: "reply-1", Platform: "linkedin", DraftArtifactID: draft.ID, ParentRemoteID: "parent-1"}
	p, err := s.PreparePublication(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, draft.Digest)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReplaceDraft(ctx, scope, p.ID, p.Revision, revised.ID)
	if err != nil || p.State != "draft" || p.ReviewedDigest != "" {
		t.Fatal(p, err)
	}
	if _, err = s.ReservePublication(ctx, scope, p.ID, p.Revision); err == nil {
		t.Fatal("changed reviewed draft published")
	}
	p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, revised.Digest)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReservePublication(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReplaceDraft(ctx, scope, p.ID, p.Revision, draft.ID); err == nil {
		t.Fatal("replaced reserved draft")
	}
	p, err = s.StartDispatch(ctx, scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	wrong := RemotePost{ID: "reply", URL: "https://www.linkedin.com/feed/update/reply", Text: "second", PublishedAt: now, ParentID: "other"}
	if _, err = s.RecordCreated(ctx, scope, p.ID, p.Revision, wrong); err == nil {
		t.Fatal("reply attached to wrong parent")
	}
	wrong.ParentID = "parent-1"
	p, err = s.RecordCreated(ctx, scope, p.ID, p.Revision, wrong)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.RecordVerified(ctx, scope, p.ID, p.Revision, wrong, revised.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(7 * 24 * time.Hour) }
	due, err := s.DueMetrics(ctx, scope, 10)
	if err != nil || len(due) != 1 || due[0].ID != p.ID {
		t.Fatal(due, err)
	}
	metric, err := s.RecordMetric(ctx, scope, p.ID, map[string]*int64{"impressions": nil})
	if err != nil || metric.Late || metric.ValuesJSON != `{"impressions":null}` {
		t.Fatal(metric, err)
	}
	due, err = s.DueMetrics(ctx, scope, 10)
	if err != nil || len(due) != 0 {
		t.Fatal(due, err)
	}
}
