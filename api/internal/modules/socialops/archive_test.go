package socialops

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDailyLocalArchiveContainsActualPlatformContentOnce(t *testing.T) {
	s, scope, _, now := fixture(t)
	ctx := context.Background()
	connection := uuid.New()
	for _, item := range []struct{ key, platform, english, parent string }{{"main", "x", "Actual post", ""}, {"comment", "x", "Actual comment", "main"}, {"main", "facebook", "Actual Facebook variant", ""}} {
		draft, err := s.PutArtifact(ctx, scope, "text/plain", []byte("OLD DRAFT 中文审核稿"))
		if err != nil {
			t.Fatal(err)
		}
		p, err := s.PreparePublication(ctx, PublicationRequest{Scope: scope, ConnectionID: connection, ContentKey: item.platform + "-" + item.key, Platform: item.platform, DraftArtifactID: draft.ID, ParentRemoteID: item.parent})
		if err != nil {
			t.Fatal(err)
		}
		p, err = s.ReviewPublication(ctx, scope, p.ID, p.Revision, draft.Digest)
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
		remote := RemotePost{ID: item.key, URL: "https://example.com/" + item.platform + "/" + item.key, Text: item.english, ParentID: item.parent, PublishedAt: now}
		p, err = s.RecordCreated(ctx, scope, p.ID, p.Revision, remote)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := s.PutArtifact(ctx, scope, "text/plain", []byte(item.english))
		if err != nil {
			t.Fatal(err)
		}
		p, err = s.RecordVerified(ctx, scope, p.ID, p.Revision, remote, actual.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Archive(ctx, scope, p.ID, p.Revision, "Asia/Shanghai"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		record, err := s.WriteDailyArchive(ctx, scope, "2026-10-20")
		if err != nil {
			t.Fatal(err)
		}
		data, err := s.ReadArtifact(ctx, scope, record.ArtifactID)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, want := range []string{"# 2026-10-20", "## x", "## facebook", "Actual post", "Actual comment", "Actual Facebook variant", "https://example.com/x/main", "Parent post: main"} {
			if strings.Count(text, want) != 1 {
				t.Fatal("missing or duplicated", want, text)
			}
		}
		if strings.Contains(text, "OLD DRAFT") || strings.Contains(text, "中文") {
			t.Fatal("draft leaked into archive")
		}
	}
	rows, err := s.ListRecords(ctx, scope, "archive", uuid.Nil, 20)
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 {
		t.Fatal(rows, err)
	}
}
